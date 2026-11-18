use std::env;
use std::fs;
use std::path::{Path, PathBuf};
use std::sync::Mutex;

use crate::{EnvironmentDefinition, PebbleError, get_workspace_info, save_environment};

static KEYCHAIN_INDEX_LOCK: Mutex<()> = Mutex::new(());
static KEYCHAIN_OPERATION_LOCK: Mutex<()> = Mutex::new(());

#[derive(Default, serde::Deserialize, serde::Serialize)]
struct KeychainIndex {
    #[serde(default)]
    services: std::collections::BTreeMap<String, Vec<String>>,
}

#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum SecretBackend {
    File,
    Keychain,
    Environment,
}

impl SecretBackend {
    pub fn parse(value: &str) -> Result<Self, String> {
        match value.trim().to_ascii_lowercase().as_str() {
            "" | "file" => Ok(Self::File),
            "keychain" => Ok(Self::Keychain),
            "env" => Ok(Self::Environment),
            other => Err(format!(
                "unknown secret backend {other:?}; use file, keychain, or env"
            )),
        }
    }

    pub fn as_str(self) -> &'static str {
        match self {
            Self::File => "file",
            Self::Keychain => "keychain",
            Self::Environment => "env",
        }
    }
}

pub fn configured_backend(
    root: &Path,
    environment: &str,
    override_backend: Option<&str>,
) -> Result<SecretBackend, String> {
    let environment_override = env::var("PEBBLEPOST_SECRET_BACKEND").ok();
    if let Some(backend) = override_backend
        .filter(|backend| !backend.trim().is_empty())
        .or_else(|| {
            environment_override
                .as_deref()
                .filter(|backend| !backend.trim().is_empty())
        })
    {
        return SecretBackend::parse(backend);
    }
    let config = get_workspace_info(root)
        .ok()
        .map(|workspace| workspace.secret_backend);
    let per_environment = config
        .as_ref()
        .and_then(|config| config.per_env.get(environment))
        .map(String::as_str)
        .filter(|value| !value.is_empty());
    let selected = per_environment
        .or_else(|| {
            config
                .as_ref()
                .map(|config| config.default.as_str())
                .filter(|value| !value.is_empty())
        })
        .unwrap_or("file");
    SecretBackend::parse(selected)
}

pub fn list(root: &Path, environment: &str, backend: SecretBackend) -> Result<Vec<String>, String> {
    match backend {
        SecretBackend::File => read_file_environment(root, environment).map(|record| {
            record
                .variables
                .iter()
                .map(|variable| variable.key.clone())
                .collect()
        }),
        SecretBackend::Environment => {
            Err("the env secret backend is read-only and does not support listing keys".into())
        }
        SecretBackend::Keychain => keychain_list(root, environment),
    }
}

pub fn get(
    root: &Path,
    environment: &str,
    key: &str,
    backend: SecretBackend,
) -> Result<String, String> {
    match backend {
        SecretBackend::File => {
            let record = read_file_environment(root, environment)?;
            record
                .variables
                .iter()
                .find(|variable| variable.key == key)
                .map(|variable| variable.value.clone())
                .ok_or_else(|| format!("secret not found: {environment}/{key}"))
        }
        SecretBackend::Environment => {
            env::var(key).map_err(|_| format!("secret not found: OS env {key:?} is not set"))
        }
        SecretBackend::Keychain => keychain_get(root, environment, key),
    }
}

pub fn set(
    root: &Path,
    environment: &str,
    key: &str,
    value: &str,
    backend: SecretBackend,
) -> Result<(), String> {
    match backend {
        SecretBackend::File => {
            if key.is_empty() {
                return Err("secret key cannot be empty".into());
            }
            let root = fs::canonicalize(root)
                .map_err(|error| format!("could not access workspace: {error}"))?;
            let _ = secret_directory(&root, true)?;
            let mut record = read_file_environment(&root, environment)?;
            if let Some(variable) = record
                .variables
                .iter_mut()
                .find(|variable| variable.key == key)
            {
                variable.value = value.to_owned();
                variable.secret = true;
            } else {
                record.variables.push(crate::models::KeyValue {
                    key: key.to_owned(),
                    value: value.to_owned(),
                    enabled: true,
                    secret: true,
                    ..crate::models::KeyValue::default()
                });
            }
            save_environment(&root, record, true).map_err(|error| error.to_string())
        }
        SecretBackend::Environment => Err("the env secret backend is read-only".into()),
        SecretBackend::Keychain => keychain_set(root, environment, key, value),
    }
}

