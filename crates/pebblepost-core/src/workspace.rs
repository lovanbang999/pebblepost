use std::cmp::Ordering;
use std::collections::{BTreeMap, HashMap};
use std::fs;
use std::io;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering as AtomicOrdering};
use std::sync::{Mutex, OnceLock};
use std::time::{Duration, Instant};

use serde::Serialize;
use serde_json::{Value, json};
use thiserror::Error;

use crate::models::{
    EnvironmentDefinition, KeyValue, SecretBackendConfig, TreeNode, WorkspaceDefinition,
};

const PEBBLE_DIR: &str = ".pebble";
const ENVIRONMENTS_DIR: &str = "environments";
const COLLECTIONS_DIR: &str = "collections";
const REQUEST_SUFFIX: &str = ".pebble.json";
const FOLDER_FILE: &str = "_folder.pebble.json";
static TEMP_ID: AtomicU64 = AtomicU64::new(0);
static SUPPRESSED_PATHS: OnceLock<Mutex<HashMap<PathBuf, Instant>>> = OnceLock::new();

/// Suppress watcher events caused by PebblePost's own filesystem writes.
pub fn suppress_workspace_path(path: &Path, duration: Duration) {
    let path = normalize_watch_path(path);
    let duration = if duration.is_zero() {
        Duration::from_secs(1)
    } else {
        duration
    };
    let expiry = Instant::now() + duration;
    let paths = SUPPRESSED_PATHS.get_or_init(|| Mutex::new(HashMap::new()));
    let mut paths = paths
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner);
    paths.retain(|_, expires| *expires > Instant::now());
    paths.insert(path, expiry);
}

/// Return whether a path or one of its parent directories is currently suppressed.
pub fn workspace_path_is_suppressed(path: &Path) -> bool {
    let path = normalize_watch_path(path);
    let now = Instant::now();
    let Some(paths) = SUPPRESSED_PATHS.get() else {
        return false;
    };
    let mut paths = paths
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner);
    paths.retain(|_, expires| *expires > now);
    path.ancestors()
        .any(|ancestor| paths.get(ancestor).is_some_and(|expires| *expires > now))
}

fn normalize_watch_path(path: &Path) -> PathBuf {
    let absolute = if path.is_absolute() {
        path.to_path_buf()
    } else {
        std::env::current_dir().unwrap_or_default().join(path)
    };
    let mut normalized = PathBuf::new();
    for component in absolute.components() {
        match component {
            std::path::Component::CurDir => {}
            std::path::Component::ParentDir => {
                normalized.pop();
            }
            other => normalized.push(other.as_os_str()),
        }
    }
    if let Ok(canonical) = fs::canonicalize(&normalized) {
        return canonical;
    }
    if let (Some(parent), Some(name)) = (normalized.parent(), normalized.file_name())
        && let Ok(parent) = fs::canonicalize(parent)
    {
        return parent.join(name);
    }
    normalized
}

