use std::env;
use std::fs::{self, OpenOptions};
use std::io::{Cursor, Read, Write};
use std::path::{Path, PathBuf};
use std::sync::RwLock;
use std::time::{SystemTime, UNIX_EPOCH};

use axum::Json;
use axum::extract::{Query, State};
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use base64::Engine;
use flate2::read::GzDecoder;
use ring::signature::{self, UnparsedPublicKey};
use serde::{Deserialize, Serialize};
use serde_json::json;
use tar::Archive;
use zip::ZipArchive;

const GITHUB_API_BASE: &str = "https://api.github.com";
const GITHUB_OWNER: &str = "lovanbang999";
const GITHUB_REPOSITORY: &str = "pebblepost";
const MAX_SIGNATURE_BYTES: usize = 64 * 1024;

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct UpdateInfo {
    #[serde(default)]
    current_version: String,
    #[serde(default)]
    latest_version: String,
    #[serde(default)]
    has_update: bool,
    #[serde(default)]
    release_notes: String,
    #[serde(default)]
    release_url: String,
    #[serde(default)]
    published_at: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    asset_url: String,
    #[serde(default, skip_serializing_if = "String::is_empty")]
    signature_url: String,
}

#[derive(Clone, Debug, Deserialize, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct UpdateSettings {
    #[serde(default = "default_check_on_startup")]
    check_on_startup: bool,
    #[serde(default = "default_channel")]
    channel: String,
}

impl Default for UpdateSettings {
    fn default() -> Self {
        Self {
            check_on_startup: true,
            channel: default_channel(),
        }
    }
}

#[derive(Deserialize)]
struct GitHubRelease {
    #[serde(default)]
    tag_name: String,
    #[serde(default)]
    body: String,
    #[serde(default)]
    html_url: String,
    #[serde(default)]
    published_at: String,
    #[serde(default)]
    assets: Vec<GitHubAsset>,
}

#[derive(Deserialize)]
struct GitHubAsset {
    #[serde(default)]
    name: String,
    #[serde(default)]
    browser_download_url: String,
}

#[derive(Deserialize)]
pub(super) struct CheckQuery {
    #[serde(default)]
    force: bool,
}

#[derive(Deserialize)]
pub(crate) struct VerifyPayload {
    pub(crate) message: String,
    pub(crate) signature: String,
    #[serde(default)]
    #[serde(rename = "publicKey")]
    pub(crate) public_key: String,
}

pub struct UpdateService {
    client: reqwest::Client,
    current_version: String,
    settings_path: PathBuf,
    settings: RwLock<UpdateSettings>,
    public_key: Option<Vec<u8>>,
}

impl UpdateService {
    pub fn new(data_dir: &Path) -> Self {
        let settings_path = data_dir.join("update-settings.json");
        let settings = fs::read(&settings_path)
            .ok()
            .and_then(|data| serde_json::from_slice::<UpdateSettings>(&data).ok())
            .unwrap_or_default();
        let public_key = env::var("PEBBLEPOST_UPDATE_PUBLIC_KEY")
            .ok()
            .and_then(|key| parse_public_key(key.as_bytes()).ok());
        Self {
            client: reqwest::Client::builder()
                .timeout(std::time::Duration::from_secs(10))
                .build()
                .unwrap_or_else(|_| reqwest::Client::new()),
            current_version: env!("CARGO_PKG_VERSION").to_owned(),
            settings_path,
            settings: RwLock::new(settings),
            public_key,
        }
    }

    fn settings(&self) -> Result<UpdateSettings, String> {
        self.settings
            .read()
            .map(|settings| settings.clone())
            .map_err(|_| "update settings lock was poisoned".to_owned())
    }

    fn save_settings(&self, mut settings: UpdateSettings) -> Result<UpdateSettings, String> {
        if settings.channel.is_empty() {
            settings.channel = default_channel();
        }
        let mut current = self
            .settings
            .write()
            .map_err(|_| "update settings lock was poisoned".to_owned())?;
        *current = settings.clone();
        let parent = self
            .settings_path
            .parent()
            .ok_or("update settings path has no parent")?;
        fs::create_dir_all(parent).map_err(|error| error.to_string())?;
        let contents = serde_json::to_vec_pretty(&settings).map_err(|error| error.to_string())?;
        fs::write(&self.settings_path, contents).map_err(|error| error.to_string())?;
        Ok(settings)
    }