pub fn delete(
    root: &Path,
    environment: &str,
    key: &str,
    backend: SecretBackend,
) -> Result<(), String> {
    match backend {
        SecretBackend::File => {
            let root = fs::canonicalize(root)
                .map_err(|error| format!("could not access workspace: {error}"))?;
            let mut record = read_file_environment(&root, environment)?;
            let old_len = record.variables.len();
            record.variables.retain(|variable| variable.key != key);
            if record.variables.len() == old_len {
                return Err(format!("secret not found: {environment}/{key}"));
            }
            if record.variables.is_empty() {
                let path = secret_file(&root, environment, false)?
                    .ok_or_else(|| format!("secret not found: {environment}/{key}"))?;
                fs::remove_file(path).map_err(|error| error.to_string())
            } else {
                save_environment(&root, record, true).map_err(|error| error.to_string())
            }
        }
        SecretBackend::Environment => Err("the env secret backend is read-only".into()),
        SecretBackend::Keychain => keychain_delete(root, environment, key),
    }
}

pub fn rollback(
    root: &Path,
    environment: &str,
    backup: &Path,
    destination: SecretBackend,
) -> Result<PathBuf, String> {
    let bytes = fs::read(backup).map_err(|error| format!("read backup: {error}"))?;
    let backup_environment: EnvironmentDefinition = serde_json::from_slice(&bytes)
        .map_err(|error| format!("parse backup secret environment: {error}"))?;
    let restored = restore_backup(backup)?;
    if destination == SecretBackend::Keychain {
        for key in backup_environment
            .variables
            .iter()
            .map(|variable| variable.key.as_str())
        {
            let _ = delete(root, environment, key, destination);
        }
    }
    Ok(restored)
}

pub fn backup_file(root: &Path, environment: &str) -> Result<Option<PathBuf>, String> {
    let Some(path) = secret_file(root, environment, false)? else {
        return Ok(None);
    };
    if !path.exists() {
        return Ok(None);
    }
    let stamp = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map_err(|error| error.to_string())?
        .as_secs();
    let backup = PathBuf::from(format!("{}.bak.{stamp}", path.display()));
    fs::copy(&path, &backup).map_err(|error| error.to_string())?;
    set_private_permissions(&backup)?;
    Ok(Some(backup))
}

pub fn restore_backup(backup: &Path) -> Result<PathBuf, String> {
    let backup = fs::canonicalize(backup).map_err(|error| format!("read backup: {error}"))?;
    let name = backup
        .file_name()
        .and_then(|name| name.to_str())
        .ok_or("backup path has no filename")?;
    let marker = name
        .rfind(".bak.")
        .ok_or("backup filename must end with .bak.<timestamp>")?;
    let destination_name = &name[..marker];
    if destination_name.is_empty() {
        return Err("backup filename has no original target".into());
    }
    let destination = backup.with_file_name(destination_name);
    let parent = destination
        .parent()
        .ok_or("backup destination has no parent directory")?;
    if !parent.is_dir() {
        return Err("backup destination directory does not exist".into());
    }
    let bytes = fs::read(&backup).map_err(|error| format!("read backup: {error}"))?;
    write_private_atomic(&destination, &bytes)?;
    Ok(destination)
}

fn read_file_environment(root: &Path, environment: &str) -> Result<EnvironmentDefinition, String> {
    validate_name(environment).map_err(|error| error.to_string())?;
    let Some(path) = secret_file(root, environment, false)? else {
        return Ok(empty_record(environment));
    };
    match fs::read(path) {
        Ok(bytes) => serde_json::from_slice(&bytes).map_err(|error| {
            format!("could not parse secret environment {environment:?}: {error}")
        }),
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(empty_record(environment)),
        Err(error) => Err(error.to_string()),
    }
}

fn empty_record(environment: &str) -> EnvironmentDefinition {
    EnvironmentDefinition {
        schema_version: 1,
        name: environment.to_owned(),
        variables: Vec::new(),
    }
}