#[derive(Debug, Error)]
pub enum PebbleError {
    #[error("invalid path: {0}")]
    InvalidPath(String),
    #[error("I/O error: {0}")]
    Io(#[from] io::Error),
    #[error("invalid JSON: {0}")]
    Json(#[from] serde_json::Error),
}

#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ScanResult {
    pub workspace: WorkspaceDefinition,
    pub tree: Vec<TreeNode>,
    pub environments: Vec<EnvironmentDefinition>,
    pub gitignore_status: String,
}

pub fn scan_workspace(root: &Path) -> Result<ScanResult, PebbleError> {
    validate_untrusted_path(root)?;
    let root = canonical_existing(root)?;
    if !root.is_dir() {
        return Err(PebbleError::InvalidPath(
            "workspace path is not a directory".into(),
        ));
    }
    let collections = root.join(COLLECTIONS_DIR);
    let scan_root = if collections.is_dir() {
        canonical_existing(&collections)?
    } else {
        root.clone()
    };
    let tree = scan_dir(&scan_root, &root)?;
    let workspace = get_workspace_info(&root)?;
    let environments = list_environments(&root)?;
    let gitignore_status = match fs::read_to_string(root.join(".gitignore")) {
        Err(err) if err.kind() == io::ErrorKind::NotFound => "missing",
        Err(_) => "no_rule",
        Ok(contents) if contents.lines().any(gitignore_has_secret_rule) => "ok",
        Ok(_) => "no_rule",
    };
    Ok(ScanResult {
        workspace,
        tree,
        environments,
        gitignore_status: gitignore_status.into(),
    })
}

pub fn validate_workspace_root(requested: &Path) -> Result<PathBuf, PebbleError> {
    validate_untrusted_path(requested)?;
    let root = fs::canonicalize(requested).map_err(|error| {
        PebbleError::InvalidPath(format!("workspace path does not exist: {error}"))
    })?;
    if !root.is_dir() {
        return Err(PebbleError::InvalidPath(
            "workspace path is not a directory".into(),
        ));
    }
    Ok(root)
}

pub fn get_workspace_info(root: &Path) -> Result<WorkspaceDefinition, PebbleError> {
    let file = root.join(PEBBLE_DIR).join("workspace.json");
    match fs::read(file) {
        Ok(data) => {
            let mut workspace: WorkspaceDefinition = serde_json::from_slice(&data)?;
            if workspace.schema_version < 1 {
                workspace.schema_version = 1;
            }
            Ok(workspace)
        }
        Err(err) if err.kind() == io::ErrorKind::NotFound => {
            let name = root
                .file_name()
                .and_then(|part| part.to_str())
                .unwrap_or("My Workspace");
            Ok(WorkspaceDefinition {
                schema_version: 1,
                version: "1.0".into(),
                name: name.into(),
                active_environment: "dev".into(),
                trusted: true,
                secret_backend: SecretBackendConfig {
                    default: "file".into(),
                    per_env: BTreeMap::new(),
                },
            })
        }
        Err(err) => Err(err.into()),
    }
}

/// Persist workspace metadata using the same atomic JSON write as other
/// workspace configuration files.
pub fn save_workspace_info(
    root: &Path,
    definition: &WorkspaceDefinition,
) -> Result<(), PebbleError> {
    if root.as_os_str().is_empty() {
        return Err(PebbleError::InvalidPath(
            "workspace root cannot be empty".into(),
        ));
    }
    validate_untrusted_path(root)?;
    let root = canonical_existing(root)?;
    if !root.is_dir() {
        return Err(PebbleError::InvalidPath(
            "workspace path is not a directory".into(),
        ));
    }
    let metadata_dir = root.join(PEBBLE_DIR);
    match fs::symlink_metadata(&metadata_dir) {
        Ok(metadata) if metadata.file_type().is_symlink() => {
            return Err(PebbleError::InvalidPath(
                "workspace metadata directory cannot be a symlink".into(),
            ));
        }
        Ok(_) => {}
        Err(error) if error.kind() == io::ErrorKind::NotFound => fs::create_dir_all(&metadata_dir)?,
        Err(error) => return Err(error.into()),
    }
    let metadata_dir = canonical_existing(&metadata_dir)?;
    if !metadata_dir.starts_with(&root) {
        return Err(PebbleError::InvalidPath(
            "workspace metadata path escapes workspace root".into(),
        ));
    }
    write_json_atomic(&metadata_dir.join("workspace.json"), definition)
}

pub fn init_workspace(root: &Path, name: Option<&str>) -> Result<WorkspaceDefinition, PebbleError> {
    if root.as_os_str().is_empty() {
        return Err(PebbleError::InvalidPath(
            "workspace root cannot be empty".into(),
        ));
    }
    validate_untrusted_path(root)?;
    fs::create_dir_all(root.join(PEBBLE_DIR).join(ENVIRONMENTS_DIR))?;
    suppress_workspace_path(&root.join(COLLECTIONS_DIR), Duration::from_secs(1));
    fs::create_dir_all(root.join(COLLECTIONS_DIR))?;
    let workspace_name = name
        .filter(|value| !value.trim().is_empty())
        .map(str::to_owned)
        .or_else(|| {
            root.file_name()
                .and_then(|part| part.to_str())
                .map(str::to_owned)
        })
        .unwrap_or_else(|| "My Workspace".into());
    let definition = WorkspaceDefinition {
        schema_version: 1,
        version: "1.0".into(),
        name: workspace_name,
        active_environment: "dev".into(),
        trusted: true,
        secret_backend: SecretBackendConfig {
            default: "file".into(),
            per_env: BTreeMap::new(),
        },
    };
    let metadata_dir = root.join(PEBBLE_DIR);
    let workspace_file = metadata_dir.join("workspace.json");
    if !workspace_file.exists() {
        write_json_atomic(&workspace_file, &definition)?;
    }
    let local_gitignore = metadata_dir.join(".gitignore");
    if !local_gitignore.exists() {
        fs::write(
            &local_gitignore,
            "# Ignore secret environment files containing credentials and tokens\n*.secret.env.json\n",
        )?;
    }
    let env_file = root
        .join(PEBBLE_DIR)
        .join(ENVIRONMENTS_DIR)
        .join("dev.env.json");
    if !env_file.exists() {
        write_json_atomic(
            &env_file,
            &EnvironmentDefinition {
                schema_version: 1,
                name: "dev".into(),
                variables: vec![KeyValue {
                    key: "BASE_URL".into(),
                    value: "https://httpbin.org".into(),
                    enabled: true,
                    ..KeyValue::default()
                }],
            },
        )?;
    }
    let example_dir = root.join(COLLECTIONS_DIR).join("example");
    suppress_workspace_path(&example_dir, Duration::from_secs(1));
    fs::create_dir_all(&example_dir)?;
    let example_file = example_dir.join("get-started.pebble.json");
    if !example_file.exists() {
        write_json_atomic(
            &example_file,
            &json!({
                "$schema": "https://pebblepost.dev/schemas/v2/request.json",
                "schemaVersion": 2,
                "version": "1.0",
                "name": "Get Started (HTTPBin)",
                "description": "Sample request demonstrating PebblePost collection capabilities",
                "method": "GET",
                "url": "{{BASE_URL}}/get",
                "headers": [
                    { "key": "Accept", "value": "application/json", "enabled": true },
                    { "key": "User-Agent", "value": "PebblePost/0.1.0", "enabled": true }
                ],
                "params": [{ "key": "source", "value": "pebblepost", "enabled": true }],
                "auth": { "type": "none" },
                "body": { "type": "none" },
                "scripts": {
                    "preRequest": "// Pre-request script example\npb.request.headers.set('X-Timestamp', Date.now().toString());",
                    "postResponse": "// Post-response test assertions\npb.test('Status code is 200', () => {\n  pb.expect(pb.response.status).to.eql(200);\n});"
                },
                "settings": { "followRedirects": true, "verifySSL": true, "timeoutMs": 30000 }
            }),
        )?;
    }
    get_workspace_info(root)
}

pub fn list_environments(root: &Path) -> Result<Vec<EnvironmentDefinition>, PebbleError> {
    let env_dir = root.join(PEBBLE_DIR).join(ENVIRONMENTS_DIR);
    let entries = match fs::read_dir(env_dir) {
        Ok(entries) => entries,
        Err(err) if err.kind() == io::ErrorKind::NotFound => return Ok(Vec::new()),
        Err(err) => return Err(err.into()),
    };
    let mut environment_map = std::collections::BTreeMap::<String, EnvironmentDefinition>::new();
    let mut secret_map = std::collections::BTreeMap::<String, Vec<KeyValue>>::new();
    for entry in entries {
        let entry = entry?;
        let name = entry.file_name().to_string_lossy().into_owned();
        if !name.ends_with(".env.json") || !entry.file_type()?.is_file() {
            continue;
        }
        let data = fs::read(entry.path())?;
        let is_secret_file = name.ends_with(".secret.env.json");
        let environment_name = if is_secret_file {
            name.trim_end_matches(".secret.env.json")
        } else {
            name.trim_end_matches(".env.json")
        };
        let mut environment: EnvironmentDefinition = match serde_json::from_slice(&data) {
            Ok(environment) => environment,
            Err(_) if is_secret_file => continue,
            Err(error) => return Err(error.into()),
        };
        if environment.schema_version < 1 {
            environment.schema_version = 1;
        }
        if environment.name.is_empty() {
            environment.name = environment_name.to_owned();
        }
        if is_secret_file {
            for variable in &mut environment.variables {
                variable.secret = true;
            }
            secret_map.insert(environment_name.to_owned(), environment.variables);
        } else {
            environment_map.insert(environment_name.to_owned(), environment);
        }
    }
    for (name, variables) in secret_map {
        let target = environment_map
            .entry(name.clone())
            .or_insert_with(|| EnvironmentDefinition {
                schema_version: 1,
                name,
                variables: Vec::new(),
            });
        merge_environment_variables(&mut target.variables, variables);
    }
    Ok(environment_map.into_values().collect())
}

pub fn save_environment(
    root: &Path,
    mut environment: EnvironmentDefinition,
    is_secret: bool,
) -> Result<(), PebbleError> {
    validate_identifier(&environment.name)?;
    let env_dir = root.join(PEBBLE_DIR).join(ENVIRONMENTS_DIR);
    fs::create_dir_all(&env_dir)?;
    environment.schema_version = 1;
    let public_path = env_dir.join(format!("{}.env.json", environment.name));
    let secret_path = env_dir.join(format!("{}.secret.env.json", environment.name));
    if is_secret {
        for variable in &mut environment.variables {
            variable.secret = true;
        }
        write_json_atomic_with_mode(&secret_path, &environment, Some(0o600))?;
        return Ok(());
    }
    let mut public_variables = Vec::new();
    let mut secret_variables = Vec::new();
    for variable in environment.variables {
        if variable.secret {
            secret_variables.push(variable);
        } else {
            public_variables.push(variable);
        }
    }
    write_json_atomic(
        &public_path,
        &EnvironmentDefinition {
            schema_version: 1,
            name: environment.name.clone(),
            variables: public_variables,
        },
    )?;
    if secret_variables.is_empty() {
        match fs::remove_file(&secret_path) {
            Ok(()) => {}
            Err(error) if error.kind() == io::ErrorKind::NotFound => {}
            Err(error) => return Err(error.into()),
        }
    } else {
        write_json_atomic_with_mode(
            &secret_path,
            &EnvironmentDefinition {
                schema_version: 1,
                name: environment.name,
                variables: secret_variables,
            },
            Some(0o600),
        )?;
    }
    Ok(())
}

pub fn delete_environment(root: &Path, name: &str) -> Result<(), PebbleError> {
    validate_identifier(name)?;
    let env_dir = root.join(PEBBLE_DIR).join(ENVIRONMENTS_DIR);
    for suffix in [".env.json", ".secret.env.json"] {
        let path = env_dir.join(format!("{name}{suffix}"));
        suppress_workspace_path(&path, Duration::from_secs(1));
        match fs::remove_file(path) {
            Ok(()) => {}
            Err(error) if error.kind() == io::ErrorKind::NotFound => {}
            Err(error) => return Err(error.into()),
        }
    }
    Ok(())
}

pub fn read_request(root: &Path, request_path: &Path) -> Result<Value, PebbleError> {
    let path = safe_workspace_path(root, request_path)?;
    let data = fs::read(path)?;
    let mut request: Value = serde_json::from_slice(&data)?;
    migrate_request(&mut request);
    Ok(request)
}

const CURRENT_REQUEST_SCHEMA_VERSION: u64 = 2;
const CURRENT_FOLDER_SCHEMA_VERSION: u64 = 2;
const DEFAULT_REQUEST_SCHEMA: &str = "https://pebblepost.dev/schemas/v2/request.json";
const LEGACY_REQUEST_SCHEMA: &str = "https://pebblepost.dev/schemas/v1/request.json";

fn migrate_request(request: &mut Value) {
    let Some(object) = request.as_object_mut() else {
        return;
    };
    let schema_version = object
        .get("schemaVersion")
        .and_then(Value::as_u64)
        .unwrap_or(0);
    if schema_version < CURRENT_REQUEST_SCHEMA_VERSION {
        let default_auth = if schema_version < 1 {
            "none"
        } else {
            "inherit"
        };
        let auth = object.entry("auth").or_insert_with(|| json!({}));
        if !auth.is_object() {
            *auth = json!({});
        }
        if auth
            .get("type")
            .and_then(Value::as_str)
            .is_none_or(str::is_empty)
        {
            auth["type"] = json!(default_auth);
        }
        if !object.get("extractors").is_some_and(Value::is_array) {
            object.insert("extractors".into(), json!([]));
        }
        object.insert(
            "schemaVersion".into(),
            json!(CURRENT_REQUEST_SCHEMA_VERSION),
        );
    } else {
        let auth = object.entry("auth").or_insert_with(|| json!({}));
        if !auth.is_object() {
            *auth = json!({});
        }
        if auth
            .get("type")
            .and_then(Value::as_str)
            .is_none_or(str::is_empty)
        {
            auth["type"] = json!("inherit");
        }
    }
    if object
        .get("$schema")
        .and_then(Value::as_str)
        .is_none_or(|schema| schema.is_empty() || schema == LEGACY_REQUEST_SCHEMA)
    {
        object.insert("$schema".into(), json!(DEFAULT_REQUEST_SCHEMA));
    }
    if object
        .get("version")
        .and_then(Value::as_str)
        .is_none_or(str::is_empty)
    {
        object.insert("version".into(), json!("1.0"));
    }
}

pub fn save_request(
    root: &Path,
    request_path: &Path,
    mut request: Value,
) -> Result<PathBuf, PebbleError> {
    let path = safe_workspace_write_path(root, request_path)?;
    if let Some(object) = request.as_object_mut() {
        object.insert(
            "schemaVersion".into(),
            json!(CURRENT_REQUEST_SCHEMA_VERSION),
        );
        if object
            .get("version")
            .and_then(Value::as_str)
            .is_none_or(str::is_empty)
        {
            object.insert("version".into(), json!("1.0"));
        }
        if object
            .get("$schema")
            .and_then(Value::as_str)
            .is_none_or(str::is_empty)
        {
            object.insert("$schema".into(), json!(DEFAULT_REQUEST_SCHEMA));
        }
    } else {
        return Err(PebbleError::InvalidPath(
            "request must be a JSON object".into(),
        ));
    }
    write_json_atomic(&path, &request)?;
    Ok(path)
}

pub fn read_folder(root: &Path, folder_path: &Path) -> Result<Value, PebbleError> {
    let path = safe_workspace_path(root, folder_path)?;
    let metadata_path = if path.file_name().and_then(|name| name.to_str()) == Some(FOLDER_FILE) {
        path
    } else {
        path.join(FOLDER_FILE)
    };
    match fs::read(&metadata_path) {
        Ok(data) => {
            let mut folder: Value = serde_json::from_slice(&data)?;
            if let Some(object) = folder.as_object_mut() {
                let version = object
                    .get("schemaVersion")
                    .and_then(Value::as_u64)
                    .unwrap_or(0);
                if version < CURRENT_FOLDER_SCHEMA_VERSION {
                    object.insert("schemaVersion".into(), json!(CURRENT_FOLDER_SCHEMA_VERSION));
                }
                if object
                    .get("name")
                    .and_then(Value::as_str)
                    .is_none_or(str::is_empty)
                {
                    object.insert("name".into(), json!(folder_display_name(&metadata_path)));
                }
                let auth = object.entry("auth").or_insert_with(|| json!({}));
                if !auth.is_object() {
                    *auth = json!({});
                }
                if auth
                    .get("type")
                    .and_then(Value::as_str)
                    .is_none_or(str::is_empty)
                {
                    auth["type"] = json!("inherit");
                }
            }
            Ok(folder)
        }
        Err(error) if error.kind() == io::ErrorKind::NotFound => {
            let display_name = folder_display_name(&metadata_path);
            Ok(json!({
                "schemaVersion": CURRENT_FOLDER_SCHEMA_VERSION,
                "name": display_name,
                "auth": { "type": "inherit" },
                "headers": [],
                "variables": [],
            }))
        }
        Err(error) => Err(error.into()),
    }
}

fn folder_display_name(metadata_path: &Path) -> String {
    let folder_name = metadata_path
        .parent()
        .and_then(Path::file_name)
        .and_then(|name| name.to_str())
        .unwrap_or("Folder");
    parse_order_prefix(folder_name).1
}

pub fn save_folder(root: &Path, folder_path: &Path, mut folder: Value) -> Result<(), PebbleError> {
    let folder_path = safe_workspace_target(root, folder_path)?;
    let file_path = if folder_path.file_name().and_then(|name| name.to_str()) == Some(FOLDER_FILE) {
        folder_path
    } else {
        folder_path.join(FOLDER_FILE)
    };
    if !folder.is_object() {
        return Err(PebbleError::InvalidPath(
            "folder must be a JSON object".into(),
        ));
    }
    folder
        .as_object_mut()
        .expect("validated object")
        .insert("schemaVersion".into(), json!(CURRENT_FOLDER_SCHEMA_VERSION));
    write_json_atomic(&file_path, &folder)
}

pub fn create_folder(root: &Path, folder_path: &Path) -> Result<(), PebbleError> {
    let path = safe_workspace_target(root, folder_path)?;
    suppress_workspace_path(&path, Duration::from_secs(1));
    fs::create_dir_all(path)?;
    Ok(())
}

pub fn rename_path(root: &Path, old_path: &Path, new_path: &Path) -> Result<(), PebbleError> {
    let old_path = safe_workspace_path(root, old_path)?;
    if old_path == canonical_existing(root)? {
        return Err(PebbleError::InvalidPath(
            "cannot rename workspace root".into(),
        ));
    }
    let new_path = safe_workspace_target(root, new_path)?;
    suppress_workspace_path(&old_path, Duration::from_secs(1));
    suppress_workspace_path(&new_path, Duration::from_secs(1));
    fs::rename(old_path, new_path)?;
    Ok(())
}

pub fn move_path(
    root: &Path,
    source_path: &Path,
    target_folder: &Path,
) -> Result<PathBuf, PebbleError> {
    let source = safe_workspace_path(root, source_path)?;
    let target_folder = safe_workspace_target(root, target_folder)?;
    fs::create_dir_all(&target_folder)?;
    let target = target_folder.join(
        source
            .file_name()
            .ok_or_else(|| PebbleError::InvalidPath("source path has no filename".into()))?,
    );
    if source == target {
        return Ok(source);
    }
    safe_workspace_target(root, &target)?;
    suppress_workspace_path(&source, Duration::from_secs(1));
    suppress_workspace_path(&target, Duration::from_secs(1));
    fs::rename(source, &target)?;
    Ok(target)
}

pub fn delete_path(root: &Path, target_path: &Path, permanent: bool) -> Result<(), PebbleError> {
    let root = canonical_existing(root)?;
    let target = safe_workspace_path(&root, target_path)?;
    if target == root {
        return Err(PebbleError::InvalidPath(
            "cannot delete workspace root".into(),
        ));
    }
    suppress_workspace_path(&target, Duration::from_secs(1));
    if permanent {
        if target.is_dir() {
            fs::remove_dir_all(target)?;
        } else {
            fs::remove_file(target)?;
        }
        return Ok(());
    }
    let trash = root.join(PEBBLE_DIR).join("trash");
    fs::create_dir_all(&trash)?;
    let name = target
        .file_name()
        .ok_or_else(|| PebbleError::InvalidPath("target path has no filename".into()))?;
    let mut destination = trash.join(name);
    if destination.exists() {
        let nonce = TEMP_ID.fetch_add(1, AtomicOrdering::Relaxed);
        destination = trash.join(format!("{}-{nonce}", name.to_string_lossy()));
    }
    fs::rename(target, destination)?;
    Ok(())
}

pub fn duplicate_request(root: &Path, request_path: &Path) -> Result<PathBuf, PebbleError> {
    let source = safe_workspace_path(root, request_path)?;
    let mut request = read_request(root, &source)?;
    let filename = source
        .file_name()
        .and_then(|name| name.to_str())
        .unwrap_or("request.pebble.json");
    let base = filename.strip_suffix(REQUEST_SUFFIX).unwrap_or(filename);
    let mut copy_number = 1;
    let destination = loop {
        let suffix = if copy_number == 1 {
            " (Copy)".to_owned()
        } else {
            format!(" (Copy {copy_number})")
        };
        let candidate = source.with_file_name(format!("{base}{suffix}{REQUEST_SUFFIX}"));
        if !candidate.exists() {
            break candidate;
        }
        copy_number += 1;
    };
    let request_name = request
        .get("name")
        .and_then(Value::as_str)
        .unwrap_or(base)
        .to_owned();
    if let Some(object) = request.as_object_mut() {
        object.insert(
            "name".into(),
            json!(if copy_number == 1 {
                format!("{request_name} (Copy)")
            } else {
                format!("{request_name} (Copy {copy_number})")
            }),
        );
        object.insert("id".into(), json!(""));
    }
    let destination = safe_workspace_target(root, &destination)?;
    write_json_atomic(&destination, &request)?;
    Ok(destination)
}

/// Resolve folder-level headers, authentication, variables and script metadata for a request.
/// The request contents remain otherwise unchanged so callers can still apply interpolation
/// at execution time.
pub fn resolve_request(
    root: &Path,
    request_path: &Path,
    _environment_name: Option<&str>,
) -> Result<Value, PebbleError> {
    let root = canonical_existing(root)?;
    let request_file = safe_workspace_path(&root, request_path)?;
    let mut request = read_request(&root, &request_file)?;
    let mut chain = Vec::new();
    let mut current = request_file
        .parent()
        .ok_or_else(|| PebbleError::InvalidPath("request path has no parent".into()))?;
    while current.starts_with(&root) {
        chain.push(current.to_path_buf());
        if current == root {
            break;
        }
        current = current
            .parent()
            .ok_or_else(|| PebbleError::InvalidPath("request path has no parent".into()))?;
    }
    chain.reverse();

    let mut headers = Vec::<Value>::new();
    let mut header_indexes = std::collections::HashMap::<String, usize>::new();
    let mut inherited_headers = serde_json::Map::new();
    let mut overridden_headers = serde_json::Map::new();
    let mut folder_auth: Option<(Value, Value)> = None;
    let mut inherited_vars = serde_json::Map::new();
    let mut resolved_variables = serde_json::Map::new();
    let mut folder_pre_scripts = Vec::new();
    let mut folder_post_scripts = Vec::new();
    let mut folder_pre_script_chain = Vec::new();
    let mut folder_post_script_chain = Vec::new();

    for directory in &chain {
        let metadata = directory.join(FOLDER_FILE);
        let folder = match fs::read(&metadata) {
            Ok(bytes) => serde_json::from_slice::<Value>(&bytes)?,
            Err(error) if error.kind() == io::ErrorKind::NotFound => continue,
            Err(error) => return Err(error.into()),
        };
        let name = directory
            .file_name()
            .and_then(|part| part.to_str())
            .unwrap_or("Folder");
        let relative = directory
            .strip_prefix(&root)
            .unwrap_or(directory)
            .to_string_lossy()
            .replace('\\', "/");
        let source = json!({ "sourceFolder": name, "sourcePath": relative });
        if let Some(items) = folder.get("headers").and_then(Value::as_array) {
            for item in items {
                let key = item.get("key").and_then(Value::as_str).unwrap_or("").trim();
                if key.is_empty() {
                    continue;
                }
                let lower = key.to_lowercase();
                let index = if let Some(index) = header_indexes.get(&lower) {
                    *index
                } else {
                    let index = headers.len();
                    header_indexes.insert(lower.clone(), index);
                    headers.push(Value::Null);
                    index
                };
                headers[index] = item.clone();
                inherited_headers.insert(lower, source.clone());
            }
        }
        if let Some(auth) = folder.get("auth") {
            let auth_type = auth.get("type").and_then(Value::as_str).unwrap_or("");
            if !auth_type.is_empty() && auth_type != "inherit" {
                folder_auth = Some((auth.clone(), source.clone()));
            }
        }
        if let Some(variables) = folder.get("variables").and_then(Value::as_array) {
            for variable in variables {
                if variable
                    .get("enabled")
                    .and_then(Value::as_bool)
                    .unwrap_or(false)
                    && let Some(key) = variable
                        .get("key")
                        .and_then(Value::as_str)
                        .filter(|key| !key.is_empty())
                {
                    inherited_vars.insert(key.to_owned(), source.clone());
                    if let Some(value) = variable.get("value") {
                        resolved_variables.insert(key.to_owned(), value.clone());
                    }
                }
            }
        }
        if let Some(script) = folder
            .pointer("/scripts/preRequest")
            .and_then(Value::as_str)
            .filter(|script| !script.trim().is_empty())
        {
            folder_pre_scripts.push(name.to_owned());
            folder_pre_script_chain
                .push(json!({ "source": name, "path": relative.clone(), "script": script }));
        }
        if let Some(script) = folder
            .pointer("/scripts/postResponse")
            .and_then(Value::as_str)
            .filter(|script| !script.trim().is_empty())
        {
            folder_post_scripts.push(name.to_owned());
            folder_post_script_chain
                .push(json!({ "source": name, "path": relative.clone(), "script": script }));
        }
    }

    if let Some(request_headers) = request.get("headers").and_then(Value::as_array).cloned() {
        for item in request_headers {
            let key = item.get("key").and_then(Value::as_str).unwrap_or("").trim();
            if key.is_empty() {
                continue;
            }
            let lower = key.to_lowercase();
            if let Some(source) = inherited_headers.remove(&lower) {
                overridden_headers.insert(lower.clone(), source);
            }
            let index = if let Some(index) = header_indexes.get(&lower) {
                *index
            } else {
                let index = headers.len();
                header_indexes.insert(lower, index);
                headers.push(Value::Null);
                index
            };
            headers[index] = item;
        }
    }
    if !headers.is_empty() {
        request["headers"] = Value::Array(headers);
    }

    let request_auth_type = request
        .pointer("/auth/type")
        .and_then(Value::as_str)
        .unwrap_or("");
    let mut inherited_auth = Value::Null;
    let mut parent_auth = Value::Null;
    let mut parent_auth_source = Value::Null;
    if let Some((auth, source)) = folder_auth {
        parent_auth = auth.clone();
        parent_auth_source = source.clone();
        if request_auth_type.is_empty() || request_auth_type == "inherit" {
            request["auth"] = auth;
            inherited_auth = source;
        }
    } else if request_auth_type.is_empty() || request_auth_type == "inherit" {
        request["auth"] = json!({ "type": "none" });
    }

    let mut pre_script_chain = folder_pre_script_chain;
    if let Some(script) = request
        .pointer("/scripts/preRequest")
        .and_then(Value::as_str)
        .filter(|script| !script.trim().is_empty())
    {
        pre_script_chain.push(json!({ "source": "request", "path": "request", "script": script }));
    }
    let mut post_script_chain = Vec::new();
    if let Some(script) = request
        .pointer("/scripts/postResponse")
        .and_then(Value::as_str)
        .filter(|script| !script.trim().is_empty())
    {
        post_script_chain.push(json!({ "source": "request", "path": "request", "script": script }));
    }
    post_script_chain.extend(folder_post_script_chain.into_iter().rev());

    Ok(json!({
        "request": request,
        "inheritedHeaders": inherited_headers,
        "overriddenHeaders": overridden_headers,
        "inheritedAuth": inherited_auth,
        "parentAuth": parent_auth,
        "parentAuthSource": parent_auth_source,
        "inheritedVars": inherited_vars,
        "resolvedVariables": resolved_variables,
        "folderPreScripts": folder_pre_scripts,
        "folderPostScripts": folder_post_scripts,
        "preScriptChain": pre_script_chain,
        "postScriptChain": post_script_chain,
    }))
}

/// Add or replace a saved response example on a request, masking secret environment values.
pub fn save_example(
    root: &Path,
    request_path: &Path,
    mut example: Value,
) -> Result<(Value, Vec<Value>), PebbleError> {
    let root = canonical_existing(root)?;
    let path = safe_workspace_path(&root, request_path)?;
    let mut request = read_request(&root, &path)?;
    if !example.is_object() {
        return Err(PebbleError::InvalidPath(
            "example must be a JSON object".into(),
        ));
    }

    let mut secret_values = Vec::new();
    let active_environment = get_workspace_info(&root)
        .ok()
        .map(|workspace| workspace.active_environment);
    if let Ok(environments) = list_environments(&root) {
        for environment in environments {
            if active_environment.as_deref().is_none_or(str::is_empty)
                || active_environment.as_deref() == Some(environment.name.as_str())
            {
                for variable in environment.variables {
                    if variable.secret && !variable.value.is_empty() {
                        secret_values.push(variable.value);
                    }
                }
            }
        }
    }
    if let Some(headers) = example.get_mut("headers").and_then(Value::as_array_mut) {
        for header in headers {
            let key = header
                .get("key")
                .and_then(Value::as_str)
                .unwrap_or("")
                .to_ascii_lowercase();
            if matches!(
                key.as_str(),
                "authorization"
                    | "proxy-authorization"
                    | "cookie"
                    | "set-cookie"
                    | "x-api-key"
                    | "x-auth-token"
            ) {
                header["value"] = json!("[REDACTED]");
            } else if let Some(value) = header.get("value").and_then(Value::as_str) {
                header["value"] = json!(mask_secrets(value, &secret_values));
            }
        }
    }
    if let Some(body) = example.get("body").and_then(Value::as_str) {
        let mut body = body.to_owned();
        if body.len() > 512 * 1024 {
            let mut end = 512 * 1024;
            while !body.is_char_boundary(end) {
                end -= 1;
            }
            body.truncate(end);
            body.push_str("\n\n[Truncated: response body exceeds 512KB limit]");
        }
        body = mask_secrets(&body, &secret_values);
        example["body"] = json!(body);
        example["size"] = json!(body.len());
    }
    if example
        .get("id")
        .and_then(Value::as_str)
        .unwrap_or("")
        .is_empty()
    {
        example["id"] = json!(format!(
            "ex_{}_{}",
            chrono::Utc::now().timestamp_millis(),
            TEMP_ID.fetch_add(1, AtomicOrdering::Relaxed)
        ));
    }
    if example
        .get("savedAt")
        .and_then(Value::as_str)
        .unwrap_or("")
        .is_empty()
    {
        example["savedAt"] = json!(chrono::Utc::now().to_rfc3339());
    }
    if example
        .get("name")
        .and_then(Value::as_str)
        .unwrap_or("")
        .is_empty()
    {
        let code = example
            .get("statusCode")
            .and_then(Value::as_i64)
            .unwrap_or_default();
        let status = example
            .get("statusText")
            .and_then(Value::as_str)
            .unwrap_or("");
        example["name"] = json!(format!("{code} {status} Example").trim());
    }

    let examples = request
        .as_object_mut()
        .ok_or_else(|| PebbleError::InvalidPath("request must be a JSON object".into()))?
        .entry("examples")
        .or_insert_with(|| json!([]));
    let examples = examples
        .as_array_mut()
        .ok_or_else(|| PebbleError::InvalidPath("request examples must be an array".into()))?;
    if let Some(index) = examples
        .iter()
        .position(|saved| saved.get("id") == example.get("id"))
    {
        examples[index] = example.clone();
    } else {
        examples.push(example.clone());
    }
    if examples.len() > 10 {
        let remove_count = examples.len() - 10;
        examples.drain(0..remove_count);
    }
    let examples = examples.clone();
    save_request(&root, &path, request)?;
    Ok((example, examples))
}

pub fn delete_example(
    root: &Path,
    request_path: &Path,
    example_id: &str,
) -> Result<Vec<Value>, PebbleError> {
    if example_id.is_empty() {
        return Err(PebbleError::InvalidPath("example id is required".into()));
    }
    let root = canonical_existing(root)?;
    let path = safe_workspace_path(&root, request_path)?;
    let mut request = read_request(&root, &path)?;
    let examples = request
        .get_mut("examples")
        .and_then(Value::as_array_mut)
        .ok_or_else(|| PebbleError::InvalidPath("request has no examples".into()))?;
    examples.retain(|example| example.get("id").and_then(Value::as_str) != Some(example_id));
    let examples = examples.clone();
    save_request(&root, &path, request)?;
    Ok(examples)
}

pub fn list_cookies(root: &Path) -> Result<Vec<Value>, PebbleError> {
    let path = root.join(PEBBLE_DIR).join("cookies.json");
    match fs::read(path) {
        Ok(bytes) => {
            let mut cookies: Vec<Value> = serde_json::from_slice(&bytes)?;
            let now = chrono::Utc::now();
            cookies.retain(|cookie| {
                cookie
                    .get("expires")
                    .and_then(Value::as_str)
                    .is_none_or(|expires| {
                        chrono::DateTime::parse_from_rfc3339(expires)
                            .map(|date| date.with_timezone(&chrono::Utc) > now)
                            .unwrap_or(true)
                    })
            });
            Ok(cookies)
        }
        Err(error) if error.kind() == io::ErrorKind::NotFound => Ok(Vec::new()),
        Err(error) => Err(error.into()),
    }
}

pub fn save_cookie(root: &Path, mut cookie: Value) -> Result<(), PebbleError> {
    let name = cookie
        .get("name")
        .and_then(Value::as_str)
        .unwrap_or("")
        .trim()
        .to_owned();
    let domain = cookie
        .get("domain")
        .and_then(Value::as_str)
        .unwrap_or("")
        .trim()
        .trim_start_matches('.')
        .to_ascii_lowercase();
    if name.is_empty() || domain.is_empty() {
        return Err(PebbleError::InvalidPath(
            "cookie name and domain are required".into(),
        ));
    }
    validate_identifier(&name)?;
    let path = cookie
        .get("path")
        .and_then(Value::as_str)
        .filter(|path| !path.is_empty())
        .unwrap_or("/")
        .to_owned();
    cookie["name"] = json!(name);
    cookie["domain"] = json!(domain);
    cookie["path"] = json!(path);
    let mut cookies = list_cookies(root)?;
    if let Some(index) = cookies.iter().position(|existing| {
        existing.get("name").and_then(Value::as_str) == Some(name.as_str())
            && existing
                .get("domain")
                .and_then(Value::as_str)
                .is_some_and(|value| value.trim_start_matches('.').eq_ignore_ascii_case(&domain))
            && existing.get("path").and_then(Value::as_str).unwrap_or("/") == path
    }) {
        cookies[index] = cookie;
    } else {
        cookies.push(cookie);
    }
    let file = root.join(PEBBLE_DIR).join("cookies.json");
    write_json_atomic_with_mode(&file, &cookies, Some(0o600))
}

pub fn delete_cookies(
    root: &Path,
    domain: Option<&str>,
    path: Option<&str>,
    name: Option<&str>,
) -> Result<Vec<Value>, PebbleError> {
    let mut cookies = list_cookies(root)?;
    let domain = domain
        .unwrap_or("")
        .trim()
        .trim_start_matches('.')
        .to_ascii_lowercase();
    let path = path.unwrap_or("/");
    let name = name.unwrap_or("");
    cookies.retain(|cookie| {
        let matches_domain = domain.is_empty()
            || cookie
                .get("domain")
                .and_then(Value::as_str)
                .is_some_and(|value| value.trim_start_matches('.').eq_ignore_ascii_case(&domain));
        let matches_name =
            name.is_empty() || cookie.get("name").and_then(Value::as_str) == Some(name);
        let matches_path = path.is_empty()
            || path == "/" && name.is_empty()
            || cookie.get("path").and_then(Value::as_str).unwrap_or("/") == path;
        !(matches_domain && matches_name && matches_path)
    });
    let file = root.join(PEBBLE_DIR).join("cookies.json");
    write_json_atomic_with_mode(&file, &cookies, Some(0o600))?;
    Ok(cookies)
}

pub fn cookies_for_request(
    root: &Path,
    host: &str,
    request_path: &str,
    secure: bool,
) -> Result<Vec<Value>, PebbleError> {
    let host = host.to_ascii_lowercase();
    let now = chrono::Utc::now();
    Ok(list_cookies(root)?
        .into_iter()
        .filter(|cookie| {
            let domain = cookie
                .get("domain")
                .and_then(Value::as_str)
                .unwrap_or("")
                .trim_start_matches('.')
                .to_ascii_lowercase();
            let cookie_path = cookie.get("path").and_then(Value::as_str).unwrap_or("/");
            let domain_matches = host == domain || host.ends_with(&format!(".{domain}"));
            let path_matches = cookie_path == "/"
                || request_path == cookie_path
                || request_path
                    .strip_prefix(cookie_path)
                    .is_some_and(|rest| cookie_path.ends_with('/') || rest.starts_with('/'));
            let secure_matches = !cookie
                .get("secure")
                .and_then(Value::as_bool)
                .unwrap_or(false)
                || secure;
            let unexpired = cookie
                .get("expires")
                .and_then(Value::as_str)
                .is_none_or(|expires| {
                    chrono::DateTime::parse_from_rfc3339(expires)
                        .map(|date| date.with_timezone(&chrono::Utc) > now)
                        .unwrap_or(true)
                });
            domain_matches && path_matches && secure_matches && unexpired
        })
        .collect())
}

fn mask_secrets(input: &str, secrets: &[String]) -> String {
    crate::security::mask_secrets(input, secrets)
}

pub fn safe_workspace_path(root: &Path, requested: &Path) -> Result<PathBuf, PebbleError> {
    if requested.as_os_str().is_empty() {
        return Err(PebbleError::InvalidPath("path cannot be empty".into()));
    }
    validate_untrusted_path(requested)?;
    let root = canonical_existing(root)?;
    let candidate = if requested.is_absolute() {
        requested.to_path_buf()
    } else {
        root.join(requested)
    };
    let candidate = canonical_existing(&candidate)?;
    if !candidate.starts_with(&root) {
        return Err(PebbleError::InvalidPath(
            "path escapes workspace root".into(),
        ));
    }
    Ok(candidate)
}

pub fn ensure_gitignore(root: &Path) -> Result<String, PebbleError> {
    let path = root.join(".gitignore");
    let current = match fs::read_to_string(&path) {
        Ok(contents) => contents,
        Err(err) if err.kind() == io::ErrorKind::NotFound => String::new(),
        Err(err) => return Err(err.into()),
    };
    if current.lines().any(gitignore_has_secret_rule) {
        return Ok("ok".into());
    }
    let mut updated = current;
    if !updated.is_empty() && !updated.ends_with('\n') {
        updated.push('\n');
    }
    updated.push_str("\n# PebblePost private environment values\n*.secret.env.json\n");
    suppress_workspace_path(&path, Duration::from_secs(1));
    fs::write(path, updated)?;
    Ok("ok".into())
}

fn gitignore_has_secret_rule(line: &str) -> bool {
    let line = line.trim();
    line == "*.secret.env.json"
        || line == "/*.secret.env.json"
        || line.ends_with("/*.secret.env.json")
}

fn safe_workspace_write_path(root: &Path, requested: &Path) -> Result<PathBuf, PebbleError> {
    validate_untrusted_path(requested)?;
    let root = canonical_existing(root)?;
    let mut candidate = if requested.is_absolute() {
        requested.to_path_buf()
    } else {
        root.join(requested)
    };
    if candidate.extension().and_then(|ext| ext.to_str()) != Some("json")
        || !candidate.to_string_lossy().ends_with(REQUEST_SUFFIX)
    {
        candidate = PathBuf::from(format!("{}.pebble.json", candidate.display()));
    }
    let parent = candidate
        .parent()
        .ok_or_else(|| PebbleError::InvalidPath("request path has no parent".into()))?;
    let parent = canonical_existing(parent)?;
    if !parent.starts_with(&root) {
        return Err(PebbleError::InvalidPath(
            "path escapes workspace root".into(),
        ));
    }
    Ok(parent.join(
        candidate
            .file_name()
            .ok_or_else(|| PebbleError::InvalidPath("request path has no filename".into()))?,
    ))
}

fn safe_workspace_target(root: &Path, requested: &Path) -> Result<PathBuf, PebbleError> {
    validate_untrusted_path(requested)?;
    let root = canonical_existing(root)?;
    if requested.as_os_str().is_empty() {
        return Err(PebbleError::InvalidPath("path cannot be empty".into()));
    }
    let candidate = if requested.is_absolute() {
        requested.to_path_buf()
    } else {
        root.join(requested)
    };
    let parent = candidate
        .parent()
        .ok_or_else(|| PebbleError::InvalidPath("path has no parent".into()))?;
    let parent = canonical_existing(parent)?;
    if !parent.starts_with(&root) {
        return Err(PebbleError::InvalidPath(
            "path escapes workspace root".into(),
        ));
    }
    let name = candidate
        .file_name()
        .ok_or_else(|| PebbleError::InvalidPath("path has no name".into()))?;
    Ok(parent.join(name))
}

fn canonical_existing(path: &Path) -> Result<PathBuf, PebbleError> {
    validate_untrusted_path(path)?;
    fs::canonicalize(path).map_err(PebbleError::from)
}

fn validate_untrusted_path(path: &Path) -> Result<(), PebbleError> {
    let value = path.to_string_lossy();
    if value.contains('\0')
        || path
            .components()
            .any(|component| matches!(component, std::path::Component::ParentDir))
    {
        return Err(PebbleError::InvalidPath(
            "path traversal or null byte detected".into(),
        ));
    }
    let normalized = value.replace('\\', "/");
    let windows_absolute = normalized.starts_with("//")
        || (normalized.as_bytes().get(1) == Some(&b':')
            && normalized
                .as_bytes()
                .first()
                .is_some_and(u8::is_ascii_alphabetic));
    if windows_absolute && !path.is_absolute() {
        return Err(PebbleError::InvalidPath(
            "absolute paths are not allowed in this context".into(),
        ));
    }
    for part in normalized.split('/') {
        if part.is_empty() || part == "." {
            continue;
        }
        let stem = part.split('.').next().unwrap_or(part).to_ascii_uppercase();
        if matches!(
            stem.as_str(),
            "CON"
                | "PRN"
                | "AUX"
                | "NUL"
                | "COM1"
                | "COM2"
                | "COM3"
                | "COM4"
                | "COM5"
                | "COM6"
                | "COM7"
                | "COM8"
                | "COM9"
                | "LPT1"
                | "LPT2"
                | "LPT3"
                | "LPT4"
                | "LPT5"
                | "LPT6"
                | "LPT7"
                | "LPT8"
                | "LPT9"
        ) {
            return Err(PebbleError::InvalidPath(format!(
                "reserved path component: {part}"
            )));
        }
    }
    Ok(())
}

fn scan_dir(dir: &Path, workspace_root: &Path) -> Result<Vec<TreeNode>, PebbleError> {
    let mut nodes = Vec::new();
    let mut item_order = Vec::new();
    let order_file = dir.join(FOLDER_FILE);
    if let Ok(data) = fs::read(order_file)
        && let Ok(value) = serde_json::from_slice::<Value>(&data)
    {
        item_order = value
            .get("itemOrder")
            .and_then(Value::as_array)
            .cloned()
            .unwrap_or_default()
            .into_iter()
            .filter_map(|value| value.as_str().map(str::to_owned))
            .collect();
    }
    for entry in fs::read_dir(dir)? {
        let entry = entry?;
        let name = entry.file_name().to_string_lossy().into_owned();
        if name.starts_with('.') || matches!(name.as_str(), "node_modules" | "dist" | FOLDER_FILE) {
            continue;
        }
        let file_type = entry.file_type()?;
        if !file_type.is_dir() && !file_type.is_file() {
            continue;
        }
        let full_path = entry.path();
        let rel_path = full_path
            .strip_prefix(workspace_root)
            .unwrap_or(&full_path)
            .to_string_lossy()
            .replace('\\', "/");
        let (prefix_order, display_name) = parse_order_prefix(&name);
        if file_type.is_dir() {
            nodes.push(TreeNode {
                id: rel_path.clone(),
                name: name.clone(),
                display_name,
                path: full_path.to_string_lossy().into_owned(),
                rel_path,
                is_dir: true,
                order: prefix_order,
                method: String::new(),
                git_status: String::new(),
                children: scan_dir(&full_path, workspace_root)?,
            });
        } else if name.ends_with(REQUEST_SUFFIX) {
            let contents = fs::read(&full_path)
                .ok()
                .and_then(|bytes| serde_json::from_slice::<Value>(&bytes).ok());
            let method = contents
                .as_ref()
                .and_then(|value| value.get("method"))
                .and_then(Value::as_str)
                .unwrap_or("GET")
                .to_owned();
            let order = contents
                .as_ref()
                .and_then(|value| value.get("order"))
                .and_then(Value::as_i64)
                .filter(|value| *value > 0)
                .unwrap_or(prefix_order);
            nodes.push(TreeNode {
                id: rel_path.clone(),
                name,
                display_name,
                path: full_path.to_string_lossy().into_owned(),
                rel_path,
                is_dir: false,
                order,
                method,
                git_status: String::new(),
                children: Vec::new(),
            });
        }
    }
    nodes.sort_by(|a, b| compare_nodes(a, b, &item_order));
    Ok(nodes)
}

fn compare_nodes(a: &TreeNode, b: &TreeNode, item_order: &[String]) -> Ordering {
    if a.is_dir != b.is_dir {
        return if a.is_dir {
            Ordering::Less
        } else {
            Ordering::Greater
        };
    }
    let index = |node: &TreeNode| {
        item_order
            .iter()
            .position(|item| item.eq_ignore_ascii_case(&node.name))
    };
    match (index(a), index(b)) {
        (Some(a), Some(b)) if a != b => return a.cmp(&b),
        (Some(_), None) => return Ordering::Less,
        (None, Some(_)) => return Ordering::Greater,
        _ => {}
    }
    match (a.order > 0, b.order > 0) {
        (true, true) if a.order != b.order => return a.order.cmp(&b.order),
        (true, false) => return Ordering::Less,
        (false, true) => return Ordering::Greater,
        _ => {}
    }
    a.display_name
        .to_lowercase()
        .cmp(&b.display_name.to_lowercase())
}

fn parse_order_prefix(name: &str) -> (i64, String) {
    let clean = name
        .strip_suffix(REQUEST_SUFFIX)
        .or_else(|| name.strip_suffix(".json"))
        .unwrap_or(name);
    if let Some((digits, rest)) = clean.split_once('-')
        && !digits.is_empty()
        && digits.chars().all(|ch| ch.is_ascii_digit())
        && let Ok(order) = digits.parse()
    {
        return (order, rest.to_owned());
    }
    (0, clean.to_owned())
}

fn write_json_atomic(path: &Path, value: &impl Serialize) -> Result<(), PebbleError> {
    write_json_atomic_with_mode(path, value, None)
}

fn write_json_atomic_with_mode(
    path: &Path,
    value: &impl Serialize,
    _mode: Option<u32>,
) -> Result<(), PebbleError> {
    suppress_workspace_path(path, Duration::from_secs(1));
    let parent = path
        .parent()
        .ok_or_else(|| PebbleError::InvalidPath("file path has no parent".into()))?;
    fs::create_dir_all(parent)?;
    let nonce = TEMP_ID.fetch_add(1, AtomicOrdering::Relaxed);
    let temp_path = parent.join(format!(".pebblepost-{}-{nonce}.tmp", std::process::id()));
    let mut data = serde_json::to_vec_pretty(value)?;
    data.push(b'\n');
    let mut options = fs::OpenOptions::new();
    options.write(true).create_new(true);
    #[cfg(unix)]
    if let Some(mode) = _mode {
        use std::os::unix::fs::OpenOptionsExt;
        options.mode(mode);
    }
    let mut temp_file = options.open(&temp_path)?;
    use std::io::Write;
    temp_file.write_all(&data)?;
    temp_file.sync_all()?;
    #[cfg(windows)]
    if path.exists() {
        let filename = path
            .file_name()
            .and_then(|name| name.to_str())
            .unwrap_or("file");
        let backup = parent.join(format!(
            ".{filename}-{}.bak",
            TEMP_ID.fetch_add(1, AtomicOrdering::Relaxed)
        ));
        fs::rename(path, &backup)?;
        if let Err(error) = fs::rename(&temp_path, path) {
            let _ = fs::rename(&backup, path);
            let _ = fs::remove_file(&temp_path);
            return Err(error.into());
        }
        let _ = fs::remove_file(backup);
        return Ok(());
    }
    if let Err(err) = fs::rename(&temp_path, path) {
        let _ = fs::remove_file(&temp_path);
        return Err(err.into());
    }
    Ok(())
}

fn merge_environment_variables(target: &mut Vec<KeyValue>, additions: Vec<KeyValue>) {
    for variable in additions {
        let key = variable.key.as_str();
        if let Some(index) = target.iter().position(|existing| existing.key == key) {
            target[index] = variable;
        } else {
            target.push(variable);
        }
    }
}

pub(crate) fn validate_identifier(name: &str) -> Result<(), PebbleError> {
    let trimmed = name.trim();
    if trimmed.is_empty()
        || trimmed != name
        || name == "."
        || name == ".."
        || name
            .chars()
            .any(|ch| ch == '/' || ch == '\\' || ch == '\0' || ch.is_control())
    {
        return Err(PebbleError::InvalidPath("invalid name".into()));
    }
    let stem = name.split('.').next().unwrap_or(name).to_ascii_uppercase();
    if [
        "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8",
        "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
    ]
    .contains(&stem.as_str())
    {
        return Err(PebbleError::InvalidPath("reserved filename".into()));
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn temp_dir(label: &str) -> PathBuf {
        let nonce = TEMP_ID.fetch_add(1, AtomicOrdering::Relaxed);
        let path =
            std::env::temp_dir().join(format!("pebblepost-{label}-{}-{nonce}", std::process::id()));
        fs::create_dir_all(&path).unwrap();
        path
    }

    #[test]
    fn folder_inheritance_resolves_headers_auth_variables_and_script_order() {
        let root = temp_dir("inheritance");
        let collection = root.join("collections/demo");
        let child = collection.join("auth");
        fs::create_dir_all(&child).unwrap();
        save_folder(&root, &collection, json!({
            "schemaVersion": 2, "name": "Demo", "auth": { "type": "basic", "username": "root" },
            "headers": [{ "key": "X-Root", "value": "root", "enabled": true }, { "key": "X-Common", "value": "root", "enabled": true }],
            "variables": [{ "key": "ENV", "value": "dev", "enabled": true }],
            "scripts": { "preRequest": "root-pre", "postResponse": "root-post" }
        })).unwrap();
        save_folder(&root, &child, json!({
            "schemaVersion": 2, "name": "Auth", "auth": { "type": "bearer", "token": "child-token" },
            "headers": [{ "key": "X-Common", "value": "child", "enabled": true }],
            "variables": [{ "key": "ENV", "value": "test", "enabled": true }, { "key": "SKIP", "value": "disabled", "enabled": false }],
            "scripts": { "preRequest": "child-pre", "postResponse": "child-post" }
        })).unwrap();
        let request_path = child.join("get.pebble.json");
        save_request(&root, &request_path, json!({
            "schemaVersion": 2, "name": "Get", "method": "GET", "url": "/", "auth": { "type": "inherit" },
            "headers": [{ "key": "x-common", "value": "request", "enabled": true }],
            "variables": [], "scripts": { "preRequest": "request-pre", "postResponse": "request-post" }
        })).unwrap();

        let resolved = resolve_request(&root, &request_path, None).unwrap();
        assert_eq!(
            resolved
                .pointer("/request/auth/type")
                .and_then(Value::as_str),
            Some("bearer")
        );
        assert_eq!(
            resolved
                .pointer("/request/auth/token")
                .and_then(Value::as_str),
            Some("child-token")
        );
        assert_eq!(
            resolved
                .pointer("/request/headers")
                .and_then(Value::as_array)
                .unwrap()
                .len(),
            2
        );
        assert_eq!(
            resolved
                .pointer("/request/headers/1/value")
                .and_then(Value::as_str),
            Some("request")
        );
        assert_eq!(
            resolved
                .pointer("/request/headers/0/key")
                .and_then(Value::as_str),
            Some("X-Root")
        );
        assert_eq!(
            resolved
                .pointer("/resolvedVariables/ENV")
                .and_then(Value::as_str),
            Some("test")
        );
        assert!(resolved.pointer("/resolvedVariables/SKIP").is_none());
        let pre = resolved["preScriptChain"]
            .as_array()
            .unwrap()
            .iter()
            .filter_map(|item| item["source"].as_str())
            .collect::<Vec<_>>();
        let post = resolved["postScriptChain"]
            .as_array()
            .unwrap()
            .iter()
            .filter_map(|item| item["source"].as_str())
            .collect::<Vec<_>>();
        assert_eq!(pre, ["demo", "auth", "request"]);
        assert_eq!(post, ["request", "auth", "demo"]);
        fs::remove_dir_all(root).unwrap();
    }

    #[test]
    fn stable_workspace_serialization_is_deterministic_newline_terminated_and_atomic() {
        let root = temp_dir("serializer");
        let path = root.join("request.pebble.json");
        let request = crate::RequestDefinition {
            schema: "https://pebblepost.dev/schemas/v1/request.json".into(),
            schema_version: 1,
            version: "1.0".into(),
            name: "Golden Request".into(),
            method: "GET".into(),
            url: "https://api.example.test/items".into(),
            auth: crate::AuthDefinition {
                r#type: "none".into(),
                ..Default::default()
            },
            body: crate::BodyDefinition {
                r#type: "none".into(),
                ..Default::default()
            },
            settings: crate::SettingDefinition {
                follow_redirects: true,
                verify_ssl: true,
                timeout_ms: 30_000,
                ..Default::default()
            },
            ..Default::default()
        };
        write_json_atomic(&path, &request).unwrap();
        let first = fs::read(&path).unwrap();
        assert_eq!(first.last(), Some(&b'\n'));
        assert!(String::from_utf8_lossy(&first).starts_with("{\n  \"$schema\": "));
        assert!(String::from_utf8_lossy(&first).contains("\n  \"schemaVersion\": 1,"));
        write_json_atomic(&path, &request).unwrap();
        assert_eq!(fs::read(&path).unwrap(), first);
        let files = fs::read_dir(&root)
            .unwrap()
            .map(|entry| entry.unwrap().file_name())
            .collect::<Vec<_>>();
        assert_eq!(files, [std::ffi::OsString::from("request.pebble.json")]);
        fs::remove_dir_all(root).unwrap();
    }

    #[test]
    fn workspace_path_guards_reject_traversal_devices_and_symlink_escapes() {
        let root = temp_dir("pathguard");
        fs::write(root.join("inside.txt"), "safe").unwrap();
        assert!(safe_workspace_path(&root, Path::new("inside.txt")).is_ok());
        for path in [
            "../secret",
            "a/../../secret",
            "NUL.txt",
            "dir/COM1/file",
            "C:\\Windows\\system32",
            "\\\\server\\share",
        ] {
            assert!(
                safe_workspace_path(&root, Path::new(path)).is_err(),
                "accepted unsafe path {path:?}"
            );
        }
        assert!(safe_workspace_path(&root, Path::new("")).is_err());
        assert!(safe_workspace_path(&root, Path::new("bad\0name")).is_err());
        assert!(validate_workspace_root(Path::new("")).is_err());
        assert!(validate_identifier("valid_name-1").is_ok());
        for name in [
            "",
            " ",
            "../escape",
            "a/b",
            "a\\b",
            ".",
            "..",
            "CON",
            "aux",
            "line\nbreak",
        ] {
            assert!(
                validate_identifier(name).is_err(),
                "accepted invalid identifier {name:?}"
            );
        }
        #[cfg(unix)]
        {
            let outside = temp_dir("outside");
            fs::write(outside.join("secret.txt"), "private").unwrap();
            std::os::unix::fs::symlink(outside.join("secret.txt"), root.join("escape-link"))
                .unwrap();
            assert!(safe_workspace_path(&root, Path::new("escape-link")).is_err());
            fs::remove_dir_all(outside).unwrap();
        }
        fs::remove_dir_all(root).unwrap();
    }

    #[test]
    fn gitignore_status_and_ensure_cover_missing_existing_and_secret_rules() {
        let root = temp_dir("gitignore");
        let scan_status = |root: &Path| scan_workspace(root).unwrap().gitignore_status;
        assert_eq!(scan_status(&root), "missing");
        fs::write(root.join(".gitignore"), "node_modules/\ndist/\n").unwrap();
        assert_eq!(scan_status(&root), "no_rule");
        ensure_gitignore(&root).unwrap();
        assert_eq!(scan_status(&root), "ok");
        let contents = fs::read_to_string(root.join(".gitignore")).unwrap();
        assert!(contents.contains("# PebblePost private environment values\n*.secret.env.json\n"));
        ensure_gitignore(&root).unwrap();
        assert_eq!(
            fs::read_to_string(root.join(".gitignore")).unwrap(),
            contents
        );
        fs::remove_dir_all(root).unwrap();
    }
}