    async fn check_for_update(&self, force: bool) -> Result<UpdateInfo, String> {
        let settings = self.settings()?;
        let current_version = self.current_version.clone();
        if !settings.check_on_startup && !force {
            return Ok(no_update(current_version));
        }
        let url =
            format!("{GITHUB_API_BASE}/repos/{GITHUB_OWNER}/{GITHUB_REPOSITORY}/releases/latest");
        let response = self
            .client
            .get(url)
            .header(reqwest::header::ACCEPT, "application/vnd.github.v3+json")
            .header(
                reqwest::header::USER_AGENT,
                format!("PebblePost-Updater/{current_version}"),
            )
            .send()
            .await
            .map_err(|error| format!("failed to query GitHub releases: {error}"))?;
        if response.status() == StatusCode::NOT_FOUND {
            return Ok(no_update(current_version));
        }
        if !response.status().is_success() {
            let status = response.status();
            let body = response.bytes().await.unwrap_or_default();
            let body = String::from_utf8_lossy(&body[..body.len().min(1024)]);
            return Err(format!("GitHub API returned status {status}: {body}"));
        }
        let release: GitHubRelease = response
            .json()
            .await
            .map_err(|error| format!("failed to decode GitHub release response: {error}"))?;
        let latest_version = release
            .tag_name
            .strip_prefix('v')
            .unwrap_or(&release.tag_name)
            .to_owned();
        let has_update = compare_semver(&latest_version, &current_version) > 0;
        let (goos, goarch) = platform_names();
        let mut info = UpdateInfo {
            current_version,
            latest_version,
            has_update,
            release_notes: release.body,
            release_url: release.html_url,
            published_at: release.published_at,
            asset_url: String::new(),
            signature_url: String::new(),
        };
        for asset in release.assets {
            let name = asset.name.to_ascii_lowercase();
            let is_signature = name.ends_with(".minisig") || name.ends_with(".sig");
            if is_signature {
                info.signature_url = asset.browser_download_url.clone();
            }
            if name.contains(goos)
                && (name.contains(goarch) || name.contains("universal") || name.contains("all"))
                && (info.asset_url.is_empty() || !is_signature)
            {
                info.asset_url = asset.browser_download_url;
            }
        }
        Ok(info)
    }

    fn verify(&self, message: &[u8], signature_bytes: &[u8], public_key: Option<&[u8]>) -> bool {
        let Some(public_key) = public_key.or(self.public_key.as_deref()) else {
            return false;
        };
        let Ok(signature) = extract_signature(signature_bytes) else {
            return false;
        };
        verify_signature(message, &signature, public_key)
    }

    async fn apply_update(&self, info: UpdateInfo) -> Result<(), String> {
        if compare_semver(&info.latest_version, &self.current_version) <= 0 {
            return Err(format!(
                "refusing to downgrade or reinstall version {} when current version is {}",
                info.latest_version, self.current_version
            ));
        }
        if !info.asset_url.to_ascii_lowercase().starts_with("https://") {
            return Err(format!(
                "insecure asset URL: HTTPS is required (got {})",
                info.asset_url
            ));
        }
        if !info.signature_url.is_empty()
            && !info
                .signature_url
                .to_ascii_lowercase()
                .starts_with("https://")
        {
            return Err(format!(
                "insecure signature URL: HTTPS is required (got {})",
                info.signature_url
            ));
        }
        let public_key = self.public_key.as_deref();
        if public_key.is_some() && info.signature_url.trim().is_empty() {
            return Err("missing signature URL: update artifact cannot be verified".into());
        }
        if public_key.is_none() && !info.signature_url.trim().is_empty() {
            return Err("trusted public key not configured: cannot verify update signature".into());
        }

        let signature_bytes = if public_key.is_some() {
            let response = self
                .client
                .get(&info.signature_url)
                .send()
                .await
                .map_err(|error| format!("failed to download signature: {error}"))?;
            if response.status() != StatusCode::OK {
                return Err(format!(
                    "signature download failed with status {}",
                    response.status()
                ));
            }
            let mut response = response;
            let mut bytes = Vec::with_capacity(MAX_SIGNATURE_BYTES);
            while let Some(chunk) = response
                .chunk()
                .await
                .map_err(|error| format!("failed to read signature: {error}"))?
            {
                let remaining = MAX_SIGNATURE_BYTES.saturating_sub(bytes.len());
                bytes.extend_from_slice(&chunk[..chunk.len().min(remaining)]);
                if bytes.len() == MAX_SIGNATURE_BYTES {
                    break;
                }
            }
            bytes
        } else {
            Vec::new()
        };
        let response = self
            .client
            .get(&info.asset_url)
            .send()
            .await
            .map_err(|error| format!("failed to download asset: {error}"))?;
        if response.status() != StatusCode::OK {
            return Err(format!(
                "asset download failed with status {}",
                response.status()
            ));
        }
        let asset = response
            .bytes()
            .await
            .map_err(|error| format!("failed to read downloaded update asset: {error}"))?;
        if asset.is_empty() {
            return Err("downloaded update asset is empty".into());
        }
        if let Some(public_key) = public_key
            && !self.verify(&asset, &signature_bytes, Some(public_key))
        {
            return Err(
                "update signature verification failed: tampered artifact or invalid signature"
                    .into(),
            );
        }
        install_binary(&asset)
    }
}