fn secret_file(
    root: &Path,
    environment: &str,
    create_directory: bool,
) -> Result<Option<PathBuf>, String> {
    validate_name(environment).map_err(|error| error.to_string())?;
    let Some(directory) = secret_directory(root, create_directory)? else {
        return Ok(None);
    };
    let path = directory.join(format!("{environment}.secret.env.json"));
    if path.exists() {
        let canonical = fs::canonicalize(&path).map_err(|error| error.to_string())?;
        let canonical_root = fs::canonicalize(root).map_err(|error| error.to_string())?;
        if !canonical.starts_with(canonical_root) {
            return Err("secret file escapes the workspace".into());
        }
        Ok(Some(canonical))
    } else {
        Ok(Some(path))
    }
}

fn secret_directory(root: &Path, create: bool) -> Result<Option<PathBuf>, String> {
    let root =
        fs::canonicalize(root).map_err(|error| format!("could not access workspace: {error}"))?;
    if !root.is_dir() {
        return Err("workspace path is not a directory".into());
    }
    let directory = root.join(".pebble").join("environments");
    if create {
        fs::create_dir_all(&directory).map_err(|error| error.to_string())?;
    }
    if !directory.exists() {
        return Ok(None);
    }
    let canonical = fs::canonicalize(directory).map_err(|error| error.to_string())?;
    if !canonical.starts_with(&root) {
        return Err("secret environment directory escapes the workspace".into());
    }
    Ok(Some(canonical))
}

fn validate_name(environment: &str) -> Result<(), PebbleError> {
    crate::workspace::validate_identifier(environment)
}

fn keychain_entry(root: &Path, environment: &str, key: &str) -> Result<keyring::Entry, String> {
    validate_name(environment).map_err(|error| error.to_string())?;
    if key.is_empty() {
        return Err("secret key cannot be empty".into());
    }
    let root =
        fs::canonicalize(root).map_err(|error| format!("could not access workspace: {error}"))?;
    let workspace = get_workspace_info(&root).map_err(|error| error.to_string())?;
    let service = format!("pebblepost/{}/{}", workspace.name, environment);
    keyring::Entry::new(&service, key)
        .map_err(|error| format!("open OS keychain entry {environment}/{key}: {error}"))
}

fn keychain_list(_root: &Path, environment: &str) -> Result<Vec<String>, String> {
    validate_name(environment).map_err(|error| error.to_string())?;
    let _guard = KEYCHAIN_INDEX_LOCK
        .lock()
        .map_err(|_| "keychain index lock was poisoned".to_owned())?;
    let path = keychain_index_path()?;
    let index = load_keychain_index(&path)?;
    Ok(index.services.get(environment).cloned().unwrap_or_default())
}

fn keychain_get(root: &Path, environment: &str, key: &str) -> Result<String, String> {
    let _guard = KEYCHAIN_OPERATION_LOCK
        .lock()
        .map_err(|_| "keychain operation lock was poisoned".to_owned())?;
    let entry = keychain_entry(root, environment, key)?;
    entry.get_password().map_err(|error| match error {
        keyring::Error::NoEntry => format!("secret not found: {environment}/{key}"),
        error => format!("keychain get {environment}/{key}: {error}"),
    })
}

fn keychain_set(root: &Path, environment: &str, key: &str, value: &str) -> Result<(), String> {
    let _guard = KEYCHAIN_OPERATION_LOCK
        .lock()
        .map_err(|_| "keychain operation lock was poisoned".to_owned())?;
    let entry = keychain_entry(root, environment, key)?;
    entry
        .set_password(value)
        .map_err(|error| format!("keychain set {environment}/{key}: {error}"))?;
    update_keychain_index(environment, key, true)
}

fn keychain_delete(root: &Path, environment: &str, key: &str) -> Result<(), String> {
    let _guard = KEYCHAIN_OPERATION_LOCK
        .lock()
        .map_err(|_| "keychain operation lock was poisoned".to_owned())?;
    let entry = keychain_entry(root, environment, key)?;
    entry.delete_credential().map_err(|error| match error {
        keyring::Error::NoEntry => format!("secret not found: {environment}/{key}"),
        error => format!("keychain delete {environment}/{key}: {error}"),
    })?;
    update_keychain_index(environment, key, false)
}