pub(super) async fn check(
    State(state): State<std::sync::Arc<super::AppState>>,
    Query(query): Query<CheckQuery>,
) -> Response {
    match state.updates.check_for_update(query.force).await {
        Ok(info) => Json(info).into_response(),
        Err(error) => (
            StatusCode::BAD_GATEWAY,
            Json(json!({ "error": error, "success": false })),
        )
            .into_response(),
    }
}

pub(super) async fn get_settings(State(state): State<std::sync::Arc<super::AppState>>) -> Response {
    match state.updates.settings() {
        Ok(settings) => Json(settings).into_response(),
        Err(error) => (
            StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({ "error": error })),
        )
            .into_response(),
    }
}

pub(super) async fn save_settings(
    State(state): State<std::sync::Arc<super::AppState>>,
    Json(settings): Json<UpdateSettings>,
) -> Response {
    match state.updates.save_settings(settings) {
        Ok(settings) => Json(json!({ "success": true, "settings": settings })).into_response(),
        Err(error) => (
            StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({ "error": format!("failed to save settings: {error}") })),
        )
            .into_response(),
    }
}

pub(super) async fn verify(
    State(state): State<std::sync::Arc<super::AppState>>,
    Json(payload): Json<VerifyPayload>,
) -> Response {
    if payload.message.trim().is_empty() || payload.signature.trim().is_empty() {
        return (StatusCode::BAD_REQUEST, Json(json!({ "valid": false, "error": "missing required fields: message and signature" }))).into_response();
    }
    let public_key = if payload.public_key.is_empty() {
        state.updates.public_key.as_deref().map(Vec::from)
    } else {
        match parse_public_key(payload.public_key.as_bytes()) {
            Ok(public_key) => Some(public_key),
            Err(error) => {
                return (
                    StatusCode::BAD_REQUEST,
                    Json(json!({ "valid": false, "error": error })),
                )
                    .into_response();
            }
        }
    };
    let Some(public_key) = public_key else {
        return (
            StatusCode::BAD_REQUEST,
            Json(json!({ "valid": false, "error": "no public key provided or configured" })),
        )
            .into_response();
    };
    Json(json!({ "valid": state.updates.verify(payload.message.as_bytes(), payload.signature.as_bytes(), Some(&public_key)) })).into_response()
}

pub(super) async fn apply(
    State(state): State<std::sync::Arc<super::AppState>>,
    Json(info): Json<UpdateInfo>,
) -> Response {
    match state.updates.apply_update(info).await {
        Ok(()) => Json(json!({ "success": true, "message": "Update applied successfully" }))
            .into_response(),
        Err(error) => (
            StatusCode::BAD_REQUEST,
            Json(json!({ "error": error, "success": false })),
        )
            .into_response(),
    }
}

fn default_check_on_startup() -> bool {
    true
}
fn default_channel() -> String {
    "stable".into()
}

fn no_update(version: String) -> UpdateInfo {
    UpdateInfo {
        current_version: version.clone(),
        latest_version: version,
        has_update: false,
        release_notes: String::new(),
        release_url: String::new(),
        published_at: "0001-01-01T00:00:00Z".into(),
        asset_url: String::new(),
        signature_url: String::new(),
    }
}

fn platform_names() -> (&'static str, &'static str) {
    let os = match env::consts::OS {
        "macos" => "darwin",
        other => other,
    };
    let arch = match env::consts::ARCH {
        "x86_64" => "amd64",
        "x86" => "386",
        "aarch64" => "arm64",
        "arm" => "arm",
        other => other,
    };
    (os, arch)
}

fn compare_semver(left: &str, right: &str) -> i8 {
    let left = semver_parts(left);
    let right = semver_parts(right);
    for index in 0..3 {
        if left[index] > right[index] {
            return 1;
        }
        if left[index] < right[index] {
            return -1;
        }
    }
    0
}

fn semver_parts(version: &str) -> [i64; 3] {
    let version = version.trim().strip_prefix('v').unwrap_or(version.trim());
    let version = version.split(['-', '+']).next().unwrap_or(version);
    let mut parts = [0; 3];
    for (index, part) in version.split('.').take(3).enumerate() {
        parts[index] = part.parse().unwrap_or(0);
    }
    parts
}

fn extract_signature(data: &[u8]) -> Result<Vec<u8>, String> {
    let text = String::from_utf8_lossy(data);
    let trimmed = text.trim();
    let lines = trimmed.lines().collect::<Vec<_>>();
    if lines.len() >= 2
        && lines[0]
            .trim()
            .to_ascii_lowercase()
            .starts_with("untrusted comment")
    {
        return signature_payload(lines[1].trim());
    }
    if let Ok(decoded) = base64::engine::general_purpose::STANDARD.decode(trimmed)
        && decoded.len() == 64
    {
        return Ok(decoded);
    }
    if data.len() == 64 {
        return Ok(data.to_vec());
    }
    Err("unrecognized signature format".into())
}

fn signature_payload(encoded: &str) -> Result<Vec<u8>, String> {
    let decoded = base64::engine::general_purpose::STANDARD
        .decode(encoded)
        .map_err(|error| format!("failed to decode minisign base64 signature: {error}"))?;
    match decoded.len() {
        74 => Ok(decoded[10..].to_vec()),
        64 => Ok(decoded),
        length => Err(format!(
            "invalid minisign signature payload length: {length}"
        )),
    }
}

fn parse_public_key(data: &[u8]) -> Result<Vec<u8>, String> {
    let text = String::from_utf8_lossy(data);
    let trimmed = text.trim();
    let lines = trimmed.lines().collect::<Vec<_>>();
    let encoded = if lines.len() >= 2
        && lines[0]
            .trim()
            .to_ascii_lowercase()
            .starts_with("untrusted comment")
    {
        lines[1].trim()
    } else {
        trimmed
    };
    if let Ok(decoded) = base64::engine::general_purpose::STANDARD.decode(encoded) {
        match decoded.len() {
            32 => return Ok(decoded),
            42 => return Ok(decoded[10..].to_vec()),
            _ => {}
        }
    }
    if data.len() == 32 {
        return Ok(data.to_vec());
    }
    Err("invalid public key format: expected 32-byte Ed25519 key or Minisign public key".into())
}

fn verify_signature(message: &[u8], signature_bytes: &[u8], public_key: &[u8]) -> bool {
    if signature_bytes.len() != 64 || public_key.len() != 32 {
        return false;
    }
    UnparsedPublicKey::new(&signature::ED25519, public_key)
        .verify(message, signature_bytes)
        .is_ok()
}

fn install_binary(asset: &[u8]) -> Result<(), String> {
    let nonce = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_err(|error| error.to_string())?
        .as_nanos();
    let directory =
        env::temp_dir().join(format!("pebblepost-update-{}-{nonce}", std::process::id()));
    fs::create_dir(&directory)
        .map_err(|error| format!("failed to create temp directory: {error}"))?;
    let guard = TemporaryUpdateDirectory(directory.clone());
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(&directory, fs::Permissions::from_mode(0o700))
            .map_err(|error| error.to_string())?;
    }
    let downloaded = directory.join("update-binary.tmp");
    write_private_file(&downloaded, asset, 0o600)?;
    let binary = extract_binary_if_archive(asset)?;
    let extracted = directory.join("update-binary.extracted");
    write_private_file(&extracted, &binary, 0o755)?;
    let target = env::current_exe()
        .map_err(|error| format!("failed to resolve current executable path: {error}"))?;
    let parent = target
        .parent()
        .ok_or("current executable has no parent directory")?;
    fs::create_dir_all(parent)
        .map_err(|error| format!("failed to create target binary parent directory: {error}"))?;
    if let Err(rename_error) = fs::rename(&extracted, &target) {
        fs::copy(&extracted, &target).map_err(|error| {
            format!("failed to replace binary: {error} (rename: {rename_error})")
        })?;
    }
    drop(guard);
    Ok(())
}