fn update_keychain_index(environment: &str, key: &str, add: bool) -> Result<(), String> {
    let _guard = KEYCHAIN_INDEX_LOCK
        .lock()
        .map_err(|_| "keychain index lock was poisoned".to_owned())?;
    let path = keychain_index_path()?;
    let mut index = load_keychain_index(&path)?;
    let keys = index.services.entry(environment.to_owned()).or_default();
    if add {
        if !keys.iter().any(|existing| existing == key) {
            keys.push(key.to_owned());
        }
    } else {
        keys.retain(|existing| existing != key);
    }
    save_keychain_index(&path, &index)
}

fn load_keychain_index(path: &Path) -> Result<KeychainIndex, String> {
    match fs::read(path) {
        Ok(bytes) => {
            serde_json::from_slice(&bytes).map_err(|error| format!("parse keychain index: {error}"))
        }
        Err(error) if error.kind() == std::io::ErrorKind::NotFound => Ok(KeychainIndex::default()),
        Err(error) => Err(format!("read keychain index: {error}")),
    }
}

fn save_keychain_index(path: &Path, index: &KeychainIndex) -> Result<(), String> {
    let parent = path
        .parent()
        .ok_or("keychain index path has no parent directory")?;
    fs::create_dir_all(parent)
        .map_err(|error| format!("create keychain config directory: {error}"))?;
    set_private_directory_permissions(parent)?;
    let bytes = serde_json::to_vec_pretty(index).map_err(|error| error.to_string())?;
    write_private_atomic(path, &bytes).map_err(|error| format!("write keychain index: {error}"))
}

fn keychain_index_path() -> Result<PathBuf, String> {
    let config_dir = if cfg!(target_os = "windows") {
        env::var_os("APPDATA")
            .map(PathBuf::from)
            .or_else(|| {
                env::var_os("USERPROFILE")
                    .map(|path| PathBuf::from(path).join("AppData").join("Roaming"))
            })
            .ok_or("could not determine user config directory")?
    } else if cfg!(target_os = "macos") {
        env::var_os("HOME")
            .map(|path| {
                PathBuf::from(path)
                    .join("Library")
                    .join("Application Support")
            })
            .ok_or("could not determine user config directory")?
    } else if let Some(path) = env::var_os("XDG_CONFIG_HOME").filter(|path| !path.is_empty()) {
        PathBuf::from(path)
    } else {
        env::var_os("HOME")
            .map(|path| PathBuf::from(path).join(".config"))
            .ok_or("could not determine user config directory")?
    };
    Ok(config_dir.join("pebblepost").join("keychain-index.json"))
}

fn set_private_permissions(_path: &Path) -> Result<(), String> {
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(_path, fs::Permissions::from_mode(0o600))
            .map_err(|error| error.to_string())?;
    }
    Ok(())
}

fn set_private_directory_permissions(_path: &Path) -> Result<(), String> {
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(_path, fs::Permissions::from_mode(0o700))
            .map_err(|error| error.to_string())?;
    }
    Ok(())
}

fn write_private_atomic(path: &Path, bytes: &[u8]) -> Result<(), String> {
    let parent = path.parent().ok_or("secret path has no parent")?;
    let nonce = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map_err(|error| error.to_string())?
        .as_nanos();
    let temporary = parent.join(format!(".pebble-secret-{}-{nonce}.tmp", std::process::id()));
    let mut options = fs::OpenOptions::new();
    options.write(true).create_new(true);
    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt;
        options.mode(0o600);
    }
    use std::io::Write;
    let mut file = options
        .open(&temporary)
        .map_err(|error| error.to_string())?;
    if let Err(error) = file.write_all(bytes).and_then(|()| file.sync_all()) {
        let _ = fs::remove_file(&temporary);
        return Err(error.to_string());
    }
    drop(file);
    #[cfg(windows)]
    if path.exists() {
        let backup = parent.join(format!(
            ".pebble-restore-{}-{nonce}.bak",
            std::process::id()
        ));
        fs::rename(path, &backup).map_err(|error| error.to_string())?;
        if let Err(error) = fs::rename(&temporary, path) {
            let _ = fs::rename(&backup, path);
            let _ = fs::remove_file(&temporary);
            return Err(error.to_string());
        }
        let _ = fs::remove_file(backup);
        return Ok(());
    }
    if let Err(error) = fs::rename(&temporary, path) {
        let _ = fs::remove_file(&temporary);
        return Err(error.to_string());
    }
    Ok(())
}