struct TemporaryUpdateDirectory(PathBuf);

impl Drop for TemporaryUpdateDirectory {
    fn drop(&mut self) {
        let _ = fs::remove_dir_all(&self.0);
    }
}

fn write_private_file(path: &Path, contents: &[u8], mode: u32) -> Result<(), String> {
    let mut options = OpenOptions::new();
    options.write(true).create_new(true);
    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt;
        options.mode(mode);
    }
    #[cfg(not(unix))]
    let _ = mode;
    let mut file = options.open(path).map_err(|error| error.to_string())?;
    file.write_all(contents)
        .map_err(|error| error.to_string())?;
    file.sync_all().map_err(|error| error.to_string())?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(path, fs::Permissions::from_mode(mode))
            .map_err(|error| error.to_string())?;
    }
    Ok(())
}

fn extract_binary_if_archive(asset: &[u8]) -> Result<Vec<u8>, String> {
    if asset.starts_with(&[0x50, 0x4b, 0x03, 0x04]) {
        let mut archive = ZipArchive::new(Cursor::new(asset))
            .map_err(|error| format!("failed to read zip archive: {error}"))?;
        let mut candidate = None;
        let mut largest = None::<(usize, u64)>;
        for index in 0..archive.len() {
            let file = archive
                .by_index(index)
                .map_err(|error| format!("failed to inspect zip archive: {error}"))?;
            if file.is_dir() {
                continue;
            }
            let name_cow = file.name().unwrap_or_default();
            let name: &str = &name_cow;
            let basename = archive_basename(name);
            if is_update_executable(basename) {
                candidate = Some(index);
                break;
            }
            if !basename.starts_with('.') && largest.is_none_or(|(_, size)| file.size() > size) {
                largest = Some((index, file.size()));
            }
        }
        let selected = candidate
            .or_else(|| largest.map(|(index, _)| index))
            .ok_or_else(|| "no executable found in zip archive".to_owned())?;
        let mut file = archive
            .by_index(selected)
            .map_err(|error| format!("failed to open file inside zip: {error}"))?;
        let mut bytes = Vec::new();
        file.read_to_end(&mut bytes)
            .map_err(|error| format!("failed to extract file from zip: {error}"))?;
        return Ok(bytes);
    }
    if asset.starts_with(&[0x1f, 0x8b]) {
        let decoder = GzDecoder::new(Cursor::new(asset));
        let mut archive = Archive::new(decoder);
        let entries = archive
            .entries()
            .map_err(|error| format!("failed to read tar.gz archive: {error}"))?;
        let mut best = Vec::new();
        let mut best_size = 0_u64;
        let mut found_exact = false;
        for entry in entries {
            let mut entry = entry.map_err(|error| format!("failed reading tar header: {error}"))?;
            if !entry.header().entry_type().is_file() {
                continue;
            }
            let name = entry
                .path()
                .map_err(|error| error.to_string())?
                .to_string_lossy()
                .into_owned();
            let basename = archive_basename(&name);
            let size = entry.size();
            let exact = is_update_executable(basename);
            if exact || (!found_exact && (best.is_empty() || size > best_size)) {
                let mut bytes = Vec::new();
                entry
                    .read_to_end(&mut bytes)
                    .map_err(|error| format!("failed reading file {name} in tar: {error}"))?;
                best_size = size;
                best = bytes;
                if exact {
                    found_exact = true;
                }
            }
        }
        if best.is_empty() {
            return Err("no regular file found in tar.gz archive".into());
        }
        return Ok(best);
    }
    Ok(asset.to_vec())
}

fn archive_basename(path: &str) -> &str {
    path.rsplit(['/', '\\']).next().unwrap_or(path)
}

fn is_update_executable(name: &str) -> bool {
    name.eq_ignore_ascii_case("pebblepost") || name.eq_ignore_ascii_case("pebblepost.exe")
}
