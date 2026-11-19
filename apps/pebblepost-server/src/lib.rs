use std::collections::{BTreeMap, HashMap, VecDeque};
use std::convert::Infallible;
use std::fs;
use std::io::{Read, Seek, SeekFrom, Write};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering as AtomicOrdering};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant, UNIX_EPOCH};
use tokio::process::Command;
use tokio::sync::oneshot;

use axum::extract::{DefaultBodyLimit, Path as AxumPath, Query, State};
use axum::http::{HeaderMap, HeaderValue, Method, Request, StatusCode, Uri, header};
use axum::middleware::{self, Next};
use axum::response::sse::{Event, KeepAlive};
use axum::response::{IntoResponse, Response, Sse};
use axum::routing::{get, post};
use axum::{Json, Router};
use futures_util::stream;
use pebblepost_core::{
    EnvironmentDefinition, PebbleError, SecretBackendConfig, build_variable_map,
    build_variable_map_with_folder, cookies_for_request, create_folder, delete_cookies,
    delete_environment, delete_example, delete_path, duplicate_request, ensure_gitignore,
    get_workspace_info, init_workspace, interpolate_value, list_cookies, list_environments,
    move_path, read_folder, read_request, rename_path, resolve_request, safe_workspace_path,
    save_cookie, save_environment, save_example, save_folder, save_request, save_workspace_info,
    scan_workspace, validate_workspace_root,
};
use serde::Deserialize;
use serde_json::{Value, json};
use tower_http::cors::{AllowOrigin, CorsLayer};

pub mod docs;
mod history;
pub mod import_export;
mod mock_api;
mod sync_api;
pub use mock_api::LocalMock;
mod grpc_api;
mod http_auth;
mod oauth2_api;
mod scripting;
mod stream_api;
mod update_api;
use history::HistoryStore;

#[derive(Clone)]
pub(crate) struct AppState {
    token: String,
    frontend_dir: PathBuf,
    history: Arc<HistoryStore>,
    active_runs: Arc<Mutex<HashMap<String, oneshot::Sender<()>>>>,
    mock_manager: Arc<mock_api::MockManager>,
    stream_manager: Arc<stream_api::StreamManager>,
    grpc_manager: Arc<grpc_api::GrpcManager>,
    pub(crate) updates: Arc<update_api::UpdateService>,
}
#[derive(Deserialize)]
struct PathQuery {
    path: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct DocsQuery {
    workspace_path: Option<String>,
    path: Option<String>,
    #[serde(default = "default_docs_format")]
    format: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct ImportPayload {
    #[serde(default)]
    source: String,
    content: String,
    #[serde(default)]
    save: bool,
    out_dir: Option<String>,
    workspace: Option<String>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct ExportPayload {
    format: String,
    collection_name: Option<String>,
    folder_path: Option<String>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct ScriptValidationPayload {
    source: Option<String>,
    #[serde(default)]
    script: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct RunnerPayload {
    run_id: Option<String>,
    stream: Option<bool>,
    #[serde(default)]
    workspace_path: String,
    target_path: Option<String>,
    #[serde(default)]
    request_paths: Vec<String>,
    #[serde(alias = "requestPattern", alias = "nameFilter")]
    request_filter: Option<String>,
    #[serde(alias = "folderPattern")]
    folder_filter: Option<String>,
    #[serde(alias = "tagPattern")]
    tag_filter: Option<String>,
    environment_name: Option<String>,
    dry_run: Option<bool>,
    data_file: Option<String>,
    secret_backend: Option<String>,
    iterations: Option<usize>,
    delay_ms: Option<u64>,
    bail: Option<bool>,
    trusted: Option<bool>,
    timeout_ms: Option<u64>,
    retry_count: Option<usize>,
    max_executions: Option<usize>,
    #[serde(default)]
    base_vars: HashMap<String, String>,
    #[serde(default)]
    extra_vars: HashMap<String, String>,
    data_rows: Option<Vec<Value>>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct RunnerStopPayload {
    run_id: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct RunnerDataPayload {
    filename: String,
    content: String,
}

fn default_docs_format() -> String {
    "data".into()
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct WorkspaceEventsQuery {
    workspace_path: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct GitQuery {
    workspace_path: String,
    path: Option<String>,
    file: Option<String>,
    limit: Option<usize>,
}

#[derive(Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ExecutePayload {
    workspace_path: Option<String>,
    environment_name: Option<String>,
    secret_backend: Option<String>,
    path: Option<String>,
    request: Value,
    trusted: Option<bool>,
    timeout_ms: Option<u64>,
    #[serde(default)]
    overrides: HashMap<String, String>,
    iteration: Option<usize>,
    iteration_count: Option<usize>,
    data_row: Option<Value>,
}

#[derive(Deserialize)]
struct ResponseBodyQuery {
    file: String,
    offset: Option<u64>,
    limit: Option<usize>,
}

#[derive(Deserialize)]
struct HistoryQuery {
    workspace: String,
    page: Option<String>,
    limit: Option<String>,
    status: Option<String>,
    method: Option<String>,
    search: Option<String>,
    since: Option<String>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct CookieQuery {
    workspace_path: String,
    domain: Option<String>,
    path: Option<String>,
    name: Option<String>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct CookiePayload {
    workspace_path: String,
    cookie: Value,
}

static TEMP_BODY_ID: AtomicU64 = AtomicU64::new(0);

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct RequestQuery {
    path: String,
    workspace_path: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct FolderQuery {
    path: String,
    workspace_path: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct EnvironmentQuery {
    workspace_path: String,
    name: Option<String>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct SecretBackendQuery {
    workspace_path: String,
}

#[derive(Deserialize)]
struct InitPayload {
    path: String,
    name: Option<String>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct WorkspacePathPayload {
    workspace_path: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct SaveRequestPayload {
    workspace_path: String,
    path: String,
    request: Value,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct FolderPayload {
    workspace_path: String,
    path: String,
    folder: Value,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct PathPayload {
    workspace_path: String,
    path: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct RenamePayload {
    workspace_path: String,
    old_path: String,
    new_path: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct MovePayload {
    workspace_path: String,
    source_path: String,
    target_folder: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct DeletePayload {
    workspace_path: String,
    path: String,
    #[serde(default)]
    permanent: bool,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct SaveEnvironmentPayload {
    workspace_path: String,
    environment: EnvironmentDefinition,
    #[serde(default)]
    is_secret: bool,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct InterpolatePayload {
    workspace_path: String,
    environment_name: Option<String>,
    request: Value,
    #[serde(default)]
    overrides: HashMap<String, String>,
}

pub fn router(
    token: String,
    frontend_dir: PathBuf,
    data_dir: PathBuf,
) -> Result<Router, std::io::Error> {
    let history = Arc::new(HistoryStore::open(&data_dir)?);
    let state = Arc::new(AppState {
        token,
        frontend_dir,
        history,
        active_runs: Arc::new(Mutex::new(HashMap::new())),
        mock_manager: Arc::new(mock_api::MockManager::default()),
        stream_manager: Arc::new(stream_api::StreamManager::default()),
        grpc_manager: Arc::new(grpc_api::GrpcManager::default()),
        updates: Arc::new(update_api::UpdateService::new(&data_dir)),
    });
    let cors = CorsLayer::new()
        .allow_origin(AllowOrigin::predicate(|origin: &HeaderValue, _| {
            origin.to_str().ok().is_some_and(is_local_origin)
        }))
        .allow_methods([
            Method::GET,
            Method::POST,
            Method::PUT,
            Method::DELETE,
            Method::OPTIONS,
        ])
        .allow_headers([header::AUTHORIZATION, header::CONTENT_TYPE]);
    Ok(Router::new()
        .route("/api/health", get(health))
        .route("/health", get(health))
        .route("/api/workspace/scan", get(scan))
        .route("/api/workspace/events", get(workspace_events))
        .route("/api/workspace/info", get(workspace_info))
        .route("/api/workspace/init", post(workspace_init))
        .route("/api/workspace/gitignore/ensure", post(gitignore_ensure))
        .route("/api/workspace/folder", post(create_workspace_folder))
        .route("/api/workspace/rename", post(rename_workspace_path))
        .route("/api/workspace/move", post(move_workspace_path))
        .route(
            "/api/workspace/duplicate",
            post(duplicate_workspace_request),
        )
        .route("/api/folder", get(get_folder))
        .route("/api/folder/save", post(put_folder))
        .route("/api/request", get(get_request))
        .route("/api/request/resolved", get(get_resolved_request))
        .route("/api/request/save", post(put_request))
        .route("/api/request/delete", post(remove_request))
        .route("/api/request/example/save", post(put_example))
        .route("/api/request/example/delete", post(remove_example))
        .route("/api/request/interpolate", post(interpolate_request))
        .route("/api/request/execute", post(execute_request))
        .route("/api/scripts/validate", post(validate_script))
        .route("/api/docs", get(get_docs))
        .route("/api/import", post(import_collection))
        .route("/api/impexp/export", post(export_collection))
        .route("/api/runner/run", post(runner_run))
        .route("/api/runner/stop", post(runner_stop))
        .route("/api/runner/parse-data", post(runner_parse_data))
        .route("/api/oauth2/token", post(oauth2_token))
        .route("/api/oauth2/authorize", post(oauth2_api::authorize))
        .route(
            "/api/grpc/reflect/services",
            post(grpc_api::reflect_services),
        )
        .route("/api/grpc/proto/services", post(grpc_api::proto_services))
        .route("/api/grpc/sample-message", post(grpc_api::sample_message))
        .route("/api/grpc/stream", post(grpc_api::stream))
        .route("/api/grpc/stream/cancel", post(grpc_api::cancel_stream))
        .route("/api/graphql/introspect", post(graphql_introspect))
        .route("/api/stream/connect", post(stream_api::connect))
        .route("/api/stream/send", post(stream_api::send))
        .route("/api/stream/disconnect", post(stream_api::disconnect))
        .route("/api/stream/status", get(stream_api::status))
        .route("/api/sync/diff", get(sync_api::diff))
        .route("/api/sync/apply", post(sync_api::apply))
        .route("/api/sync/link", post(sync_api::link))
        .route("/api/mock/start", post(mock_api::start))
        .route("/api/mock/stop", post(mock_api::stop))
        .route("/api/mock/status", get(mock_api::status))
        .route("/api/mock/logs", get(mock_api::logs))
        .route("/api/mock/logs/stream", get(mock_api::logs_stream))
        .route(
            "/api/mock/overrides",
            post(mock_api::update_override).put(mock_api::update_override),
        )
        .route("/api/response/body", get(response_body))
        .route("/api/history", get(history_list).delete(history_clear))
        .route("/api/history/:id", get(history_get).delete(history_delete))
        .route(
            "/api/cookies",
            get(cookies_list).post(cookies_save).delete(cookies_delete),
        )
        .route(
            "/api/environments",
            get(environments).delete(remove_environment),
        )
        .route("/api/environments/save", post(put_environment))
        .route(
            "/api/workspace/secret-backend",
            get(secret_backend).put(set_secret_backend),
        )
        .route(
            "/api/workspace/secret-backend/migrate",
            post(secret_backend_migrate),
        )
        .route("/api/update/check", get(update_api::check))
        .route(
            "/api/update/settings",
            get(update_api::get_settings)
                .post(update_api::save_settings)
                .put(update_api::save_settings),
        )
        .route("/api/update/verify", post(update_api::verify))
        .route("/api/update/apply", post(update_api::apply))
        .route("/api/git/status", get(git_status))
        .route("/api/git/branches", get(git_branches))
        .route("/api/git/diff", get(git_diff))
        .route("/api/git/log", get(git_log))
        .route("/api/git/stage", post(git_stage))
        .route("/api/git/unstage", post(git_unstage))
        .route("/api/git/commit", post(git_commit))
        .route("/api/git/checkout", post(git_checkout))
        .route("/api/git/branch/create", post(git_create_branch))
        .route("/api/git/pull", post(git_pull))
        .route("/api/git/push", post(git_push))
        .route("/api/git/init", post(git_init))
        .fallback(static_asset)
        .layer(DefaultBodyLimit::max(10 * 1024 * 1024))
        .layer(cors)
        .layer(middleware::from_fn_with_state(
            state.clone(),
            auth_middleware,
        ))
        .layer(middleware::from_fn(security_headers))
        .layer(middleware::from_fn(host_header))
        .with_state(state))
}

/// Execute a collection through the same Axum runner used by the desktop UI.
/// The temporary server stays bound to loopback and uses an isolated history
/// directory that is removed when the run completes.
pub async fn run_local_runner(payload: Value) -> Result<Value, String> {
    let nonce = std::time::SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_err(|error| error.to_string())?
        .as_nanos();
    let data_dir =
        std::env::temp_dir().join(format!("pebblepost-cli-{}-{nonce}", std::process::id()));
    fs::create_dir(&data_dir)
        .map_err(|error| format!("could not create temporary runner data directory: {error}"))?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        if let Err(error) = fs::set_permissions(&data_dir, fs::Permissions::from_mode(0o700)) {
            let _ = fs::remove_dir_all(&data_dir);
            return Err(format!(
                "could not secure temporary runner data directory: {error}"
            ));
        }
    }
    let app = match router(String::new(), PathBuf::new(), data_dir.clone()) {
        Ok(app) => app,
        Err(error) => {
            let _ = fs::remove_dir_all(&data_dir);
            return Err(format!("could not initialize local runner: {error}"));
        }
    };
    let listener = match tokio::net::TcpListener::bind("127.0.0.1:0").await {
        Ok(listener) => listener,
        Err(error) => {
            let _ = fs::remove_dir_all(&data_dir);
            return Err(format!("could not start local runner: {error}"));
        }
    };
    let address = listener.local_addr().map_err(|error| error.to_string())?;
    let (shutdown_sender, shutdown_receiver) = oneshot::channel();
    let server_task = tokio::spawn(async move {
        let _ = axum::serve(listener, app)
            .with_graceful_shutdown(async {
                let _ = shutdown_receiver.await;
            })
            .await;
    });
    let run_result = async {
        let response = reqwest::Client::new()
            .post(format!("http://{address}/api/runner/run"))
            .header(reqwest::header::ACCEPT, "text/event-stream")
            .json(&payload)
            .send()
            .await
            .map_err(|error| format!("local runner request failed: {error}"))?;
        let status = response.status();
        let body = response
            .text()
            .await
            .map_err(|error| format!("could not read local runner response: {error}"))?;
        if !status.is_success() {
            return Err(format!("local runner returned HTTP {status}: {body}"));
        }
        for block in body.split("\n\n") {
            let mut event = "";
            let mut data = String::new();
            for line in block.lines() {
                if let Some(value) = line.strip_prefix("event:") {
                    event = value.trim();
                }
                if let Some(value) = line.strip_prefix("data:") {
                    if !data.is_empty() {
                        data.push('\n');
                    }
                    data.push_str(value.trim_start());
                }
            }
            if event == "summary" {
                return serde_json::from_str(&data)
                    .map_err(|error| format!("invalid runner summary: {error}"));
            }
        }
        Err("local runner ended without a summary event".into())
    }
    .await;
    let _ = shutdown_sender.send(());
    let _ = server_task.await;
    let _ = fs::remove_dir_all(&data_dir);
    run_result
}

/// Call a JSON API route in an isolated loopback server for standalone CLI commands.
pub async fn run_local_api(
    method: &str,
    path: &str,
    query: &[(String, String)],
    payload: Option<Value>,
) -> Result<Value, String> {
    let nonce = std::time::SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_err(|error| error.to_string())?
        .as_nanos();
    let data_dir =
        std::env::temp_dir().join(format!("pebblepost-cli-api-{}-{nonce}", std::process::id()));
    fs::create_dir(&data_dir)
        .map_err(|error| format!("could not create temporary API data directory: {error}"))?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        if let Err(error) = fs::set_permissions(&data_dir, fs::Permissions::from_mode(0o700)) {
            let _ = fs::remove_dir_all(&data_dir);
            return Err(format!(
                "could not secure temporary API data directory: {error}"
            ));
        }
    }
    let app = match router(String::new(), PathBuf::new(), data_dir.clone()) {
        Ok(app) => app,
        Err(error) => {
            let _ = fs::remove_dir_all(&data_dir);
            return Err(format!("could not initialize local API: {error}"));
        }
    };
    let listener = match tokio::net::TcpListener::bind("127.0.0.1:0").await {
        Ok(listener) => listener,
        Err(error) => {
            let _ = fs::remove_dir_all(&data_dir);
            return Err(format!("could not start local API: {error}"));
        }
    };
    let address = match listener.local_addr() {
        Ok(address) => address,
        Err(error) => {
            let _ = fs::remove_dir_all(&data_dir);
            return Err(error.to_string());
        }
    };
    let (shutdown_sender, shutdown_receiver) = oneshot::channel();
    let server_task = tokio::spawn(async move {
        let _ = axum::serve(listener, app)
            .with_graceful_shutdown(async {
                let _ = shutdown_receiver.await;
            })
            .await;
    });
    let result = async {
        let method =
            reqwest::Method::from_bytes(method.as_bytes()).map_err(|error| error.to_string())?;
        let mut request = reqwest::Client::new().request(method, format!("http://{address}{path}"));
        if !query.is_empty() {
            request = request.query(query);
        }
        if let Some(payload) = payload.as_ref() {
            request = request.json(payload);
        }
        let response = request
            .send()
            .await
            .map_err(|error| format!("local API request failed: {error}"))?;
        let status = response.status();
        let body = response
            .text()
            .await
            .map_err(|error| format!("could not read local API response: {error}"))?;
        let value = serde_json::from_str::<Value>(&body)
            .map_err(|error| format!("local API returned invalid JSON: {error}"))?;
        if status.is_success() {
            Ok(value)
        } else {
            Err(value
                .get("error")
                .and_then(Value::as_str)
                .map(str::to_owned)
                .unwrap_or_else(|| format!("local API returned HTTP {status}")))
        }
    }
    .await;
    let _ = shutdown_sender.send(());
    let _ = server_task.await;
    let _ = fs::remove_dir_all(&data_dir);
    result
}

async fn health() -> Json<Value> {
    Json(json!({ "status": "ok", "version": env!("CARGO_PKG_VERSION"), "name": "PebblePost" }))
}

async fn static_asset(State(state): State<Arc<AppState>>, uri: Uri) -> Response {
    if uri.path().starts_with("/api/") {
        return (StatusCode::NOT_FOUND, Json(json!({ "error": "not found" }))).into_response();
    }
    let requested = uri.path().trim_start_matches('/');
    let relative = Path::new(requested);
    let safe_relative = relative
        .components()
        .all(|component| matches!(component, std::path::Component::Normal(_)));
    let requested_file = if safe_relative && !requested.is_empty() {
        state.frontend_dir.join(relative)
    } else {
        state.frontend_dir.join("index.html")
    };
    let file = if requested_file.is_file() {
        requested_file
    } else {
        state.frontend_dir.join("index.html")
    };
    match fs::read(&file) {
        Ok(bytes) => {
            let mut response = Response::new(axum::body::Body::from(bytes));
            let content_type = content_type(&file);
            if let Ok(value) = HeaderValue::from_str(content_type) {
                response.headers_mut().insert(header::CONTENT_TYPE, value);
            }
            response
        }
        Err(_) => (StatusCode::NOT_FOUND, "frontend build not found").into_response(),
    }
}

async fn scan(Query(query): Query<PathQuery>) -> Result<Json<Value>, ApiError> {
    let result = scan_workspace(Path::new(&query.path))?;
    Ok(Json(serde_json::to_value(result)?))
}

async fn get_docs(Query(query): Query<DocsQuery>) -> Result<Json<Value>, ApiError> {
    let workspace = PathBuf::from(query.workspace_path.as_deref().unwrap_or("."));
    let result = docs::generate(&workspace, query.path.as_deref(), &query.format)?;
    Ok(Json(result))
}

async fn import_collection(Json(payload): Json<ImportPayload>) -> Response {
    let result = match import_export::parse_import(&payload.source, &payload.content) {
        Ok(result) => result,
        Err(error) => {
            return (
                StatusCode::UNPROCESSABLE_ENTITY,
                Json(json!({ "error": error })),
            )
                .into_response();
        }
    };
    if payload.save {
        let workspace = PathBuf::from(payload.workspace.as_deref().unwrap_or("."));
        if let Err(error) =
            import_export::save_import(&workspace, payload.out_dir.as_deref(), &result)
        {
            return (
                StatusCode::BAD_REQUEST,
                Json(json!({ "error": error.to_string() })),
            )
                .into_response();
        }
    }
    let requests = result
        .requests
        .iter()
        .map(|(_, request)| request.clone())
        .collect::<Vec<_>>();
    let report_requests = result.requests.iter().map(|(path, request)| json!({ "name": request.get("name").and_then(Value::as_str).unwrap_or("Request"), "relPath": path, "request": request })).collect::<Vec<_>>();
    let report_folders = result
        .folders
        .iter()
        .map(|(path, definition)| {
            json!({
                "name": definition.get("name").and_then(Value::as_str).unwrap_or("Folder"),
                "relPath": path,
                "definition": definition
            })
        })
        .collect::<Vec<_>>();
    let report = json!({
        "sourceFormat": result.format,
        "collectionName": result.collection_name,
        "totalRequests": requests.len(),
        "totalFolders": result.folders.len(),
        "totalVariables": result.variables.len(),
        "requests": report_requests,
        "folders": report_folders,
        "variables": result.variables,
        "warnings": result.warnings,
        "skipped": result.skipped
    });
    let count = requests.len();
    Json(json!({ "count": count, "report": report, "requests": requests })).into_response()
}

async fn export_collection(Json(payload): Json<ExportPayload>) -> Result<Json<Value>, ApiError> {
    let folder = payload.folder_path.as_deref().unwrap_or("collections");
    let result = import_export::export_collection(
        Path::new("."),
        folder,
        &payload.format,
        payload.collection_name.as_deref(),
    )?;
    Ok(Json(result))
}

async fn oauth2_token(Json(payload): Json<Value>) -> Result<Json<Value>, ApiError> {
    let auth = oauth2_api::resolve_auth(&payload)
        .map_err(|error| ApiError(PebbleError::InvalidPath(error)))?;
    let token = oauth2_api::get_token(&auth)
        .await
        .map_err(|error| ApiError(PebbleError::InvalidPath(error)))?;
    Ok(Json(token))
}

async fn graphql_introspect(Json(payload): Json<Value>) -> Result<Json<Value>, ApiError> {
    const INTROSPECTION_QUERY: &str = "query IntrospectionQuery { __schema { queryType { name } mutationType { name } subscriptionType { name } types { ...FullType } directives { name description locations args { ...InputValue } } } } fragment FullType on __Type { kind name description fields(includeDeprecated: true) { name description args { ...InputValue } type { ...TypeRef } isDeprecated deprecationReason } inputFields { ...InputValue } interfaces { ...TypeRef } enumValues(includeDeprecated: true) { name description isDeprecated deprecationReason } possibleTypes { ...TypeRef } } fragment InputValue on __InputValue { name description type { ...TypeRef } defaultValue } fragment TypeRef on __Type { kind name ofType { kind name ofType { kind name ofType { kind name ofType { kind name ofType { kind name ofType { kind name ofType { kind name } } } } } } } }";
    let workspace = payload
        .get("workspacePath")
        .and_then(Value::as_str)
        .unwrap_or(".");
    let environment_name = payload.get("environmentName").and_then(Value::as_str);
    let mut request = payload.get("request").cloned().ok_or_else(|| {
        ApiError(PebbleError::InvalidPath(
            "request definition is required".into(),
        ))
    })?;
    if let Some(path) = payload
        .get("path")
        .and_then(Value::as_str)
        .filter(|path| !path.is_empty())
        && let Ok(resolved) =
            resolve_request(Path::new(workspace), Path::new(path), environment_name)
        && let Some(resolved_request) = resolved.get("request")
    {
        request = resolved_request.clone();
    }
    let environments = list_environments(Path::new(workspace)).unwrap_or_default();
    let environment = environment_name.and_then(|name| {
        environments
            .iter()
            .find(|item| item.name.eq_ignore_ascii_case(name))
    });
    let mut interpolated = request.clone();
    interpolate_value(
        &mut interpolated,
        &build_variable_map(environment, &HashMap::new()),
    )
    .map_err(|error| ApiError(PebbleError::InvalidPath(error)))?;
    let url = interpolated
        .get("url")
        .and_then(Value::as_str)
        .filter(|url| !url.is_empty())
        .ok_or_else(|| {
            ApiError(PebbleError::InvalidPath(
                "GraphQL request URL is required".into(),
            ))
        })?;
    let settings = interpolated.get("settings").cloned().unwrap_or(Value::Null);
    let client = reqwest::Client::builder()
        .timeout(Duration::from_millis(
            settings
                .get("timeoutMs")
                .and_then(Value::as_u64)
                .filter(|value| *value > 0)
                .unwrap_or(30_000),
        ))
        .danger_accept_invalid_certs(
            !settings
                .get("verifySSL")
                .and_then(Value::as_bool)
                .unwrap_or(true),
        )
        .build()
        .map_err(|error| ApiError(PebbleError::InvalidPath(error.to_string())))?;
    let mut parsed_url = reqwest::Url::parse(url)
        .map_err(|error| ApiError(PebbleError::InvalidPath(error.to_string())))?;
    if let Some(params) = interpolated.get("params").and_then(Value::as_array) {
        for item in params.iter().filter(|item| {
            item.get("enabled")
                .and_then(Value::as_bool)
                .unwrap_or(false)
        }) {
            if let (Some(key), Some(value)) = (
                item.get("key").and_then(Value::as_str),
                item.get("value").and_then(Value::as_str),
            ) && !key.is_empty()
            {
                parsed_url.query_pairs_mut().append_pair(key, value);
            }
        }
    }
    let mut body = json!({ "query": INTROSPECTION_QUERY });
    let mut builder = client.post(parsed_url);
    if let Some(headers) = interpolated.get("headers").and_then(Value::as_array) {
        for header in headers.iter().filter(|header| {
            header
                .get("enabled")
                .and_then(Value::as_bool)
                .unwrap_or(false)
        }) {
            if let (Some(key), Some(value)) = (
                header.get("key").and_then(Value::as_str),
                header.get("value").and_then(Value::as_str),
            ) && !key.is_empty()
            {
                builder = builder.header(key, value);
            }
        }
    }
    let auth = interpolated
        .get("auth")
        .cloned()
        .unwrap_or_else(|| json!({ "type": "none" }));
    let oauth2_token = if auth
        .get("type")
        .and_then(Value::as_str)
        .is_some_and(|kind| kind.eq_ignore_ascii_case("oauth2"))
    {
        oauth2_api::get_token(&auth).await.ok().and_then(|token| {
            token
                .get("access_token")
                .and_then(Value::as_str)
                .filter(|value| !value.trim().is_empty())
                .map(str::to_owned)
        })
    } else {
        None
    };
    match auth
        .get("type")
        .and_then(Value::as_str)
        .unwrap_or("none")
        .to_ascii_lowercase()
        .as_str()
    {
        "bearer" => {
            builder = builder.bearer_auth(auth.get("token").and_then(Value::as_str).unwrap_or(""))
        }
        "basic" => {
            builder = builder.basic_auth(
                auth.get("username").and_then(Value::as_str).unwrap_or(""),
                Some(auth.get("password").and_then(Value::as_str).unwrap_or("")),
            )
        }
        "oauth2" => {
            if let Some(token) = oauth2_token.as_deref().or_else(|| {
                auth.get("token")
                    .and_then(Value::as_str)
                    .filter(|token| !token.is_empty())
            }) {
                builder = if token.trim().to_ascii_lowercase().starts_with("bearer ") {
                    builder.header(reqwest::header::AUTHORIZATION, token.trim())
                } else {
                    builder.bearer_auth(token)
                };
            }
        }
        "apikey" => {
            let key = auth.get("key").and_then(Value::as_str).unwrap_or("");
            let value = auth.get("value").and_then(Value::as_str).unwrap_or("");
            if !key.is_empty() && auth.get("addTo").and_then(Value::as_str) == Some("query") {
                builder = builder.query(&[(key, value)]);
            } else if !key.is_empty() {
                builder = builder.header(key, value);
            }
        }
        _ => {}
    }
    if let Some(graphql) = interpolated.pointer("/body/graphql") {
        if let Some(query) = graphql.get("query") {
            body["query"] = query.clone();
        }
        if let Some(variables) = graphql
            .get("variables")
            .and_then(Value::as_str)
            .and_then(|value| serde_json::from_str::<Value>(value).ok())
        {
            body["variables"] = variables;
        }
        if let Some(operation) = graphql.get("operationName") {
            body["operationName"] = operation.clone();
        }
    }
    let response = builder.json(&body).send().await.map_err(|error| {
        ApiError(PebbleError::InvalidPath(format!(
            "GraphQL introspection request failed: {error}"
        )))
    })?;
    let status = response.status();
    let response: Value = response.json().await.map_err(|error| {
        ApiError(PebbleError::InvalidPath(format!(
            "GraphQL endpoint returned invalid JSON: {error}"
        )))
    })?;
    if !status.is_success() {
        return Err(ApiError(PebbleError::InvalidPath(format!(
            "GraphQL introspection returned HTTP {status}: {response}"
        ))));
    }
    Ok(Json(response))
}

async fn runner_parse_data(Json(payload): Json<RunnerDataPayload>) -> Response {
    if payload.content.trim().is_empty() {
        return (
            StatusCode::BAD_REQUEST,
            Json(json!({ "error": "Data content cannot be empty" })),
        )
            .into_response();
    }
    let parsed = if payload.filename.to_ascii_lowercase().ends_with(".json")
        || matches!(payload.content.trim().chars().next(), Some('[' | '{'))
    {
        match parse_runner_json(&payload.content) {
            Ok(rows) => rows,
            Err(error) => {
                return (StatusCode::BAD_REQUEST, Json(json!({ "error": error }))).into_response();
            }
        }
    } else {
        match parse_runner_csv(&payload.content) {
            Ok(rows) => rows,
            Err(error) => {
                return (StatusCode::BAD_REQUEST, Json(json!({ "error": error }))).into_response();
            }
        }
    };
    let mut columns = std::collections::BTreeSet::new();
    for row in &parsed {
        if let Some(object) = row.as_object() {
            columns.extend(object.keys().cloned());
        }
    }
    let total_rows = parsed.len();
    Json(json!({ "rows": parsed, "totalRows": total_rows, "columns": columns.into_iter().collect::<Vec<_>>() })).into_response()
}

pub fn parse_cli_data(filename: &str, content: &str) -> Result<Vec<Value>, String> {
    if content.trim().is_empty() {
        return Err("Data content cannot be empty".into());
    }
    if filename.to_ascii_lowercase().ends_with(".json")
        || matches!(content.trim().chars().next(), Some('[' | '{'))
    {
        parse_runner_json(content)
    } else {
        parse_runner_csv(content)
    }
}

fn parse_runner_json(content: &str) -> Result<Vec<Value>, String> {
    let value: Value =
        serde_json::from_str(content).map_err(|error| format!("Invalid JSON data: {error}"))?;
    if let Some(rows) = value.as_array() {
        if rows.iter().any(|row| !row.is_object()) {
            return Err("Every JSON data row must be an object".into());
        }
        return Ok(rows.to_vec());
    }
    if let Some(object) = value.as_object() {
        for key in ["data", "items", "rows", "iterations"] {
            if let Some(rows) = object.get(key).and_then(Value::as_array) {
                let rows = rows
                    .iter()
                    .filter(|row| row.is_object())
                    .cloned()
                    .collect::<Vec<_>>();
                if !rows.is_empty() {
                    return Ok(rows);
                }
            }
        }
        return Ok(vec![value]);
    }
    Err("Expected an array of objects or an object containing row data".into())
}

fn parse_runner_csv(content: &str) -> Result<Vec<Value>, String> {
    let first_line = content.lines().next().unwrap_or("");
    let delimiter = if first_line.contains('\t') && !first_line.contains(',') {
        '\t'
    } else if first_line.contains(';') && !first_line.contains(',') {
        ';'
    } else {
        ','
    };
    let mut records = Vec::<Vec<String>>::new();
    let mut row = Vec::new();
    let mut cell = String::new();
    let mut quoted = false;
    let mut chars = content.chars().peekable();
    while let Some(ch) = chars.next() {
        if quoted {
            if ch == '"' && chars.peek() == Some(&'"') {
                chars.next();
                cell.push('"');
            } else if ch == '"' {
                quoted = false;
            } else {
                cell.push(ch);
            }
        } else if ch == '"' && cell.is_empty() {
            quoted = true;
        } else if ch == delimiter {
            row.push(std::mem::take(&mut cell));
        } else if ch == '\n' {
            row.push(std::mem::take(&mut cell));
            records.push(std::mem::take(&mut row));
        } else if ch != '\r' {
            cell.push(ch);
        }
    }
    if quoted {
        return Err("CSV contains an unterminated quoted field".into());
    }
    if !cell.is_empty() || !row.is_empty() {
        row.push(cell);
        records.push(row);
    }
    if records.is_empty() {
        return Err("CSV data is empty".into());
    }
    let headers = records
        .remove(0)
        .into_iter()
        .map(|header| header.trim_start_matches('\u{feff}').trim().to_owned())
        .collect::<Vec<_>>();
    if headers.is_empty() || headers.iter().any(String::is_empty) {
        return Err("CSV header row contains an empty column name".into());
    }
    let mut result = Vec::new();
    for (index, record) in records.into_iter().enumerate() {
        if record.iter().all(|value| value.trim().is_empty()) {
            continue;
        }
        if record.len() > headers.len() {
            return Err(format!(
                "CSV row {} has more values than its header",
                index + 2
            ));
        }
        let mut object = serde_json::Map::new();
        for (column, header) in headers.iter().enumerate() {
            object.insert(
                header.clone(),
                json!(record.get(column).cloned().unwrap_or_default()),
            );
        }
        result.push(Value::Object(object));
    }
    if result.is_empty() {
        return Err("CSV data file contains headers but no data rows".into());
    }
    Ok(result)
}

async fn runner_stop(
    State(state): State<Arc<AppState>>,
    Json(payload): Json<RunnerStopPayload>,
) -> Json<Value> {
    let sender = state
        .active_runs
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner)
        .remove(&payload.run_id);
    let stopped = sender.is_some();
    if let Some(sender) = sender {
        let _ = sender.send(());
    }
    Json(json!({ "success": stopped }))
}

struct RunnerStream {
    app: Arc<AppState>,
    payload: RunnerPayload,
    run_id: String,
    stop: oneshot::Receiver<()>,
    iteration: usize,
    request_index: usize,
    executions: usize,
    max_executions: usize,
    runtime_vars: HashMap<String, String>,
    results: Vec<Value>,
    started: Instant,
    final_summary: Option<Value>,
    terminal_stage: u8,
    bailed: bool,
    needs_delay: bool,
    announced: bool,
}

async fn runner_run(
    State(state): State<Arc<AppState>>,
    headers: HeaderMap,
    Json(mut payload): Json<RunnerPayload>,
) -> Response {
    let stream_response = payload.stream.unwrap_or(false)
        || headers
            .get(header::ACCEPT)
            .and_then(|value| value.to_str().ok())
            .is_some_and(|value| value.contains("text/event-stream"));
    if payload.request_paths.is_empty() {
        let target_path = payload
            .target_path
            .as_deref()
            .filter(|path| !path.is_empty())
            .unwrap_or(".");
        match discover_runner_requests(Path::new(target_path)) {
            Ok(paths) => payload.request_paths = paths,
            Err(error) => return runner_error_response(stream_response, error),
        }
        if payload.request_paths.is_empty() {
            let target = payload
                .target_path
                .as_deref()
                .filter(|path| !path.is_empty())
                .unwrap_or(".");
            return runner_error_response(
                stream_response,
                format!("no *.pebble.json request files found in {target}"),
            );
        }
    }
    if payload.workspace_path.trim().is_empty() {
        let target_path = payload
            .target_path
            .as_deref()
            .filter(|path| !path.is_empty())
            .map(PathBuf::from)
            .or_else(|| payload.request_paths.first().map(PathBuf::from))
            .unwrap_or_else(|| PathBuf::from("."));
        payload.workspace_path = find_runner_workspace_root(&target_path)
            .to_string_lossy()
            .into_owned();
    }
    if let Err(error) = filter_runner_requests(&mut payload) {
        return runner_error_response(stream_response, error);
    }
    if payload.request_paths.is_empty() {
        return runner_error_response(
            stream_response,
            "no requests matched the configured runner filters",
        );
    }
    if payload
        .data_rows
        .as_ref()
        .is_some_and(|rows| rows.iter().any(|row| !row.is_object()))
    {
        return (
            StatusCode::BAD_REQUEST,
            Json(json!({ "error": "Every JSON data row must be an object" })),
        )
            .into_response();
    }
    if payload.data_rows.as_ref().is_none_or(Vec::is_empty)
        && let Some(data_file) = payload.data_file.as_deref().filter(|path| !path.is_empty())
    {
        let content = match fs::read_to_string(data_file) {
            Ok(content) => content,
            Err(error) => {
                return runner_error_response(
                    stream_response,
                    format!("failed to read data file {data_file:?}: {error}"),
                );
            }
        };
        match parse_cli_data(data_file, &content) {
            Ok(rows) => payload.data_rows = Some(rows),
            Err(error) => {
                return runner_error_response(
                    stream_response,
                    format!("failed to parse data file {data_file:?}: {error}"),
                );
            }
        }
    }
    let run_id = payload
        .run_id
        .clone()
        .filter(|id| !id.is_empty())
        .unwrap_or_else(|| format!("run-{}", chrono::Utc::now().timestamp_millis()));
    let max_executions = payload
        .max_executions
        .filter(|count| *count > 0)
        .unwrap_or(100);
    let (sender, receiver) = oneshot::channel();
    state
        .active_runs
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner)
        .insert(run_id.clone(), sender);
    let run = RunnerStream {
        app: state,
        payload,
        run_id,
        stop: receiver,
        iteration: 1,
        request_index: 0,
        executions: 0,
        max_executions,
        runtime_vars: HashMap::new(),
        results: Vec::new(),
        started: Instant::now(),
        final_summary: None,
        terminal_stage: 0,
        bailed: false,
        needs_delay: false,
        announced: false,
    };
    if stream_response {
        let events = stream::unfold(run, |mut runner| async move {
            runner_next(&mut runner)
                .await
                .map(|event| (Ok::<Event, Infallible>(event), runner))
        });
        Sse::new(events)
            .keep_alive(KeepAlive::default())
            .into_response()
    } else {
        let mut runner = run;
        while runner_next(&mut runner).await.is_some() {}
        let summary = runner
            .final_summary
            .take()
            .unwrap_or_else(|| runner_summary(&runner));
        Json(summary).into_response()
    }
}

fn runner_error_response(stream_response: bool, error: impl Into<String>) -> Response {
    let error = error.into();
    if !stream_response {
        return (
            StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({ "error": error })),
        )
            .into_response();
    }
    let events = vec![
        Ok::<_, Infallible>(
            Event::default()
                .event("error")
                .data(json!({ "error": error }).to_string()),
        ),
        Ok::<_, Infallible>(Event::default().event("complete").data("null")),
    ];
    Sse::new(stream::iter(events)).into_response()
}

fn filter_runner_requests(payload: &mut RunnerPayload) -> Result<(), String> {
    let request_filter = payload.request_filter.as_deref().unwrap_or("").trim();
    let folder_filter = payload.folder_filter.as_deref().unwrap_or("").trim();
    let tag_filter = payload.tag_filter.as_deref().unwrap_or("").trim();
    if request_filter.is_empty() && folder_filter.is_empty() && tag_filter.is_empty() {
        return Ok(());
    }

    let root = Path::new(&payload.workspace_path);
    let candidates = std::mem::take(&mut payload.request_paths);
    for path in candidates {
        let request = read_request(root, Path::new(&path))
            .map_err(|error| format!("could not filter runner request {path:?}: {error}"))?;
        let relative = runner_relative_path(root, Path::new(&path));
        if runner_request_matches(
            &request,
            &relative,
            request_filter,
            folder_filter,
            tag_filter,
        ) {
            payload.request_paths.push(path);
        }
    }
    Ok(())
}

fn runner_relative_path(workspace_root: &Path, path: &Path) -> String {
    let relative = if let Ok(relative) = path.strip_prefix(workspace_root) {
        relative.to_path_buf()
    } else if path.is_absolute()
        && let Ok(root) = fs::canonicalize(workspace_root)
        && let Ok(canonical_path) = safe_workspace_path(&root, path)
        && let Ok(relative) = canonical_path.strip_prefix(root)
    {
        relative.to_path_buf()
    } else {
        path.to_path_buf()
    };
    relative.to_string_lossy().replace('\\', "/")
}

fn runner_request_matches(
    request: &Value,
    rel_path: &str,
    request_filter: &str,
    folder_filter: &str,
    tag_filter: &str,
) -> bool {
    let directory = Path::new(rel_path).parent().unwrap_or(Path::new(""));
    let folder = if directory == Path::new(".") {
        ""
    } else {
        directory.to_str().unwrap_or("")
    };
    if !folder_filter.is_empty()
        && !glob_match(folder_filter, folder)
        && !glob_match(
            folder_filter,
            Path::new(folder)
                .file_name()
                .and_then(|name| name.to_str())
                .unwrap_or(""),
        )
    {
        return false;
    }

    let request_name = request
        .get("name")
        .and_then(Value::as_str)
        .filter(|name| !name.is_empty())
        .or_else(|| {
            Path::new(rel_path)
                .file_stem()
                .and_then(|name| name.to_str())
        })
        .unwrap_or("");
    if !request_filter.is_empty() {
        let lower_filter = request_filter.to_lowercase();
        let substring_match = request_name.to_lowercase().contains(&lower_filter)
            || rel_path.to_lowercase().contains(&lower_filter);
        let name_match = glob_match(request_filter, request_name)
            || glob_match(request_filter, rel_path)
            || glob_match(
                request_filter,
                Path::new(rel_path)
                    .file_name()
                    .and_then(|name| name.to_str())
                    .unwrap_or(""),
            );
        if !substring_match && !name_match {
            return false;
        }
    }

    if !tag_filter.is_empty() {
        let matches_tag = request
            .get("tags")
            .and_then(Value::as_array)
            .into_iter()
            .flatten()
            .filter_map(Value::as_str)
            .any(|tag| tag.eq_ignore_ascii_case(tag_filter) || glob_match(tag_filter, tag));
        if !matches_tag {
            return false;
        }
    }
    true
}

fn glob_match(pattern: &str, text: &str) -> bool {
    fn match_from(
        pattern: &[char],
        text: &[char],
        pi: usize,
        ti: usize,
        memo: &mut HashMap<(usize, usize), bool>,
    ) -> bool {
        if let Some(result) = memo.get(&(pi, ti)) {
            return *result;
        }
        let result = if pi == pattern.len() {
            ti == text.len()
        } else {
            match pattern[pi] {
                '*' => {
                    match_from(pattern, text, pi + 1, ti, memo)
                        || (ti < text.len()
                            && text[ti] != '/'
                            && match_from(pattern, text, pi, ti + 1, memo))
                }
                '?' => {
                    ti < text.len()
                        && text[ti] != '/'
                        && match_from(pattern, text, pi + 1, ti + 1, memo)
                }
                '[' => match_glob_class(pattern, text, pi, ti).is_some_and(|(next_pi, matched)| {
                    matched && match_from(pattern, text, next_pi, ti + 1, memo)
                }),
                '\\' if pi + 1 < pattern.len() => {
                    ti < text.len()
                        && pattern[pi + 1] == text[ti]
                        && match_from(pattern, text, pi + 2, ti + 1, memo)
                }
                literal => {
                    ti < text.len()
                        && literal == text[ti]
                        && match_from(pattern, text, pi + 1, ti + 1, memo)
                }
            }
        };
        memo.insert((pi, ti), result);
        result
    }

    fn match_glob_class(
        pattern: &[char],
        text: &[char],
        pi: usize,
        ti: usize,
    ) -> Option<(usize, bool)> {
        let value = *text.get(ti)?;
        if value == '/' {
            return None;
        }
        let mut index = pi + 1;
        let negated = matches!(pattern.get(index), Some('!' | '^'));
        if negated {
            index += 1;
        }
        let mut class = Vec::new();
        let mut closed = false;
        while index < pattern.len() {
            if pattern[index] == ']' && !class.is_empty() {
                index += 1;
                closed = true;
                break;
            }
            if pattern[index] == '\\' && index + 1 < pattern.len() {
                class.push(pattern[index + 1]);
                index += 2;
            } else {
                class.push(pattern[index]);
                index += 1;
            }
        }
        if !closed || class.is_empty() {
            return None;
        }
        let mut matched = false;
        let mut class_index = 0;
        while class_index < class.len() {
            if class_index + 2 < class.len() && class[class_index + 1] == '-' {
                matched |= class[class_index] <= value && value <= class[class_index + 2];
                class_index += 3;
            } else {
                matched |= class[class_index] == value;
                class_index += 1;
            }
        }
        Some((index, if negated { !matched } else { matched }))
    }

    match_from(
        &pattern.chars().collect::<Vec<_>>(),
        &text.chars().collect::<Vec<_>>(),
        0,
        0,
        &mut HashMap::new(),
    )
}

fn discover_runner_requests(target: &Path) -> Result<Vec<String>, String> {
    let metadata = fs::metadata(target).map_err(|error| error.to_string())?;
    if !metadata.is_dir() {
        if target.to_string_lossy().ends_with(".pebble.json") {
            return Ok(vec![target.to_string_lossy().into_owned()]);
        }
        return Err(format!(
            "target file '{}' is not a .pebble.json file",
            target.display()
        ));
    }

    fn visit(directory: &Path, files: &mut Vec<String>) -> Result<(), String> {
        let mut entries = fs::read_dir(directory)
            .map_err(|error| error.to_string())?
            .map(|entry| entry.map_err(|error| error.to_string()))
            .collect::<Result<Vec<_>, _>>()?;
        entries.sort_by_key(|entry| entry.file_name());
        for entry in entries {
            let path = entry.path();
            let name = entry.file_name().to_string_lossy().into_owned();
            let file_type = entry.file_type().map_err(|error| error.to_string())?;
            if file_type.is_dir() {
                if name.starts_with('.') || matches!(name.as_str(), "node_modules" | "dist") {
                    continue;
                }
                visit(&path, files)?;
            } else if name.ends_with(".pebble.json") {
                files.push(path.to_string_lossy().into_owned());
            }
        }
        Ok(())
    }

    let mut files = Vec::new();
    visit(target, &mut files)?;
    files.sort();
    Ok(files)
}

fn find_runner_workspace_root(path: &Path) -> PathBuf {
    let absolute = if path.is_absolute() {
        path.to_path_buf()
    } else {
        std::env::current_dir().unwrap_or_default().join(path)
    };
    let absolute = fs::canonicalize(&absolute).unwrap_or(absolute);
    let mut current = if fs::metadata(&absolute).is_ok_and(|metadata| !metadata.is_dir()) {
        absolute.parent().unwrap_or(&absolute).to_path_buf()
    } else {
        absolute.clone()
    };
    for _ in 0..6 {
        if current.join(".pebble").is_dir() {
            return current;
        }
        let Some(parent) = current.parent() else {
            break;
        };
        if parent == current {
            break;
        }
        current = parent.to_path_buf();
    }
    absolute
}

fn runner_iteration_count(payload: &RunnerPayload) -> usize {
    if let Some(row_count) = payload
        .data_rows
        .as_ref()
        .map(Vec::len)
        .filter(|count| *count > 0)
    {
        let requested = payload.iterations.unwrap_or(0);
        if requested == 0 || requested >= row_count {
            row_count
        } else {
            requested
        }
    } else {
        payload.iterations.unwrap_or(0).max(1)
    }
}

async fn runner_next(runner: &mut RunnerStream) -> Option<Event> {
    if !runner.announced {
        runner.announced = true;
        return Some(
            Event::default().event("start").data(
                json!({
                    "runId": runner.run_id,
                    "target": runner.payload.target_path.as_deref().unwrap_or("."),
                    "requestCount": runner.payload.request_paths.len(),
                    "iterationCount": runner_iteration_count(&runner.payload),
                    "dryRun": runner.payload.dry_run.unwrap_or(false),
                })
                .to_string(),
            ),
        );
    }
    if runner.terminal_stage > 0 {
        if runner.terminal_stage == 1 {
            runner.terminal_stage = 2;
            return Some(
                Event::default()
                    .event("complete")
                    .data(runner.final_summary.as_ref()?.to_string()),
            );
        }
        runner
            .app
            .active_runs
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .remove(&runner.run_id);
        return None;
    }
    if runner.stop.try_recv().is_ok() {
        runner.bailed = true;
    }
    let max_iterations = runner_iteration_count(&runner.payload);
    if runner.bailed
        || runner.iteration > max_iterations
        || runner.request_index >= runner.payload.request_paths.len()
    {
        if !runner.bailed
            && runner.iteration <= max_iterations
            && runner.request_index >= runner.payload.request_paths.len()
            && runner.iteration < max_iterations
        {
            runner.iteration += 1;
            runner.request_index = 0;
            runner.executions = 0;
            runner.runtime_vars.clear();
        } else {
            let summary = runner_summary(runner);
            runner.final_summary = Some(summary.clone());
            runner.terminal_stage = 1;
            return Some(Event::default().event("summary").data(summary.to_string()));
        }
    }
    if runner.needs_delay {
        runner.needs_delay = false;
        let has_next_request = runner.request_index < runner.payload.request_paths.len()
            || runner.iteration < max_iterations;
        let delay_ms = runner.payload.delay_ms.unwrap_or(0);
        if has_next_request && delay_ms > 0 {
            tokio::select! {
                _ = tokio::time::sleep(Duration::from_millis(delay_ms)) => {},
                _ = &mut runner.stop => {
                    runner.bailed = true;
                    let summary = runner_summary(runner);
                    runner.final_summary = Some(summary.clone()); runner.terminal_stage = 1;
                    return Some(Event::default().event("summary").data(summary.to_string()));
                }
            }
        }
    }
    let now = match runner.stop.try_recv() {
        Ok(()) => {
            runner.bailed = true;
            let summary = runner_summary(runner);
            runner.final_summary = Some(summary.clone());
            runner.terminal_stage = 1;
            return Some(Event::default().event("summary").data(summary.to_string()));
        }
        Err(_) => Instant::now(),
    };
    if runner.executions >= runner.max_executions {
        runner.bailed = true;
        let mut summary = runner_summary(runner);
        summary["success"] = json!(false);
        summary["error"] = json!(format!(
            "Infinite loop detected: exceeded maximum executions ({}) in iteration {}",
            runner.max_executions, runner.iteration
        ));
        runner.final_summary = Some(summary.clone());
        runner.terminal_stage = 1;
        return Some(Event::default().event("summary").data(summary.to_string()));
    }
    runner.executions += 1;
    let path = runner.payload.request_paths[runner.request_index].clone();
    runner.request_index += 1;
    let root = Path::new(&runner.payload.workspace_path);
    let request = match read_request(root, Path::new(&path)) {
        Ok(request) => request,
        Err(error) => {
            let elapsed = now.elapsed();
            let result = execution_failure(error.to_string()).0;
            let passed = false;
            let event = runner_record_result(runner, path, Value::Null, result, elapsed);
            if runner.payload.bail.unwrap_or(false) && !passed {
                runner.bailed = true;
            }
            runner.needs_delay = true;
            return Some(event);
        }
    };
    if runner.payload.dry_run.unwrap_or(false) {
        let method = request
            .get("method")
            .and_then(Value::as_str)
            .unwrap_or("GET");
        let url = request.get("url").and_then(Value::as_str).unwrap_or("");
        let rel_path = runner_relative_path(root, Path::new(&path));
        let record = json!({
            "iteration": runner.iteration,
            "filePath": path,
            "relPath": rel_path,
            "request": request,
            "passed": true,
            "error": format!("dry-run: would send {method} {url}"),
            "duration": 0,
            "skipped": true,
        });
        runner.results.push(record.clone());
        runner.needs_delay = true;
        return Some(
            Event::default()
                .event("request_result")
                .data(record.to_string()),
        );
    }
    let data_row = runner
        .payload
        .data_rows
        .as_ref()
        .and_then(|rows| rows.get(runner.iteration - 1))
        .cloned();
    let mut overrides = std::env::vars()
        .filter_map(|(key, value)| {
            key.strip_prefix("PEBBLE_VAR_")
                .map(|name| (name.to_owned(), value))
        })
        .collect::<HashMap<_, _>>();
    overrides.extend(runner.payload.base_vars.clone());
    overrides.extend(runner.payload.extra_vars.clone());
    overrides.insert("$iteration".into(), runner.iteration.to_string());
    overrides.insert("iteration".into(), runner.iteration.to_string());
    if let Some(row) = data_row.as_ref().and_then(Value::as_object) {
        for (key, value) in row {
            let value = value
                .as_str()
                .map(str::to_owned)
                .unwrap_or_else(|| value.to_string());
            overrides.insert(format!("data.{key}"), value.clone());
            overrides.insert(key.clone(), value);
        }
    }
    overrides.extend(runner.runtime_vars.clone());
    let scripts_trusted = runner.payload.trusted.unwrap_or_else(|| {
        get_workspace_info(root)
            .map(|workspace| workspace.trusted)
            .unwrap_or(false)
            || root.join(".pebbletrust").is_file()
    });
    let execute = ExecutePayload {
        workspace_path: Some(runner.payload.workspace_path.clone()),
        environment_name: runner.payload.environment_name.clone(),
        secret_backend: runner.payload.secret_backend.clone(),
        path: Some(path.clone()),
        request: request.clone(),
        trusted: Some(scripts_trusted),
        timeout_ms: runner.payload.timeout_ms,
        overrides: overrides.clone(),
        iteration: Some(runner.iteration),
        iteration_count: Some(max_iterations),
        data_row,
    };
    let started = Instant::now();
    let retry_count = runner.payload.retry_count.unwrap_or(0);
    let mut result = Value::Null;
    let mut retries = 0;
    for attempt in 0..=retry_count {
        result = tokio::select! {
            result = execute_request(State(runner.app.clone()), Json(execute.clone())) => match result {
                Ok(result) => result.0,
                Err(error) => execution_failure(error.0.to_string()).0,
            },
            _ = &mut runner.stop => {
                runner.bailed = true;
                let summary = runner_summary(runner);
                runner.final_summary = Some(summary.clone()); runner.terminal_stage = 1;
                return Some(Event::default().event("summary").data(summary.to_string()));
            }
        };
        if result.get("networkError").and_then(Value::as_bool) != Some(true)
            || attempt == retry_count
        {
            break;
        }
        retries = attempt + 1;
        tokio::select! {
            _ = tokio::time::sleep(Duration::from_millis(((attempt + 1) * 500) as u64)) => {},
            _ = &mut runner.stop => {
                runner.bailed = true;
                let summary = runner_summary(runner);
                runner.final_summary = Some(summary.clone()); runner.terminal_stage = 1;
                return Some(Event::default().event("summary").data(summary.to_string()));
            }
        }
    }
    if retries > 0 {
        result["retryCount"] = json!(retries);
    }
    let elapsed = started.elapsed();
    if let Some(extracted) = result.get("extractedEnvVars").and_then(Value::as_object) {
        runner
            .runtime_vars
            .extend(extracted.iter().filter_map(|(key, value)| {
                value.as_str().map(|value| (key.clone(), value.to_owned()))
            }));
    }
    if result
        .get("stopAll")
        .and_then(Value::as_bool)
        .unwrap_or(false)
    {
        runner.bailed = true;
    } else if let Some(target) = result.get("nextRequest").and_then(Value::as_str) {
        if target.is_empty() {
            runner.request_index = runner.payload.request_paths.len();
        } else if let Some(target_index) =
            runner.payload.request_paths.iter().position(|candidate| {
                let candidate_path = Path::new(candidate);
                candidate == target
                    || runner_relative_path(
                        Path::new(&runner.payload.workspace_path),
                        candidate_path,
                    ) == target
                    || candidate_path.file_name().and_then(|name| name.to_str()) == Some(target)
                    || read_request(Path::new(&runner.payload.workspace_path), candidate_path)
                        .ok()
                        .and_then(|request| {
                            request
                                .get("name")
                                .and_then(Value::as_str)
                                .map(str::to_owned)
                        })
                        .as_deref()
                        == Some(target)
            })
        {
            runner.request_index = target_index;
        }
    }
    let environments = list_environments(root).unwrap_or_default();
    let active_environment = get_workspace_info(root)
        .ok()
        .map(|workspace| workspace.active_environment);
    let environment_name = runner
        .payload
        .environment_name
        .as_deref()
        .or(active_environment.as_deref());
    let environment = environment_name.and_then(|name| {
        environments
            .iter()
            .find(|item| item.name.eq_ignore_ascii_case(name))
    });
    let resolved_request = resolve_request(root, Path::new(&path), environment_name).ok();
    let effective_request = resolved_request
        .as_ref()
        .and_then(|resolved| resolved.get("request").cloned())
        .unwrap_or_else(|| request.clone());
    let mut override_secrets = overrides
        .values()
        .flat_map(|value| pebblepost_core::interpolate::resolved_os_environment_secrets(value))
        .collect::<Vec<_>>();
    override_secrets.extend(
        overrides
            .iter()
            .filter(|(key, value)| is_sensitive_variable_name(key) && !value.is_empty())
            .map(|(_, value)| value.clone()),
    );
    if let Some(folder_variables) = resolved_request
        .as_ref()
        .and_then(|resolved| resolved.get("resolvedVariables"))
        .and_then(Value::as_object)
    {
        override_secrets.extend(
            folder_variables
                .values()
                .filter_map(Value::as_str)
                .flat_map(pebblepost_core::interpolate::resolved_os_environment_secrets),
        );
    }
    let secrets = script_secrets(&effective_request, environment, &override_secrets);
    mask_runner_result(&mut result, &secrets);
    let passed = runner_result_passed(&result);
    let event = runner_record_result(runner, path, request, result, elapsed);
    if runner.payload.bail.unwrap_or(false) && !passed {
        runner.bailed = true;
    }
    runner.needs_delay = true;
    Some(event)
}

fn runner_record_result(
    runner: &mut RunnerStream,
    path: String,
    request: Value,
    result: Value,
    elapsed: Duration,
) -> Event {
    let passed = runner_result_passed(&result);
    let error = runner_result_error(&result);
    let root = Path::new(&runner.payload.workspace_path);
    let rel_path = runner_relative_path(root, Path::new(&path));
    let record = json!({
        "iteration": runner.iteration,
        "filePath": path,
        "relPath": rel_path,
        "request": request,
        "result": result,
        "passed": passed,
        "error": error.map(Value::String).unwrap_or(Value::Null),
        "duration": elapsed.as_nanos().min(u64::MAX as u128) as u64
    });
    runner.results.push(record.clone());
    Event::default()
        .event("request_result")
        .data(record.to_string())
}

fn runner_result_error(result: &Value) -> Option<String> {
    if let Some(error) = result
        .get("error")
        .and_then(Value::as_str)
        .filter(|error| !error.is_empty())
    {
        return Some(error.to_owned());
    }
    let failures = result
        .get("tests")
        .and_then(Value::as_array)?
        .iter()
        .filter(|test| test.get("passed").and_then(Value::as_bool) == Some(false))
        .map(|test| {
            let name = test
                .get("name")
                .and_then(Value::as_str)
                .unwrap_or("assertion");
            let message = test
                .get("message")
                .and_then(Value::as_str)
                .unwrap_or("assertion failed");
            format!("{name}: {message}")
        })
        .collect::<Vec<_>>();
    if failures.is_empty() {
        None
    } else {
        Some(failures.join("; "))
    }
}

fn runner_result_passed(result: &Value) -> bool {
    result.get("error").is_none_or(Value::is_null)
        && result
            .get("tests")
            .and_then(Value::as_array)
            .is_none_or(|tests| {
                tests
                    .iter()
                    .all(|test| test.get("passed").and_then(Value::as_bool).unwrap_or(true))
            })
}

fn runner_summary(runner: &RunnerStream) -> Value {
    let total = runner.results.len();
    let passed = runner
        .results
        .iter()
        .filter(|result| result.get("passed").and_then(Value::as_bool) == Some(true))
        .count();
    let failed = total.saturating_sub(passed);
    let skipped = runner
        .results
        .iter()
        .filter(|result| result.get("skipped").and_then(Value::as_bool) == Some(true))
        .count();
    let (total_tests, passed_tests, failed_tests) = runner
        .results
        .iter()
        .filter_map(|record| {
            record
                .get("result")
                .and_then(|result| result.get("tests"))
                .and_then(Value::as_array)
        })
        .flatten()
        .fold((0usize, 0usize, 0usize), |(total, passed, failed), test| {
            if test.get("passed").and_then(Value::as_bool).unwrap_or(true) {
                (total + 1, passed + 1, failed)
            } else {
                (total + 1, passed, failed + 1)
            }
        });
    let mut durations = runner
        .results
        .iter()
        .filter_map(|result| result.get("duration").and_then(Value::as_u64))
        .map(|duration| duration as f64 / 1_000_000.0)
        .collect::<Vec<_>>();
    durations.sort_by(f64::total_cmp);
    let total_duration_ms = runner.started.elapsed().as_secs_f64() * 1000.0;
    let average = if total == 0 {
        0.0
    } else {
        durations.iter().sum::<f64>() / total as f64
    };
    let p95 = if durations.is_empty() {
        0.0
    } else {
        durations[((durations.len() as f64 * 0.95).ceil() as usize)
            .saturating_sub(1)
            .min(durations.len() - 1)]
    };
    let mut results_by_iteration = BTreeMap::<usize, Vec<Value>>::new();
    for result in &runner.results {
        let iteration = result.get("iteration").and_then(Value::as_u64).unwrap_or(1) as usize;
        results_by_iteration
            .entry(iteration)
            .or_default()
            .push(result.clone());
    }
    let mut passed_iterations = 0usize;
    let mut failed_iterations = 0usize;
    let iterations = results_by_iteration
        .into_iter()
        .map(|(iteration, results)| {
            let passed = results.iter().all(|result| {
                result
                    .get("passed")
                    .and_then(Value::as_bool)
                    .unwrap_or(false)
            });
            if passed {
                passed_iterations += 1;
            } else {
                failed_iterations += 1;
            }
            let tests = results
                .iter()
                .filter_map(|result| result.pointer("/result/tests").and_then(Value::as_array))
                .flatten()
                .collect::<Vec<_>>();
            let total_tests = tests.len();
            let passed_tests = tests
                .iter()
                .filter(|test| test.get("passed").and_then(Value::as_bool).unwrap_or(true))
                .count();
            drop(tests);
            let duration = results
                .iter()
                .filter_map(|result| result.get("duration").and_then(Value::as_u64))
                .fold(0u64, u64::saturating_add);
            let mut summary = json!({
                "iteration": iteration,
                "results": results,
                "totalTests": total_tests,
                "passedTests": passed_tests,
                "failedTests": total_tests.saturating_sub(passed_tests),
                "duration": duration,
                "passed": passed,
            });
            if let Some(data_row) = runner
                .payload
                .data_rows
                .as_ref()
                .and_then(|rows| rows.get(iteration.saturating_sub(1)))
                && !data_row.is_null()
            {
                summary["dataRow"] = data_row.clone();
            }
            summary
        })
        .collect::<Vec<_>>();
    let total_iterations = iterations.len();
    let mut summary = json!({
        "target": runner.payload.target_path.as_deref().unwrap_or("."),
        "environment": runner.payload.environment_name,
        "totalIterations": total_iterations,
        "passedIterations": passed_iterations,
        "failedIterations": failed_iterations,
        "totalRequests": total,
        "passedRequests": passed,
        "failedRequests": failed,
        "skippedRequests": skipped,
        "totalTests": total_tests,
        "passedTests": passed_tests,
        "failedTests": failed_tests,
        "totalDurationMs": total_duration_ms,
        "avgDurationMs": average,
        "p95DurationMs": p95,
        "passRate": if total == 0 { 100.0 } else { passed as f64 * 100.0 / total as f64 },
        "bailed": runner.bailed,
        "success": failed == 0 && !runner.bailed,
        "iterations": iterations,
        "results": runner.results
    });
    if runner.payload.dry_run.unwrap_or(false) {
        summary["dryRun"] = json!(true);
    }
    summary
}

struct WorkspaceEventStream {
    root: PathBuf,
    requested_workspace_path: String,
    snapshot: HashMap<String, (u64, i64)>,
    pending: BTreeMap<String, Value>,
    ready: VecDeque<Value>,
    debounce_due: Option<tokio::time::Instant>,
    initial: bool,
    interval: tokio::time::Interval,
}

async fn workspace_events(
    Query(query): Query<WorkspaceEventsQuery>,
) -> Result<Sse<impl futures_util::Stream<Item = Result<Event, Infallible>>>, ApiError> {
    let requested_workspace_path = query.workspace_path.clone();
    let root = validate_workspace_root(Path::new(&query.workspace_path)).map_err(ApiError)?;
    let initial_snapshot = workspace_snapshot(&root);
    let mut interval = tokio::time::interval(Duration::from_millis(500));
    interval.tick().await;
    let events = stream::unfold(
        WorkspaceEventStream {
            root: root.clone(),
            requested_workspace_path,
            snapshot: initial_snapshot,
            pending: BTreeMap::new(),
            ready: VecDeque::new(),
            debounce_due: None,
            initial: true,
            interval,
        },
        |mut state| async move {
            if state.initial {
                state.initial = false;
                let data = json!({ "type": "connected", "workspacePath": state.requested_workspace_path.as_str() });
                return Some((Ok(Event::default().data(data.to_string())), state));
            }
            let event = next_workspace_change(&mut state).await;
            Some((Ok(Event::default().data(event.to_string())), state))
        },
    );
    Ok(Sse::new(events).keep_alive(
        KeepAlive::new()
            .interval(Duration::from_secs(15))
            .text("keep-alive"),
    ))
}

async fn next_workspace_change(state: &mut WorkspaceEventStream) -> Value {
    const DEBOUNCE: Duration = Duration::from_millis(200);
    loop {
        if let Some(event) = state.ready.pop_front() {
            return event;
        }

        if state
            .debounce_due
            .is_some_and(|due| due <= tokio::time::Instant::now())
        {
            state
                .ready
                .extend(std::mem::take(&mut state.pending).into_values());
            state.debounce_due = None;
            continue;
        }

        if let Some(next_due) = state.debounce_due {
            let poll = tokio::select! {
                _ = state.interval.tick() => true,
                _ = tokio::time::sleep_until(next_due) => false,
            };
            if poll {
                collect_workspace_changes(state, DEBOUNCE);
            }
        } else {
            state.interval.tick().await;
            collect_workspace_changes(state, DEBOUNCE);
        }
    }
}

fn collect_workspace_changes(state: &mut WorkspaceEventStream, debounce: Duration) {
    let current = workspace_snapshot(&state.root);
    let previous = std::mem::replace(&mut state.snapshot, current);
    let mut changed = false;
    for (relative, (size, mtime)) in &state.snapshot {
        if previous.get(relative) == Some(&(*size, *mtime)) {
            continue;
        }
        let path = state.root.join(relative);
        if pebblepost_core::workspace_path_is_suppressed(&path) {
            continue;
        }
        state.pending.insert(
            relative.clone(),
            json!({
                "path": path.to_string_lossy(),
                "relPath": relative,
                "op": if previous.contains_key(relative) { "write" } else { "create" },
                "mtime": mtime,
            }),
        );
        changed = true;
    }
    for relative in previous.keys() {
        if state.snapshot.contains_key(relative) {
            continue;
        }
        let path = state.root.join(relative);
        if pebblepost_core::workspace_path_is_suppressed(&path) {
            continue;
        }
        state.pending.insert(
            relative.clone(),
            json!({
                "path": path.to_string_lossy(),
                "relPath": relative,
                "op": "remove",
                "mtime": std::time::SystemTime::now().duration_since(UNIX_EPOCH)
                    .map(|duration| duration.as_millis() as i64).unwrap_or_default(),
            }),
        );
        changed = true;
    }
    if changed {
        state.debounce_due = Some(tokio::time::Instant::now() + debounce);
    }
}

fn workspace_snapshot(root: &Path) -> HashMap<String, (u64, i64)> {
    fn visit(root: &Path, directory: &Path, snapshot: &mut HashMap<String, (u64, i64)>) {
        let Ok(entries) = fs::read_dir(directory) else {
            return;
        };
        for entry in entries.flatten() {
            let name = entry.file_name();
            let name = name.to_string_lossy();
            if name.starts_with('.')
                || matches!(
                    name.as_ref(),
                    "node_modules" | "dist" | "build" | "bin" | "target"
                )
                || name.ends_with(".tmp")
            {
                continue;
            }
            let path = entry.path();
            let Ok(metadata) = entry.metadata() else {
                continue;
            };
            if metadata.is_dir() {
                visit(root, &path, snapshot);
            } else if metadata.is_file() {
                let relative = path
                    .strip_prefix(root)
                    .unwrap_or(&path)
                    .to_string_lossy()
                    .replace('\\', "/");
                let mtime = metadata
                    .modified()
                    .ok()
                    .and_then(|time| time.duration_since(UNIX_EPOCH).ok())
                    .map(|duration| duration.as_millis() as i64)
                    .unwrap_or_default();
                snapshot.insert(relative, (metadata.len(), mtime));
            }
        }
    }
    let mut snapshot = HashMap::new();
    visit(root, root, &mut snapshot);
    snapshot
}

async fn workspace_info(Query(query): Query<PathQuery>) -> Result<Json<Value>, ApiError> {
    Ok(Json(serde_json::to_value(get_workspace_info(Path::new(
        &query.path,
    ))?)?))
}

async fn workspace_init(Json(payload): Json<InitPayload>) -> Result<Json<Value>, ApiError> {
    let workspace = init_workspace(Path::new(&payload.path), payload.name.as_deref())?;
    Ok(Json(json!({ "success": true, "workspace": workspace })))
}

async fn gitignore_ensure(
    Json(payload): Json<WorkspacePathPayload>,
) -> Result<Json<Value>, ApiError> {
    let status = ensure_gitignore(Path::new(&payload.workspace_path))?;
    Ok(Json(json!({ "success": true, "status": status })))
}

async fn create_workspace_folder(
    Json(payload): Json<PathPayload>,
) -> Result<Json<Value>, ApiError> {
    create_folder(Path::new(&payload.workspace_path), Path::new(&payload.path))?;
    Ok(Json(json!({ "success": true })))
}

async fn rename_workspace_path(
    Json(payload): Json<RenamePayload>,
) -> Result<Json<Value>, ApiError> {
    rename_path(
        Path::new(&payload.workspace_path),
        Path::new(&payload.old_path),
        Path::new(&payload.new_path),
    )?;
    Ok(Json(json!({ "success": true })))
}

async fn move_workspace_path(Json(payload): Json<MovePayload>) -> Result<Json<Value>, ApiError> {
    let new_path = move_path(
        Path::new(&payload.workspace_path),
        Path::new(&payload.source_path),
        Path::new(&payload.target_folder),
    )?;
    Ok(Json(json!({ "success": true, "newPath": new_path })))
}

async fn duplicate_workspace_request(
    Json(payload): Json<PathPayload>,
) -> Result<Json<Value>, ApiError> {
    let new_path = duplicate_request(Path::new(&payload.workspace_path), Path::new(&payload.path))?;
    Ok(Json(json!({ "success": true, "newPath": new_path })))
}

async fn get_folder(Query(query): Query<FolderQuery>) -> Result<Json<Value>, ApiError> {
    Ok(Json(read_folder(
        Path::new(&query.workspace_path),
        Path::new(&query.path),
    )?))
}

async fn put_folder(Json(payload): Json<FolderPayload>) -> Result<Json<Value>, ApiError> {
    save_folder(
        Path::new(&payload.workspace_path),
        Path::new(&payload.path),
        payload.folder,
    )?;
    Ok(Json(json!({ "success": true })))
}

async fn get_request(Query(query): Query<RequestQuery>) -> Result<Json<Value>, ApiError> {
    Ok(Json(read_request(
        Path::new(&query.workspace_path),
        Path::new(&query.path),
    )?))
}

async fn get_resolved_request(Query(query): Query<RequestQuery>) -> Result<Json<Value>, ApiError> {
    Ok(Json(resolve_request(
        Path::new(&query.workspace_path),
        Path::new(&query.path),
        None,
    )?))
}

async fn interpolate_request(
    Json(payload): Json<InterpolatePayload>,
) -> Result<Json<Value>, ApiError> {
    let environments = list_environments(Path::new(&payload.workspace_path))?;
    let environment = payload.environment_name.as_deref().and_then(|name| {
        environments
            .iter()
            .find(|environment| environment.name.eq_ignore_ascii_case(name))
    });
    let variables = build_variable_map(environment, &payload.overrides);
    let mut request = payload.request;
    interpolate_value(&mut request, &variables)
        .map_err(|error| ApiError(PebbleError::InvalidPath(error)))?;
    Ok(Json(
        json!({ "interpolatedRequest": request, "variables": variables }),
    ))
}

async fn environments(Query(query): Query<EnvironmentQuery>) -> Result<Json<Value>, ApiError> {
    Ok(Json(serde_json::to_value(list_environments(Path::new(
        &query.workspace_path,
    ))?)?))
}

async fn remove_environment(
    Query(query): Query<EnvironmentQuery>,
) -> Result<Json<Value>, ApiError> {
    let name = query.name.ok_or_else(|| {
        ApiError(PebbleError::InvalidPath(
            "environment name is required".into(),
        ))
    })?;
    delete_environment(Path::new(&query.workspace_path), &name)?;
    Ok(Json(json!({ "success": true })))
}

async fn put_environment(
    Json(payload): Json<SaveEnvironmentPayload>,
) -> Result<Json<Value>, ApiError> {
    save_environment(
        Path::new(&payload.workspace_path),
        payload.environment,
        payload.is_secret,
    )?;
    Ok(Json(json!({ "success": true })))
}

async fn secret_backend(Query(query): Query<SecretBackendQuery>) -> Result<Json<Value>, ApiError> {
    let workspace = get_workspace_info(Path::new(&query.workspace_path))?;
    let mut config = workspace.secret_backend;
    if config.default.is_empty() {
        config.default = "file".into();
    }
    Ok(Json(serde_json::to_value(config)?))
}

async fn set_secret_backend(
    Query(query): Query<SecretBackendQuery>,
    Json(config): Json<Value>,
) -> Result<Json<Value>, ApiError> {
    let default = config.get("default").and_then(Value::as_str).unwrap_or("");
    if !matches!(default, "" | "file" | "keychain" | "env") {
        return Err(ApiError(PebbleError::InvalidPath(format!(
            "invalid backend: {default}"
        ))));
    }
    if let Some(per_env) = config.get("perEnv") {
        let Some(entries) = per_env.as_object() else {
            return Err(ApiError(PebbleError::InvalidPath(
                "perEnv must be an object".into(),
            )));
        };
        for (environment, backend) in entries {
            let backend = backend.as_str().unwrap_or("");
            if !matches!(backend, "" | "file" | "keychain" | "env") {
                return Err(ApiError(PebbleError::InvalidPath(format!(
                    "invalid backend for env {environment}: {backend}"
                ))));
            }
        }
    }
    let mut workspace = get_workspace_info(Path::new(&query.workspace_path))?;
    let per_env = config
        .get("perEnv")
        .and_then(Value::as_object)
        .map(|entries| {
            entries
                .iter()
                .map(|(environment, backend)| {
                    (
                        environment.clone(),
                        backend.as_str().unwrap_or("").to_owned(),
                    )
                })
                .collect()
        })
        .unwrap_or_default();
    workspace.secret_backend = SecretBackendConfig {
        default: if default.is_empty() { "file" } else { default }.to_owned(),
        per_env,
    };
    save_workspace_info(Path::new(&query.workspace_path), &workspace)?;
    Ok(Json(serde_json::to_value(workspace.secret_backend)?))
}

async fn secret_backend_migrate(Json(payload): Json<Value>) -> Response {
    let workspace_path = payload
        .get("workspacePath")
        .and_then(Value::as_str)
        .unwrap_or("");
    let environment = payload
        .get("envName")
        .and_then(Value::as_str)
        .unwrap_or("")
        .trim();
    let destination = payload.get("to").and_then(Value::as_str).unwrap_or("");
    if workspace_path.is_empty() || environment.is_empty() {
        return (
            StatusCode::BAD_REQUEST,
            Json(json!({ "error": "workspacePath and envName are required" })),
        )
            .into_response();
    }
    let workspace = match get_workspace_info(Path::new(workspace_path)) {
        Ok(workspace) => workspace,
        Err(error) => {
            return (
                StatusCode::BAD_REQUEST,
                Json(json!({ "error": error.to_string() })),
            )
                .into_response();
        }
    };
    let per_env = workspace
        .secret_backend
        .per_env
        .get(environment)
        .map(String::as_str);
    let current = per_env
        .or_else(|| {
            Some(workspace.secret_backend.default.as_str()).filter(|value| !value.is_empty())
        })
        .unwrap_or("file");
    let source = match pebblepost_core::secrets::SecretBackend::parse(current) {
        Ok(backend) => backend,
        Err(error) => {
            return (StatusCode::BAD_REQUEST, Json(json!({ "error": error }))).into_response();
        }
    };
    let destination = match pebblepost_core::secrets::SecretBackend::parse(destination) {
        Ok(backend) if backend != pebblepost_core::secrets::SecretBackend::Environment => backend,
        Ok(_) => return (StatusCode::BAD_REQUEST, Json(json!({ "error": "the env backend is read-only and cannot be a migration destination" }))).into_response(),
        Err(error) => return (StatusCode::BAD_REQUEST, Json(json!({ "error": error }))).into_response(),
    };
    if source == pebblepost_core::secrets::SecretBackend::Environment {
        return (
            StatusCode::BAD_REQUEST,
            Json(
                json!({ "error": "the env backend is read-only and cannot be a migration source" }),
            ),
        )
            .into_response();
    }
    if source == destination {
        return Json(
            json!({ "message": format!("Secrets already use the {} backend", source.as_str()) }),
        )
        .into_response();
    }
    let root = Path::new(workspace_path);
    let keys = match pebblepost_core::secrets::list(root, environment, source) {
        Ok(keys) => keys,
        Err(error) => {
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(json!({ "error": error })),
            )
                .into_response();
        }
    };
    if keys.is_empty() {
        return Json(
            json!({ "message": format!("No secrets found for environment {environment:?}") }),
        )
        .into_response();
    }
    let backup = if source == pebblepost_core::secrets::SecretBackend::File {
        match pebblepost_core::secrets::backup_file(root, environment) {
            Ok(path) => path,
            Err(error) => {
                return (
                    StatusCode::INTERNAL_SERVER_ERROR,
                    Json(json!({ "error": format!("backup secret file: {error}") })),
                )
                    .into_response();
            }
        }
    } else {
        None
    };
    let mut transferred: Vec<String> = Vec::new();
    for key in &keys {
        let transfer =
            pebblepost_core::secrets::get(root, environment, key, source).and_then(|value| {
                pebblepost_core::secrets::set(root, environment, key, &value, destination)
            });
        if let Err(error) = transfer {
            for copied in &transferred {
                let _ = pebblepost_core::secrets::delete(root, environment, copied, destination);
            }
            if let Some(backup) = backup.as_ref() {
                let _ = pebblepost_core::secrets::restore_backup(backup);
            }
            return (
                StatusCode::INTERNAL_SERVER_ERROR,
                Json(json!({ "error": format!("migration failed: {error}") })),
            )
                .into_response();
        }
        transferred.push(key.clone());
    }
    let mut warnings = Vec::new();
    for key in &keys {
        if let Err(error) = pebblepost_core::secrets::delete(root, environment, key, source) {
            warnings.push(format!(
                "could not remove {key:?} from source backend: {error}"
            ));
        }
    }
    let mut result = json!({ "message": "migration complete" });
    if let Some(path) = backup {
        result["backupPath"] = json!(path.to_string_lossy());
    }
    if !warnings.is_empty() {
        result["warnings"] = json!(warnings);
    }
    Json(result).into_response()
}

async fn cookies_list(Query(query): Query<CookieQuery>) -> Result<Json<Value>, ApiError> {
    let cookies = list_cookies(Path::new(&query.workspace_path))?;
    let mut grouped = serde_json::Map::new();
    for cookie in cookies {
        let domain = cookie
            .get("domain")
            .and_then(Value::as_str)
            .unwrap_or("")
            .to_owned();
        grouped
            .entry(domain)
            .or_insert_with(|| json!([]))
            .as_array_mut()
            .expect("created array")
            .push(cookie);
    }
    Ok(Json(Value::Object(grouped)))
}

async fn cookies_save(Json(payload): Json<CookiePayload>) -> Result<Json<Value>, ApiError> {
    save_cookie(Path::new(&payload.workspace_path), payload.cookie)?;
    Ok(Json(json!({ "success": true })))
}

async fn cookies_delete(Query(query): Query<CookieQuery>) -> Result<Json<Value>, ApiError> {
    let cookies = delete_cookies(
        Path::new(&query.workspace_path),
        query.domain.as_deref(),
        query.path.as_deref(),
        query.name.as_deref(),
    )?;
    Ok(Json(json!({ "success": true, "count": cookies.len() })))
}

async fn put_request(Json(payload): Json<SaveRequestPayload>) -> Result<Json<Value>, ApiError> {
    let path = save_request(
        Path::new(&payload.workspace_path),
        Path::new(&payload.path),
        payload.request,
    )?;
    Ok(Json(json!({ "success": true, "path": path })))
}

async fn remove_request(Json(payload): Json<DeletePayload>) -> Result<Json<Value>, ApiError> {
    delete_path(
        Path::new(&payload.workspace_path),
        Path::new(&payload.path),
        payload.permanent,
    )?;
    Ok(Json(json!({ "success": true })))
}

async fn put_example(Json(mut payload): Json<Value>) -> Result<Json<Value>, ApiError> {
    let workspace = payload
        .get("workspacePath")
        .and_then(Value::as_str)
        .unwrap_or("")
        .to_owned();
    let path = payload
        .get("filePath")
        .or_else(|| payload.get("path"))
        .and_then(Value::as_str)
        .unwrap_or("")
        .to_owned();
    if workspace.is_empty() || path.is_empty() {
        return Err(ApiError(PebbleError::InvalidPath(
            "workspacePath and filePath are required".into(),
        )));
    }
    let example = if let Some(example) = payload.get("example") {
        example.clone()
    } else {
        let object = payload.as_object_mut().ok_or_else(|| {
            ApiError(PebbleError::InvalidPath(
                "example payload must be an object".into(),
            ))
        })?;
        for key in ["workspacePath", "filePath", "path"] {
            object.remove(key);
        }
        Value::Object(object.clone())
    };
    let (example, examples) = save_example(Path::new(&workspace), Path::new(&path), example)?;
    Ok(Json(
        json!({ "success": true, "example": example, "examples": examples }),
    ))
}

async fn remove_example(Json(payload): Json<Value>) -> Result<Json<Value>, ApiError> {
    let workspace = payload
        .get("workspacePath")
        .and_then(Value::as_str)
        .unwrap_or("")
        .to_owned();
    let path = payload
        .get("filePath")
        .or_else(|| payload.get("path"))
        .and_then(Value::as_str)
        .unwrap_or("")
        .to_owned();
    let example_id = payload
        .get("exampleId")
        .and_then(Value::as_str)
        .unwrap_or("")
        .to_owned();
    if workspace.is_empty() || path.is_empty() || example_id.is_empty() {
        return Err(ApiError(PebbleError::InvalidPath(
            "workspacePath, filePath and exampleId are required".into(),
        )));
    }
    let examples = delete_example(Path::new(&workspace), Path::new(&path), &example_id)?;
    Ok(Json(json!({ "success": true, "examples": examples })))
}

fn parse_set_cookie(header: &str, request_host: &str, request_path: &str) -> Option<Value> {
    let mut parts = header.split(';');
    let (name, value) = parts.next()?.trim().split_once('=')?;
    let name = name.trim();
    if name.is_empty() {
        return None;
    }
    let mut cookie = json!({
        "name": name,
        "value": value.trim(),
        "domain": request_host.to_ascii_lowercase(),
        "path": default_cookie_path(request_path),
        "secure": false,
        "httpOnly": false,
    });
    for attribute in parts {
        let attribute = attribute.trim();
        let (key, value) = attribute
            .split_once('=')
            .map(|(key, value)| (key.trim().to_ascii_lowercase(), Some(value.trim())))
            .unwrap_or_else(|| (attribute.to_ascii_lowercase(), None));
        match key.as_str() {
            "domain" => {
                if let Some(value) = value.filter(|value| !value.is_empty()) {
                    cookie["domain"] = json!(value.trim_start_matches('.').to_ascii_lowercase());
                }
            }
            "path" => {
                if let Some(value) = value.filter(|value| value.starts_with('/')) {
                    cookie["path"] = json!(value);
                }
            }
            "secure" => cookie["secure"] = json!(true),
            "httponly" => cookie["httpOnly"] = json!(true),
            "samesite" => {
                if let Some(value) = value {
                    cookie["sameSite"] = json!(value);
                }
            }
            "max-age" => {
                if let Some(value) = value.and_then(|value| value.parse::<i64>().ok()) {
                    cookie["maxAge"] = json!(value);
                }
            }
            "expires" => {
                if let Some(value) =
                    value.and_then(|value| chrono::DateTime::parse_from_rfc2822(value).ok())
                {
                    cookie["expires"] = json!(value.with_timezone(&chrono::Utc).to_rfc3339());
                }
            }
            _ => {}
        }
    }
    if let Some(age) = cookie
        .get("maxAge")
        .and_then(Value::as_i64)
        .filter(|age| *age > 0)
    {
        let safe_age = age.min(10 * 365 * 24 * 60 * 60);
        cookie["expires"] =
            json!((chrono::Utc::now() + chrono::Duration::seconds(safe_age)).to_rfc3339());
    }
    Some(cookie)
}

fn default_cookie_path(request_path: &str) -> String {
    if !request_path.starts_with('/') || request_path.matches('/').count() <= 1 {
        return "/".into();
    }
    request_path
        .rsplit_once('/')
        .map(|(directory, _)| if directory.is_empty() { "/" } else { directory })
        .unwrap_or("/")
        .to_owned()
}

fn buffered_multipart_body(
    fields: &[Value],
    workspace: Option<&str>,
) -> Result<(Vec<u8>, String), String> {
    let mut random = [0_u8; 16];
    getrandom::fill(&mut random)
        .map_err(|error| format!("could not create multipart boundary: {error}"))?;
    let random_hex = random
        .iter()
        .map(|byte| format!("{byte:02x}"))
        .collect::<String>();
    let boundary = format!("----------------pebblepost{random_hex}");
    let mut body = Vec::new();
    for item in fields.iter().filter(|item| {
        item.get("enabled")
            .and_then(Value::as_bool)
            .unwrap_or(false)
    }) {
        let name = item.get("key").and_then(Value::as_str).unwrap_or("");
        if name.is_empty() {
            continue;
        }
        let name = multipart_parameter(name);
        let value = item.get("value").and_then(Value::as_str).unwrap_or("");
        body.extend_from_slice(
            format!("--{boundary}\r\nContent-Disposition: form-data; name=\"{name}\"").as_bytes(),
        );
        if item.get("type").and_then(Value::as_str) == Some("file") {
            let workspace =
                workspace.ok_or_else(|| "file fields require a workspace root".to_owned())?;
            let path = value.strip_prefix("@file:").unwrap_or(value).trim();
            let safe_path = safe_workspace_path(Path::new(workspace), Path::new(path))
                .map_err(|error| error.to_string())?;
            let bytes = fs::read(&safe_path)
                .map_err(|error| format!("could not read upload file: {error}"))?;
            let filename = safe_path
                .file_name()
                .and_then(|name| name.to_str())
                .unwrap_or("upload.bin");
            body.extend_from_slice(
                format!(
                    "; filename=\"{}\"\r\nContent-Type: application/octet-stream\r\n\r\n",
                    multipart_parameter(filename)
                )
                .as_bytes(),
            );
            body.extend_from_slice(&bytes);
            body.extend_from_slice(b"\r\n");
        } else {
            body.extend_from_slice(b"\r\n\r\n");
            body.extend_from_slice(value.as_bytes());
            body.extend_from_slice(b"\r\n");
        }
    }
    body.extend_from_slice(format!("--{boundary}--\r\n").as_bytes());
    Ok((body, format!("multipart/form-data; boundary={boundary}")))
}

fn multipart_parameter(value: &str) -> String {
    value
        .replace(['\r', '\n'], "")
        .replace('\\', "\\\\")
        .replace('"', "\\\"")
}

fn execution_failure(message: impl Into<String>) -> Json<Value> {
    Json(json!({
        "statusCode": 0, "statusText": "Request Error", "headers": {}, "body": "", "size": 0,
        "timing": { "dnsLookupMs": 0, "tcpConnectMs": 0, "tlsHandshakeMs": 0, "ttfbMs": 0, "downloadMs": 0, "totalDurationMs": 0 },
        "tests": [], "logs": [], "executedAt": chrono::Utc::now().to_rfc3339(), "error": message.into(),
    }))
}

fn execution_network_failure(message: impl Into<String>) -> Json<Value> {
    let mut result = execution_failure(message).0;
    result["networkError"] = json!(true);
    Json(result)
}

async fn validate_script(Json(payload): Json<ScriptValidationPayload>) -> Json<Value> {
    let source = payload
        .source
        .as_deref()
        .filter(|source| !source.trim().is_empty())
        .unwrap_or("script");
    match scripting::validate_syntax(source, &payload.script) {
        Ok(()) => Json(json!({ "valid": true, "source": source })),
        Err(error) => Json(json!({ "valid": false, "source": source, "error": error })),
    }
}

#[allow(clippy::too_many_arguments)]
async fn run_script(
    phase: scripting::ScriptPhase,
    source: String,
    script: String,
    request: Value,
    response: Option<Value>,
    variables: HashMap<String, String>,
    data_row: Option<Value>,
    iteration: usize,
    iteration_count: usize,
    timeout: Duration,
    secrets: Vec<String>,
) -> Result<Value, String> {
    tokio::task::spawn_blocking(move || {
        std::thread::spawn(move || {
            scripting::execute(scripting::ScriptInput {
                phase,
                source: &source,
                script: &script,
                request: &request,
                response: response.as_ref(),
                variables: &variables,
                data_row: data_row.as_ref(),
                iteration,
                iteration_count,
                timeout,
                secrets: &secrets,
            })
        })
        .join()
        .unwrap_or_else(|_| Err("script worker panicked".into()))
    })
    .await
    .map_err(|error| format!("script task failed: {error}"))?
}

fn script_chain(
    resolved: Option<&Value>,
    request: &Value,
    key: &str,
    request_key: &str,
) -> Vec<Value> {
    if let Some(chain) = resolved
        .and_then(|value| value.get(key))
        .and_then(Value::as_array)
    {
        return chain.clone();
    }
    request
        .pointer(request_key)
        .and_then(Value::as_str)
        .filter(|script| !script.trim().is_empty())
        .map(|script| vec![json!({ "source": "request", "path": "request", "script": script })])
        .unwrap_or_default()
}

fn script_secrets(
    request: &Value,
    environment: Option<&EnvironmentDefinition>,
    extra_secrets: &[String],
) -> Vec<String> {
    let mut secrets = environment
        .into_iter()
        .flat_map(|environment| &environment.variables)
        .flat_map(|variable| {
            let mut values = Vec::new();
            if variable.secret && !variable.value.is_empty() {
                values.push(variable.value.clone());
            }
            values.extend(
                pebblepost_core::interpolate::resolved_os_environment_secrets(&variable.value),
            );
            values
        })
        .collect::<Vec<_>>();
    if let Some(folder_variables) = request.get("resolvedVariables").and_then(Value::as_object) {
        secrets.extend(
            folder_variables
                .values()
                .filter_map(Value::as_str)
                .flat_map(pebblepost_core::interpolate::resolved_os_environment_secrets),
        );
    }
    secrets.extend_from_slice(extra_secrets);
    for key in [
        "token",
        "password",
        "secretKey",
        "clientSecret",
        "refreshToken",
        "value",
    ] {
        if let Some(value) = request
            .pointer(&format!("/auth/{key}"))
            .and_then(Value::as_str)
            .filter(|value| !value.is_empty())
        {
            secrets.push(value.to_owned());
        }
    }
    secrets
}

fn is_sensitive_variable_name(name: &str) -> bool {
    let name = name.to_ascii_uppercase();
    [
        "SECRET",
        "PASSWORD",
        "TOKEN",
        "KEY",
        "AUTH",
        "CREDENTIAL",
        "PRIVATE",
    ]
    .iter()
    .any(|marker| name.contains(marker))
}

fn append_script_array(target: &mut Value, field: &str, addition: &Value) {
    let Some(values) = addition.as_array() else {
        return;
    };
    if values.is_empty() {
        return;
    }
    if !target.get(field).is_some_and(Value::is_array) {
        target[field] = json!([]);
    }
    if let Some(output) = target[field].as_array_mut() {
        output.extend(values.iter().cloned());
    }
}

fn apply_script_output(
    result: &mut Value,
    output: &Value,
    variables: &mut HashMap<String, String>,
    next_request: &mut Option<String>,
    stop_all: &mut bool,
) {
    for field in ["tests", "logs", "consoleLogs"] {
        append_script_array(result, field, &output[field]);
    }
    if let Some(env_vars) = output.get("extractedEnvVars").and_then(Value::as_object) {
        for (key, value) in env_vars {
            if let Some(value) = value.as_str() {
                variables.insert(key.clone(), value.to_owned());
            }
        }
        if !env_vars.is_empty() {
            if !result.get("extractedEnvVars").is_some_and(Value::is_object) {
                result["extractedEnvVars"] = json!({});
            }
            if let Some(target) = result["extractedEnvVars"].as_object_mut() {
                target.extend(
                    env_vars
                        .iter()
                        .map(|(key, value)| (key.clone(), value.clone())),
                );
            }
        }
    }
    if output.get("nextRequestSet").and_then(Value::as_bool) == Some(true) {
        *next_request = Some(
            output
                .get("nextRequest")
                .and_then(Value::as_str)
                .unwrap_or("")
                .to_owned(),
        );
    }
    *stop_all |= output
        .get("stopAll")
        .and_then(Value::as_bool)
        .unwrap_or(false);
}

#[allow(clippy::too_many_arguments)]
async fn run_post_script(
    entry: &Value,
    request: &Value,
    result: &mut Value,
    variables: &mut HashMap<String, String>,
    data_row: Option<Value>,
    iteration: usize,
    iteration_count: usize,
    timeout: Duration,
    secrets: &[String],
    next_request: &mut Option<String>,
    stop_all: &mut bool,
) {
    let source = entry
        .get("source")
        .and_then(Value::as_str)
        .unwrap_or("request")
        .to_owned();
    let script = entry
        .get("script")
        .and_then(Value::as_str)
        .unwrap_or("")
        .to_owned();
    match run_script(
        scripting::ScriptPhase::PostResponse,
        source.clone(),
        script,
        request.clone(),
        Some(result.clone()),
        variables.clone(),
        data_row,
        iteration,
        iteration_count,
        timeout,
        secrets.to_vec(),
    )
    .await
    {
        Ok(output) => {
            apply_script_output(result, &output, variables, next_request, stop_all);
            if let Some(error) = output
                .get("error")
                .and_then(Value::as_str)
                .filter(|error| !error.is_empty())
                && let Some(logs) = result.get_mut("logs").and_then(Value::as_array_mut)
            {
                logs.push(json!(format!("[{source} Error] {error}")));
            }
        }
        Err(error) => {
            let message = format!("[{source}] execution failed: {error}");
            if !result.get("tests").is_some_and(Value::is_array) {
                result["tests"] = json!([]);
            }
            result["tests"].as_array_mut().unwrap().push(json!({ "name": format!("{source} Script Error"), "passed": false, "message": message }));
            if !result.get("logs").is_some_and(Value::is_array) {
                result["logs"] = json!([]);
            }
            result["logs"].as_array_mut().unwrap().push(json!(message));
            if !result.get("consoleLogs").is_some_and(Value::is_array) {
                result["consoleLogs"] = json!([]);
            }
            result["consoleLogs"].as_array_mut().unwrap().push(json!({
                "timestamp": chrono::Utc::now().to_rfc3339(), "level": "error", "source": source, "message": error
            }));
        }
    }
}

fn mask_script_logs(value: &mut Value, secrets: &[String]) {
    if let Some(logs) = value.get_mut("logs").and_then(Value::as_array_mut) {
        for log in logs {
            if let Some(message) = log.as_str().map(str::to_owned) {
                *log = json!(pebblepost_core::security::mask_secrets(&message, secrets));
            }
        }
    }
    if let Some(entries) = value.get_mut("consoleLogs").and_then(Value::as_array_mut) {
        for entry in entries {
            if let Some(message) = entry
                .get("message")
                .and_then(Value::as_str)
                .map(str::to_owned)
            {
                entry["message"] =
                    json!(pebblepost_core::security::mask_secrets(&message, secrets));
            }
        }
    }
    if let Some(error) = value
        .get("error")
        .and_then(Value::as_str)
        .map(str::to_owned)
    {
        value["error"] = json!(pebblepost_core::security::mask_secrets(&error, secrets));
    }
}

fn mask_runner_result(result: &mut Value, secrets: &[String]) {
    if let Some(body) = result
        .get("body")
        .and_then(Value::as_str)
        .map(str::to_owned)
    {
        result["body"] = json!(pebblepost_core::security::mask_secrets(&body, secrets));
    }
    mask_script_logs(result, secrets);
    if let Some(tests) = result.get_mut("tests").and_then(Value::as_array_mut) {
        for test in tests {
            if let Some(message) = test
                .get("message")
                .and_then(Value::as_str)
                .map(str::to_owned)
            {
                test["message"] = json!(pebblepost_core::security::mask_secrets(&message, secrets));
            }
        }
    }
}

async fn execute_request(
    State(state): State<Arc<AppState>>,
    Json(payload): Json<ExecutePayload>,
) -> Result<Json<Value>, ApiError> {
    let mut request = payload.request;
    let mut resolved = None;
    if let (Some(workspace), Some(path)) =
        (payload.workspace_path.as_deref(), payload.path.as_deref())
        && !path.is_empty()
        && let Ok(resolved_info) = resolve_request(
            Path::new(workspace),
            Path::new(path),
            payload.environment_name.as_deref(),
        )
    {
        if let Some(resolved_request) = resolved_info.get("request") {
            request = resolved_request.clone();
        }
        resolved = Some(resolved_info);
    }
    let environments = payload
        .workspace_path
        .as_deref()
        .and_then(|workspace| list_environments(Path::new(workspace)).ok());
    let active_environment = payload
        .workspace_path
        .as_deref()
        .and_then(|workspace| get_workspace_info(Path::new(workspace)).ok())
        .map(|workspace| workspace.active_environment)
        .filter(|name| !name.is_empty());
    let environment_name = payload
        .environment_name
        .as_deref()
        .or(active_environment.as_deref());
    let mut environment = environment_name
        .and_then(|name| {
            environments
                .as_ref()?
                .iter()
                .find(|item| item.name.eq_ignore_ascii_case(name))
        })
        .cloned();
    let selected_secret_backend = payload
        .workspace_path
        .as_deref()
        .zip(environment_name)
        .and_then(|(workspace, name)| {
            pebblepost_core::secrets::configured_backend(
                Path::new(workspace),
                name,
                payload.secret_backend.as_deref(),
            )
            .ok()
        });
    if matches!(
        selected_secret_backend,
        Some(
            pebblepost_core::secrets::SecretBackend::Environment
                | pebblepost_core::secrets::SecretBackend::Keychain
        )
    ) && let Some(environment) = environment.as_mut()
    {
        for variable in &mut environment.variables {
            let is_secret = variable.secret;
            let is_blank = variable.value.is_empty();
            if is_secret && is_blank && !variable.key.is_empty() {
                let key = variable.key.clone();
                let value = match selected_secret_backend {
                    Some(pebblepost_core::secrets::SecretBackend::Environment) => {
                        std::env::var(&key).ok()
                    }
                    Some(pebblepost_core::secrets::SecretBackend::Keychain) => {
                        match payload.workspace_path.as_deref().zip(environment_name).map(
                            |(workspace, name)| {
                                pebblepost_core::secrets::get(
                                    Path::new(workspace),
                                    name,
                                    &key,
                                    pebblepost_core::secrets::SecretBackend::Keychain,
                                )
                            },
                        ) {
                            Some(Ok(value)) => Some(value),
                            Some(Err(error)) if error.starts_with("secret not found:") => None,
                            Some(Err(error)) => return Ok(execution_failure(error)),
                            None => None,
                        }
                    }
                    _ => None,
                };
                if let Some(value) = value {
                    variable.value = value;
                }
            }
        }
    }
    let environment = environment.as_ref();
    let folder_variables = resolved
        .as_ref()
        .and_then(|value| value.get("resolvedVariables"))
        .and_then(Value::as_object);
    let mut variables =
        build_variable_map_with_folder(environment, folder_variables, &payload.overrides);
    if let Err(error) = interpolate_value(&mut request, &variables) {
        return Ok(execution_failure(error));
    }
    let mut override_secrets = payload
        .overrides
        .values()
        .flat_map(|value| pebblepost_core::interpolate::resolved_os_environment_secrets(value))
        .collect::<Vec<_>>();
    override_secrets.extend(
        payload
            .overrides
            .iter()
            .filter(|(key, value)| is_sensitive_variable_name(key) && !value.is_empty())
            .map(|(_, value)| value.clone()),
    );
    if let Some(folder_variables) = folder_variables {
        override_secrets.extend(
            folder_variables
                .values()
                .filter_map(Value::as_str)
                .flat_map(pebblepost_core::interpolate::resolved_os_environment_secrets),
        );
    }
    let secrets = script_secrets(&request, environment, &override_secrets);
    let script_timeout = request
        .pointer("/settings/scriptTimeoutMs")
        .and_then(Value::as_u64)
        .filter(|ms| *ms > 0)
        .map(Duration::from_millis)
        .unwrap_or(Duration::from_secs(5));
    let pre_scripts = script_chain(
        resolved.as_ref(),
        &request,
        "preScriptChain",
        "/scripts/preRequest",
    );
    let post_scripts = script_chain(
        resolved.as_ref(),
        &request,
        "postScriptChain",
        "/scripts/postResponse",
    );
    let mut pre_output =
        json!({ "tests": [], "logs": [], "consoleLogs": [], "extractedEnvVars": {} });
    let mut script_next_request = None;
    let mut script_stop_all = false;
    if payload.trusted == Some(false) {
        for entry in &pre_scripts {
            let source = entry
                .get("source")
                .and_then(Value::as_str)
                .unwrap_or("request");
            pre_output["logs"]
                .as_array_mut()
                .unwrap()
                .push(json!(format!(
                    "[WARN] Pre-request script ({source}) blocked: workspace is not trusted"
                )));
            pre_output["consoleLogs"]
                .as_array_mut()
                .unwrap()
                .push(json!({
                    "timestamp": chrono::Utc::now().to_rfc3339(), "level": "warn", "source": source,
                    "message": "Pre-request script blocked: workspace is not trusted"
                }));
        }
    } else {
        for entry in &pre_scripts {
            let source = entry
                .get("source")
                .and_then(Value::as_str)
                .unwrap_or("request")
                .to_owned();
            let script = entry
                .get("script")
                .and_then(Value::as_str)
                .unwrap_or("")
                .to_owned();
            let output = match run_script(
                scripting::ScriptPhase::PreRequest,
                source.clone(),
                script,
                request.clone(),
                None,
                variables.clone(),
                payload.data_row.clone(),
                payload.iteration.unwrap_or(0),
                payload.iteration_count.unwrap_or(1),
                script_timeout,
                secrets.clone(),
            )
            .await
            {
                Ok(output) => output,
                Err(error) => {
                    let mut failure =
                        execution_failure(format!("[{source}] Pre-request script error: {error}"))
                            .0;
                    failure["logs"] = pre_output["logs"].clone();
                    failure["consoleLogs"] = pre_output["consoleLogs"].clone();
                    return Ok(Json(failure));
                }
            };
            if let Some(updated_request) = output.get("request") {
                request = updated_request.clone();
            }
            apply_script_output(
                &mut pre_output,
                &output,
                &mut variables,
                &mut script_next_request,
                &mut script_stop_all,
            );
            if let Some(error) = output
                .get("error")
                .and_then(Value::as_str)
                .filter(|error| !error.is_empty())
            {
                let mut failure =
                    execution_failure(format!("[{source}] Pre-request script error: {error}")).0;
                failure["logs"] = pre_output["logs"].clone();
                failure["consoleLogs"] = pre_output["consoleLogs"].clone();
                mask_script_logs(&mut failure, &secrets);
                return Ok(Json(failure));
            }
        }
    }
    if request.get("protocol").and_then(Value::as_str) == Some("grpc")
        || request.get("method").and_then(Value::as_str) == Some("GRPC")
    {
        let grpc = request.get("grpc").cloned().unwrap_or(Value::Null);
        let timeout_ms = payload
            .timeout_ms
            .filter(|ms| *ms > 0)
            .or_else(|| {
                request
                    .pointer("/settings/timeoutMs")
                    .and_then(Value::as_u64)
                    .filter(|ms| *ms > 0)
            })
            .unwrap_or(30_000);
        let mut result =
            grpc_api::execute(&grpc, payload.workspace_path.as_deref(), timeout_ms, |_| {}).await;
        if matches!(
            result.get("statusCode").and_then(Value::as_u64),
            Some(503 | 504)
        ) || matches!(
            result.get("grpcStatus").and_then(Value::as_u64),
            Some(4 | 14)
        ) {
            result["networkError"] = json!(true);
        }
        apply_extractors(&request, &mut result);
        if let Some(extracted) = result.get("extractedEnvVars").and_then(Value::as_object) {
            variables.extend(extracted.iter().filter_map(|(key, value)| {
                value.as_str().map(|value| (key.clone(), value.to_owned()))
            }));
        }
        for field in ["logs", "consoleLogs"] {
            append_script_array(&mut result, field, &pre_output[field]);
        }
        if !result.get("logs").is_some_and(Value::is_array) {
            result["logs"] = json!([]);
        }
        if !result.get("consoleLogs").is_some_and(Value::is_array) {
            result["consoleLogs"] = json!([]);
        }
        for entry in &post_scripts {
            if payload.trusted == Some(false) {
                let source = entry
                    .get("source")
                    .and_then(Value::as_str)
                    .unwrap_or("request");
                result["logs"].as_array_mut().unwrap().push(json!(format!(
                    "[WARN] Post-response script ({source}) blocked: workspace is not trusted"
                )));
                result["consoleLogs"].as_array_mut().unwrap().push(json!({
                    "timestamp": chrono::Utc::now().to_rfc3339(), "level": "warn", "source": source,
                    "message": "Post-response script blocked: workspace is not trusted"
                }));
                continue;
            }
            run_post_script(
                entry,
                &request,
                &mut result,
                &mut variables,
                payload.data_row.clone(),
                payload.iteration.unwrap_or(0),
                payload.iteration_count.unwrap_or(1),
                script_timeout,
                &secrets,
                &mut script_next_request,
                &mut script_stop_all,
            )
            .await;
        }
        if let Some(pre_vars) = pre_output
            .get("extractedEnvVars")
            .and_then(Value::as_object)
        {
            if !result.get("extractedEnvVars").is_some_and(Value::is_object) {
                result["extractedEnvVars"] = json!({});
            }
            if let Some(target) = result["extractedEnvVars"].as_object_mut() {
                target.extend(
                    pre_vars
                        .iter()
                        .map(|(key, value)| (key.clone(), value.clone())),
                );
            }
        }
        if let Some(target) = script_next_request {
            result["nextRequest"] = json!(target);
        }
        result["stopAll"] = json!(script_stop_all);
        mask_script_logs(&mut result, &secrets);
        return Ok(Json(result));
    }
    if matches!(
        request.get("protocol").and_then(Value::as_str),
        Some("websocket" | "sse")
    ) || request
        .get("method")
        .and_then(Value::as_str)
        .is_some_and(|method| matches!(method, "WS" | "SSE"))
    {
        let timeout_ms = payload.timeout_ms.filter(|ms| *ms > 0).or_else(|| {
            request
                .pointer("/settings/timeoutMs")
                .and_then(Value::as_u64)
                .filter(|ms| *ms > 0)
        });
        let mut result = stream_api::execute(&request, timeout_ms).await;
        apply_extractors(&request, &mut result);
        if let Some(extracted) = result.get("extractedEnvVars").and_then(Value::as_object) {
            variables.extend(extracted.iter().filter_map(|(key, value)| {
                value.as_str().map(|value| (key.clone(), value.to_owned()))
            }));
        }
        for field in ["logs", "consoleLogs"] {
            append_script_array(&mut result, field, &pre_output[field]);
        }
        if !result.get("logs").is_some_and(Value::is_array) {
            result["logs"] = json!([]);
        }
        if !result.get("consoleLogs").is_some_and(Value::is_array) {
            result["consoleLogs"] = json!([]);
        }
        for entry in &post_scripts {
            if payload.trusted == Some(false) {
                let source = entry
                    .get("source")
                    .and_then(Value::as_str)
                    .unwrap_or("request");
                result["logs"].as_array_mut().unwrap().push(json!(format!(
                    "[WARN] Post-response script ({source}) blocked: workspace is not trusted"
                )));
                result["consoleLogs"].as_array_mut().unwrap().push(json!({
                    "timestamp": chrono::Utc::now().to_rfc3339(), "level": "warn", "source": source,
                    "message": "Post-response script blocked: workspace is not trusted"
                }));
                continue;
            }
            run_post_script(
                entry,
                &request,
                &mut result,
                &mut variables,
                payload.data_row.clone(),
                payload.iteration.unwrap_or(0),
                payload.iteration_count.unwrap_or(1),
                script_timeout,
                &secrets,
                &mut script_next_request,
                &mut script_stop_all,
            )
            .await;
        }
        if let Some(pre_vars) = pre_output
            .get("extractedEnvVars")
            .and_then(Value::as_object)
        {
            if !result.get("extractedEnvVars").is_some_and(Value::is_object) {
                result["extractedEnvVars"] = json!({});
            }
            if let Some(target) = result["extractedEnvVars"].as_object_mut() {
                target.extend(
                    pre_vars
                        .iter()
                        .map(|(key, value)| (key.clone(), value.clone())),
                );
            }
        }
        if let Some(target) = script_next_request {
            result["nextRequest"] = json!(target);
        }
        result["stopAll"] = json!(script_stop_all);
        mask_script_logs(&mut result, &secrets);
        return Ok(Json(result));
    }
    if request
        .get("protocol")
        .and_then(Value::as_str)
        .is_some_and(|protocol| protocol != "http")
        || request
            .get("method")
            .and_then(Value::as_str)
            .is_some_and(|method| matches!(method, "WS" | "SSE"))
    {
        return Ok(execution_failure(
            "WebSocket and SSE requests must use the persistent stream API.",
        ));
    }
    let method_text = request
        .get("method")
        .and_then(Value::as_str)
        .unwrap_or("GET");
    let method = match reqwest::Method::from_bytes(method_text.as_bytes()) {
        Ok(method) => method,
        Err(error) => return Ok(execution_failure(format!("invalid HTTP method: {error}"))),
    };
    let raw_url = request
        .get("url")
        .and_then(Value::as_str)
        .unwrap_or("")
        .trim();
    let url_text = if raw_url.starts_with("http://") || raw_url.starts_with("https://") {
        raw_url.to_owned()
    } else {
        format!("http://{raw_url}")
    };
    let mut url = match reqwest::Url::parse(&url_text) {
        Ok(url) => url,
        Err(error) => return Ok(execution_failure(format!("invalid request URL: {error}"))),
    };
    let params: Vec<(String, String)> = request
        .get("params")
        .and_then(Value::as_array)
        .into_iter()
        .flatten()
        .filter(|item| {
            item.get("enabled")
                .and_then(Value::as_bool)
                .unwrap_or(false)
        })
        .filter_map(|item| {
            Some((
                item.get("key")?.as_str()?.to_owned(),
                item.get("value")
                    .and_then(Value::as_str)
                    .unwrap_or("")
                    .to_owned(),
            ))
        })
        .filter(|(key, _)| !key.is_empty())
        .collect();
    if !params.is_empty() {
        url.query_pairs_mut().extend_pairs(params.iter());
    }

    let auth = request
        .get("auth")
        .cloned()
        .unwrap_or_else(|| json!({ "type": "none" }));
    let auth_type = auth.get("type").and_then(Value::as_str).unwrap_or("none");
    let auth_key = auth.get("key").and_then(Value::as_str).unwrap_or("");
    let auth_value = auth.get("value").and_then(Value::as_str).unwrap_or("");
    if auth_type.eq_ignore_ascii_case("apiKey")
        && auth.get("addTo").and_then(Value::as_str) == Some("query")
        && !auth_key.is_empty()
    {
        url.query_pairs_mut().append_pair(auth_key, auth_value);
    }
    let settings = request.get("settings").cloned().unwrap_or(Value::Null);
    let timeout = payload
        .timeout_ms
        .filter(|ms| *ms > 0)
        .or_else(|| {
            settings
                .get("timeoutMs")
                .and_then(Value::as_u64)
                .filter(|ms| *ms > 0)
        })
        .unwrap_or(30_000);
    let connect_timeout = settings
        .get("connectTimeoutMs")
        .and_then(Value::as_u64)
        .filter(|ms| *ms > 0)
        .unwrap_or(10_000);
    let mut client_builder = reqwest::Client::builder()
        .timeout(Duration::from_millis(timeout))
        .connect_timeout(Duration::from_millis(connect_timeout))
        .danger_accept_invalid_certs(
            !settings
                .get("verifySSL")
                .and_then(Value::as_bool)
                .unwrap_or(true),
        );
    client_builder = if settings
        .get("followRedirects")
        .and_then(Value::as_bool)
        .unwrap_or(true)
    {
        client_builder.redirect(reqwest::redirect::Policy::limited(
            settings
                .get("maxRedirects")
                .and_then(Value::as_u64)
                .unwrap_or(10) as usize,
        ))
    } else {
        client_builder.redirect(reqwest::redirect::Policy::none())
    };
    if let Some(proxy) = settings
        .get("proxyUrl")
        .and_then(Value::as_str)
        .filter(|proxy| !proxy.is_empty())
    {
        let proxy_url = if proxy.contains("://") {
            proxy.to_owned()
        } else {
            format!("http://{proxy}")
        };
        match reqwest::Proxy::all(proxy_url) {
            Ok(proxy) => client_builder = client_builder.proxy(proxy),
            Err(error) => return Ok(execution_failure(format!("invalid proxy URL: {error}"))),
        }
    }
    if let (Some(cert_path), Some(key_path)) = (
        settings
            .get("clientCertPath")
            .and_then(Value::as_str)
            .filter(|path| !path.is_empty()),
        settings
            .get("clientKeyPath")
            .and_then(Value::as_str)
            .filter(|path| !path.is_empty()),
    ) {
        let cert = match fs::read(cert_path) {
            Ok(cert) => cert,
            Err(error) => {
                return Ok(execution_failure(format!(
                    "failed to read client certificate: {error}"
                )));
            }
        };
        let key = match fs::read(key_path) {
            Ok(key) => key,
            Err(error) => {
                return Ok(execution_failure(format!(
                    "failed to read client private key: {error}"
                )));
            }
        };
        let mut identity = cert;
        identity.push(b'\n');
        identity.extend_from_slice(&key);
        match reqwest::Identity::from_pem(&identity) {
            Ok(identity) => client_builder = client_builder.identity(identity),
            Err(error) => {
                return Ok(execution_failure(format!(
                    "invalid client certificate or private key: {error}"
                )));
            }
        }
    }
    let client = match client_builder.build() {
        Ok(client) => client,
        Err(error) => return Ok(execution_failure(error.to_string())),
    };
    let cookie_host = url.host_str().unwrap_or("").to_owned();
    let cookie_path = url.path().to_owned();
    let cookie_secure = url.scheme() == "https";
    let mut builder = client.request(method, url);
    let user_agent = settings
        .get("userAgent")
        .and_then(Value::as_str)
        .filter(|agent| !agent.is_empty())
        .unwrap_or("PebblePost/1.0");
    let request_headers = request.get("headers").and_then(Value::as_array);
    let has_user_agent = request_headers.is_some_and(|headers| {
        headers.iter().any(|header| {
            header
                .get("enabled")
                .and_then(Value::as_bool)
                .unwrap_or(false)
                && header
                    .get("key")
                    .and_then(Value::as_str)
                    .is_some_and(|key| key.trim().eq_ignore_ascii_case("user-agent"))
        })
    });
    let has_accept = request_headers.is_some_and(|headers| {
        headers.iter().any(|header| {
            header
                .get("enabled")
                .and_then(Value::as_bool)
                .unwrap_or(false)
                && header
                    .get("key")
                    .and_then(Value::as_str)
                    .is_some_and(|key| key.trim().eq_ignore_ascii_case("accept"))
        })
    });
    if !has_user_agent {
        builder = builder.header(reqwest::header::USER_AGENT, user_agent);
    }
    if !has_accept {
        builder = builder.header(reqwest::header::ACCEPT, "*/*");
    }
    if let Some(headers) = request_headers {
        for item in headers.iter().filter(|item| {
            item.get("enabled")
                .and_then(Value::as_bool)
                .unwrap_or(false)
        }) {
            let key = item.get("key").and_then(Value::as_str).unwrap_or("");
            let value = item.get("value").and_then(Value::as_str).unwrap_or("");
            if !key.is_empty() {
                builder = builder.header(key, value);
            }
        }
    }
    let oauth2_token = if auth_type.eq_ignore_ascii_case("oauth2") {
        oauth2_api::get_token(&auth).await.ok().and_then(|token| {
            token
                .get("access_token")
                .and_then(Value::as_str)
                .filter(|value| !value.trim().is_empty())
                .map(str::to_owned)
        })
    } else {
        None
    };
    match auth_type.to_ascii_lowercase().as_str() {
        "bearer" => {
            let token = auth
                .get("token")
                .and_then(Value::as_str)
                .unwrap_or("")
                .trim();
            if !token.is_empty() {
                builder = if token.to_ascii_lowercase().starts_with("bearer ") {
                    builder.header(reqwest::header::AUTHORIZATION, token)
                } else {
                    builder.bearer_auth(token)
                };
            }
        }
        "basic" => {
            builder = builder.basic_auth(
                auth.get("username").and_then(Value::as_str).unwrap_or(""),
                Some(auth.get("password").and_then(Value::as_str).unwrap_or("")),
            )
        }
        "oauth2" => {
            if let Some(token) = oauth2_token.as_deref().or_else(|| {
                auth.get("token")
                    .and_then(Value::as_str)
                    .filter(|token| !token.trim().is_empty())
            }) {
                let token = token.trim();
                builder = if token.to_ascii_lowercase().starts_with("bearer ") {
                    builder.header(reqwest::header::AUTHORIZATION, token)
                } else {
                    builder.bearer_auth(token)
                };
            }
        }
        "apikey"
            if auth.get("addTo").and_then(Value::as_str) != Some("query")
                && !auth_key.is_empty() =>
        {
            builder = builder.header(auth_key, auth_value)
        }
        "digest" | "awssigv4" => {}
        _ => {}
    }
    let cookies_enabled = settings
        .get("enableCookies")
        .and_then(Value::as_bool)
        .unwrap_or(true);
    if cookies_enabled && let Some(workspace) = payload.workspace_path.as_deref() {
        let cookies = cookies_for_request(
            Path::new(workspace),
            &cookie_host,
            &cookie_path,
            cookie_secure,
        )
        .unwrap_or_default();
        let header = cookies
            .iter()
            .filter_map(|cookie| {
                Some(format!(
                    "{}={}",
                    cookie.get("name")?.as_str()?,
                    cookie.get("value").and_then(Value::as_str).unwrap_or("")
                ))
            })
            .collect::<Vec<_>>()
            .join("; ");
        if !header.is_empty() {
            builder = builder.header(reqwest::header::COOKIE, header);
        }
    }

    let body = request
        .get("body")
        .cloned()
        .unwrap_or_else(|| json!({ "type": "none" }));
    let body_type = body.get("type").and_then(Value::as_str).unwrap_or("none");
    let mut sent_body = String::new();
    let file_reference = body
        .get("filePath")
        .and_then(Value::as_str)
        .filter(|path| !path.trim().is_empty())
        .or_else(|| {
            if matches!(body_type, "formData" | "urlEncoded") {
                return None;
            }
            body.get("raw")
                .and_then(Value::as_str)
                .filter(|raw| raw.trim_start().starts_with("@file:"))
        })
        .or_else(|| {
            if body_type == "file" {
                body.get("raw")
                    .and_then(Value::as_str)
                    .filter(|path| !path.trim().is_empty())
            } else {
                None
            }
        });
    let custom_content_type = request_headers.is_some_and(|headers| {
        headers.iter().any(|header| {
            header
                .get("enabled")
                .and_then(Value::as_bool)
                .unwrap_or(false)
                && header
                    .get("key")
                    .and_then(Value::as_str)
                    .is_some_and(|key| key.trim().eq_ignore_ascii_case("content-type"))
        })
    });
    if let Some(file_reference) = file_reference {
        let Some(workspace) = payload.workspace_path.as_deref() else {
            return Ok(execution_failure("file bodies require a workspace root"));
        };
        let path = file_reference
            .trim()
            .strip_prefix("@file:")
            .unwrap_or(file_reference.trim())
            .trim();
        let safe_path = match safe_workspace_path(Path::new(workspace), Path::new(path)) {
            Ok(path) => path,
            Err(error) => return Ok(execution_failure(error.to_string())),
        };
        let bytes = match fs::read(&safe_path) {
            Ok(bytes) => bytes,
            Err(error) => {
                return Ok(execution_failure(format!(
                    "could not read request body file: {error}"
                )));
            }
        };
        let content_type = match body_type {
            "json" => "application/json",
            "raw" | "text" => "text/plain",
            "xml" => "application/xml",
            _ => "application/octet-stream",
        };
        sent_body = format!("[file: {} ({} bytes)]", safe_path.display(), bytes.len());
        if !custom_content_type {
            builder = builder.header(reqwest::header::CONTENT_TYPE, content_type);
        }
        builder = builder.body(bytes);
    } else {
        match body_type {
            "raw" | "json" => {
                sent_body = body
                    .get("raw")
                    .and_then(Value::as_str)
                    .unwrap_or("")
                    .to_owned();
                if !custom_content_type {
                    let content_type = if body_type == "json" {
                        "application/json"
                    } else {
                        "text/plain"
                    };
                    builder = builder.header(reqwest::header::CONTENT_TYPE, content_type);
                }
                if !sent_body.is_empty() {
                    builder = builder.body(sent_body.clone());
                }
            }
            "urlEncoded" => {
                let fields: Vec<(String, String)> = body
                    .get("urlEncoded")
                    .and_then(Value::as_array)
                    .into_iter()
                    .flatten()
                    .filter(|item| {
                        item.get("enabled")
                            .and_then(Value::as_bool)
                            .unwrap_or(false)
                    })
                    .filter_map(|item| {
                        Some((
                            item.get("key")?.as_str()?.to_owned(),
                            item.get("value")
                                .and_then(Value::as_str)
                                .unwrap_or("")
                                .to_owned(),
                        ))
                    })
                    .filter(|(key, _)| !key.is_empty())
                    .collect();
                let mut form_url =
                    reqwest::Url::parse("http://localhost").expect("static URL is valid");
                form_url.query_pairs_mut().extend_pairs(fields.iter());
                sent_body = form_url.query().unwrap_or("").to_owned();
                if !custom_content_type {
                    builder = builder.header(
                        reqwest::header::CONTENT_TYPE,
                        "application/x-www-form-urlencoded",
                    );
                }
                builder = builder.body(sent_body.clone());
            }
            "formData" => {
                let fields = body
                    .get("formData")
                    .and_then(Value::as_array)
                    .cloned()
                    .unwrap_or_default();
                if auth_type.eq_ignore_ascii_case("digest")
                    || auth_type.eq_ignore_ascii_case("awssigv4")
                {
                    let (bytes, content_type) =
                        match buffered_multipart_body(&fields, payload.workspace_path.as_deref()) {
                            Ok(body) => body,
                            Err(error) => return Ok(execution_failure(error)),
                        };
                    builder = builder
                        .header(reqwest::header::CONTENT_TYPE, content_type)
                        .body(bytes);
                } else {
                    let mut form = reqwest::multipart::Form::new();
                    for item in fields.iter().filter(|item| {
                        item.get("enabled")
                            .and_then(Value::as_bool)
                            .unwrap_or(false)
                    }) {
                        let key = item.get("key").and_then(Value::as_str).unwrap_or("");
                        let value = item.get("value").and_then(Value::as_str).unwrap_or("");
                        if key.is_empty() {
                            continue;
                        }
                        if item.get("type").and_then(Value::as_str) == Some("file") {
                            let Some(workspace) = payload.workspace_path.as_deref() else {
                                return Ok(execution_failure(
                                    "file fields require a workspace root",
                                ));
                            };
                            let path = value.strip_prefix("@file:").unwrap_or(value).trim();
                            let safe_path =
                                match safe_workspace_path(Path::new(workspace), Path::new(path)) {
                                    Ok(path) => path,
                                    Err(error) => return Ok(execution_failure(error.to_string())),
                                };
                            let bytes = match fs::read(&safe_path) {
                                Ok(bytes) => bytes,
                                Err(error) => {
                                    return Ok(execution_failure(format!(
                                        "could not read upload file: {error}"
                                    )));
                                }
                            };
                            let filename = safe_path
                                .file_name()
                                .and_then(|name| name.to_str())
                                .unwrap_or("upload.bin")
                                .to_owned();
                            form = form.part(
                                key.to_owned(),
                                reqwest::multipart::Part::bytes(bytes).file_name(filename),
                            );
                        } else {
                            form = form.text(key.to_owned(), value.to_owned());
                        }
                    }
                    builder = builder.multipart(form);
                }
            }
            "graphql" => {
                let graphql = body.get("graphql").cloned().unwrap_or(Value::Null);
                let graphql_body = if graphql.is_null() {
                    json!({})
                } else {
                    let mut graphql_body = serde_json::Map::new();
                    graphql_body.insert(
                        "query".into(),
                        json!(graphql.get("query").and_then(Value::as_str).unwrap_or("")),
                    );
                    if let Some(variables) = graphql
                        .get("variables")
                        .and_then(Value::as_str)
                        .filter(|value| !value.trim().is_empty())
                    {
                        let variables = serde_json::from_str::<Value>(variables)
                            .unwrap_or_else(|_| json!(variables));
                        graphql_body.insert("variables".into(), variables);
                    }
                    if let Some(operation_name) = graphql
                        .get("operationName")
                        .and_then(Value::as_str)
                        .filter(|value| !value.trim().is_empty())
                    {
                        graphql_body.insert("operationName".into(), json!(operation_name));
                    }
                    Value::Object(graphql_body)
                };
                sent_body = graphql_body.to_string();
                if !custom_content_type {
                    builder = builder.header(reqwest::header::CONTENT_TYPE, "application/json");
                }
                builder = builder.body(sent_body.clone());
            }
            _ => {}
        }
    }

    let mut outgoing = match builder.build() {
        Ok(request) => request,
        Err(error) => {
            return Ok(execution_failure(format!(
                "failed to build request: {error}"
            )));
        }
    };
    if auth_type.eq_ignore_ascii_case("awsSigV4")
        && let Err(error) = http_auth::sign_aws_sigv4(&mut outgoing, &auth)
    {
        return Ok(execution_failure(error));
    }
    let sent_method = outgoing.method().to_string();
    let sent_url = outgoing.url().to_string();
    let mut sent_headers = http_auth::request_headers_json(&outgoing);
    let digest_retry_template = if auth_type.eq_ignore_ascii_case("digest") {
        outgoing.try_clone()
    } else {
        None
    };
    let started = std::time::Instant::now();
    let mut response = match client.execute(outgoing).await {
        Ok(response) => response,
        Err(error) => return Ok(execution_network_failure(error.to_string())),
    };
    if response.status() == reqwest::StatusCode::UNAUTHORIZED
        && auth_type.eq_ignore_ascii_case("digest")
    {
        let challenge = response
            .headers()
            .get(reqwest::header::WWW_AUTHENTICATE)
            .and_then(|value| value.to_str().ok())
            .map(str::to_owned);
        if let (Some(challenge), Some(mut retry)) = (challenge, digest_retry_template) {
            let body_bytes = match retry.body() {
                Some(body) => body.as_bytes(),
                None => Some(&[][..]),
            };
            let uri = format!(
                "{}{}{}",
                retry.url().path(),
                if retry.url().query().is_some() {
                    "?"
                } else {
                    ""
                },
                retry.url().query().unwrap_or("")
            );
            if let Ok(authorization) = http_auth::build_digest_authorization(
                retry.method().as_str(),
                &uri,
                auth.get("username").and_then(Value::as_str).unwrap_or(""),
                auth.get("password").and_then(Value::as_str).unwrap_or(""),
                &challenge,
                body_bytes,
            ) && let Ok(value) = reqwest::header::HeaderValue::from_str(&authorization)
            {
                retry
                    .headers_mut()
                    .insert(reqwest::header::AUTHORIZATION, value);
                sent_headers = http_auth::request_headers_json(&retry);
                drop(response);
                response = match client.execute(retry).await {
                    Ok(response) => response,
                    Err(error) => return Ok(execution_network_failure(error.to_string())),
                };
            }
        }
    }
    let status = response.status();
    let status_code = status.as_u16();
    let status_text = status.canonical_reason().unwrap_or("").to_owned();
    let mut response_headers = serde_json::Map::new();
    for (key, _) in response.headers() {
        let values = response
            .headers()
            .get_all(key)
            .iter()
            .map(|value| value.to_str().unwrap_or("").to_owned())
            .collect::<Vec<_>>();
        response_headers.insert(key.as_str().to_owned(), json!(values));
    }
    if cookies_enabled && let Some(workspace) = payload.workspace_path.as_deref() {
        for set_cookie in response
            .headers()
            .get_all(reqwest::header::SET_COOKIE)
            .iter()
            .filter_map(|value| value.to_str().ok())
        {
            if let Some(cookie) = parse_set_cookie(set_cookie, &cookie_host, &cookie_path) {
                if cookie
                    .get("maxAge")
                    .and_then(Value::as_i64)
                    .is_some_and(|age| age <= 0)
                    || cookie
                        .get("expires")
                        .and_then(Value::as_str)
                        .and_then(|date| chrono::DateTime::parse_from_rfc3339(date).ok())
                        .is_some_and(|date| date.with_timezone(&chrono::Utc) <= chrono::Utc::now())
                {
                    let domain = cookie.get("domain").and_then(Value::as_str);
                    let path = cookie.get("path").and_then(Value::as_str);
                    let name = cookie.get("name").and_then(Value::as_str);
                    let _ = delete_cookies(Path::new(workspace), domain, path, name);
                } else {
                    let _ = save_cookie(Path::new(workspace), cookie);
                }
            }
        }
    }
    let mut inline_body = Vec::new();
    let mut total_size = 0_u64;
    let mut temp_file: Option<(std::fs::File, PathBuf)> = None;
    const MAX_RESPONSE_BODY_BYTES: usize = 50 * 1024 * 1024;
    loop {
        let chunk = match response.chunk().await {
            Ok(Some(chunk)) => chunk,
            Ok(None) => break,
            Err(error) => {
                return Ok(execution_network_failure(format!(
                    "failed while reading response: {error}"
                )));
            }
        };
        let remaining = MAX_RESPONSE_BODY_BYTES.saturating_sub(total_size as usize);
        if remaining == 0 {
            break;
        }
        let chunk = &chunk[..chunk.len().min(remaining)];
        total_size += chunk.len() as u64;
        if temp_file.is_none() && total_size <= 5 * 1024 * 1024 {
            inline_body.extend_from_slice(chunk);
        } else {
            if temp_file.is_none() {
                cleanup_expired_response_files();
                let id = TEMP_BODY_ID.fetch_add(1, AtomicOrdering::Relaxed);
                let path = std::env::temp_dir()
                    .join(format!("pebble-body-{}-{id}.bin", std::process::id()));
                let mut options = fs::OpenOptions::new();
                options.write(true).create_new(true);
                #[cfg(unix)]
                {
                    use std::os::unix::fs::OpenOptionsExt;
                    options.mode(0o600);
                }
                let mut file = options.open(&path)?;
                file.write_all(&inline_body)?;
                temp_file = Some((file, path));
            }
            if let Some((file, _)) = temp_file.as_mut() {
                file.write_all(chunk)?;
            }
            if inline_body.len() < 5 * 1024 * 1024 {
                let remaining = 5 * 1024 * 1024 - inline_body.len();
                inline_body.extend_from_slice(&chunk[..chunk.len().min(remaining)]);
            }
        }
    }
    let elapsed = started.elapsed().as_secs_f64() * 1000.0;
    let body_text = String::from_utf8_lossy(&inline_body).into_owned();
    let temp_path = temp_file.map(|(file, path)| {
        drop(file);
        path.to_string_lossy().into_owned()
    });
    let mut result = json!({
        "statusCode": status_code, "statusText": status_text, "headers": response_headers, "body": body_text,
        "bodyTruncated": temp_path.is_some(), "bodySizeBytes": total_size, "size": total_size,
        "timing": { "dnsLookupMs": 0, "tcpConnectMs": 0, "tlsHandshakeMs": 0, "ttfbMs": elapsed, "downloadMs": 0, "totalDurationMs": elapsed },
        "sentRequest": { "method": sent_method, "url": sent_url, "headers": sent_headers, "body": sent_body },
        "tests": [], "logs": pre_output["logs"].clone(), "consoleLogs": pre_output["consoleLogs"].clone(),
        "executedAt": chrono::Utc::now().to_rfc3339(),
    });
    if let Some(path) = temp_path {
        result["tempBodyFile"] = json!(path);
    }
    if let Some(pre_vars) = pre_output
        .get("extractedEnvVars")
        .and_then(Value::as_object)
    {
        result["extractedEnvVars"] = Value::Object(pre_vars.clone());
    }
    apply_extractors(&request, &mut result);
    if let Some(extracted) = result.get("extractedEnvVars").and_then(Value::as_object) {
        variables.extend(extracted.iter().filter_map(|(key, value)| {
            value.as_str().map(|value| (key.clone(), value.to_owned()))
        }));
    }
    for entry in &post_scripts {
        if payload.trusted == Some(false) {
            let source = entry
                .get("source")
                .and_then(Value::as_str)
                .unwrap_or("request");
            result["logs"].as_array_mut().unwrap().push(json!(format!(
                "[WARN] Post-response script ({source}) blocked: workspace is not trusted"
            )));
            result["consoleLogs"].as_array_mut().unwrap().push(json!({
                "timestamp": chrono::Utc::now().to_rfc3339(), "level": "warn", "source": source,
                "message": "Post-response script blocked: workspace is not trusted"
            }));
            continue;
        }
        run_post_script(
            entry,
            &request,
            &mut result,
            &mut variables,
            payload.data_row.clone(),
            payload.iteration.unwrap_or(0),
            payload.iteration_count.unwrap_or(1),
            script_timeout,
            &secrets,
            &mut script_next_request,
            &mut script_stop_all,
        )
        .await;
    }
    if let Some(target) = script_next_request {
        result["nextRequest"] = json!(target);
    }
    result["stopAll"] = json!(script_stop_all);
    mask_script_logs(&mut result, &secrets);
    if let Some(workspace) = payload.workspace_path.as_deref()
        && let Err(error) = state
            .history
            .record(workspace, &request, &result, &secrets)
            .await
    {
        eprintln!("failed to record request history: {error}");
    }
    Ok(Json(result))
}

fn apply_extractors(request: &Value, result: &mut Value) {
    let Some(extractors) = request.get("extractors").and_then(Value::as_array) else {
        return;
    };
    if extractors.is_empty() {
        return;
    }
    let body = result
        .get("body")
        .and_then(Value::as_str)
        .unwrap_or_default();
    let response_headers = result.get("headers");
    let (extraction_results, variables, warnings) =
        pebblepost_core::extractor::extract_all_with_response(extractors, body, response_headers);
    result["extractorResults"] =
        serde_json::to_value(extraction_results).unwrap_or_else(|_| json!([]));
    if !variables.is_empty() {
        let existing = result
            .get("extractedEnvVars")
            .and_then(Value::as_object)
            .cloned()
            .unwrap_or_default();
        let mut combined = existing;
        combined.extend(
            variables
                .into_iter()
                .map(|(key, value)| (key, json!(value))),
        );
        result["extractedEnvVars"] = Value::Object(combined);
    }
    for warning in warnings {
        if let Some(logs) = result.get_mut("logs").and_then(Value::as_array_mut) {
            logs.push(json!(format!("[WARN] {warning}")));
        }
        let logs = result
            .as_object_mut()
            .map(|object| object.entry("consoleLogs").or_insert_with(|| json!([])));
        if let Some(Value::Array(logs)) = logs {
            logs.push(json!({
                "timestamp": chrono::Utc::now().to_rfc3339(),
                "level": "warn",
                "source": "extractor",
                "message": warning,
            }));
        }
    }
}

async fn response_body(Query(query): Query<ResponseBodyQuery>) -> Result<Json<Value>, ApiError> {
    let requested = Path::new(&query.file);
    if requested
        .file_name()
        .and_then(|name| name.to_str())
        .is_none_or(|name| !name.starts_with("pebble-body-"))
    {
        return Err(ApiError(PebbleError::InvalidPath(
            "invalid temporary body reference".into(),
        )));
    }
    let temp_dir = fs::canonicalize(std::env::temp_dir()).map_err(PebbleError::from)?;
    let canonical = fs::canonicalize(requested).map_err(PebbleError::from)?;
    if canonical.parent() != Some(temp_dir.as_path())
        || canonical
            .file_name()
            .and_then(|name| name.to_str())
            .is_none_or(|name| !name.starts_with("pebble-body-"))
    {
        return Err(ApiError(PebbleError::InvalidPath(
            "body file is outside the temporary directory".into(),
        )));
    }
    let mut file = fs::File::open(&canonical)?;
    let total_size = file.metadata()?.len();
    let offset = query.offset.unwrap_or(0).min(total_size);
    let limit = query
        .limit
        .filter(|limit| *limit > 0 && *limit <= 5 * 1024 * 1024)
        .unwrap_or(5 * 1024 * 1024);
    file.seek(SeekFrom::Start(offset))?;
    let mut chunk = vec![
        0;
        limit.min(
            total_size
                .saturating_sub(offset)
                .try_into()
                .unwrap_or(usize::MAX)
        ) as usize
    ];
    let bytes_read = file.read(&mut chunk)?;
    chunk.truncate(bytes_read);
    let has_more = offset + (bytes_read as u64) < total_size;
    drop(file);
    if !has_more {
        let _ = fs::remove_file(canonical);
    }
    Ok(Json(
        json!({ "chunk": String::from_utf8_lossy(&chunk), "offset": offset, "chunkSize": bytes_read, "totalSize": total_size, "hasMore": has_more }),
    ))
}

fn cleanup_expired_response_files() {
    let Ok(entries) = fs::read_dir(std::env::temp_dir()) else {
        return;
    };
    let expiry = std::time::SystemTime::now() - Duration::from_secs(24 * 60 * 60);
    for entry in entries.flatten() {
        if !entry
            .file_name()
            .to_string_lossy()
            .starts_with("pebble-body-")
        {
            continue;
        }
        if entry
            .metadata()
            .ok()
            .and_then(|metadata| metadata.modified().ok())
            .is_some_and(|modified| modified < expiry)
        {
            let _ = fs::remove_file(entry.path());
        }
    }
}

fn history_error(error: String) -> ApiError {
    ApiError(PebbleError::Io(std::io::Error::other(error)))
}

async fn history_list(
    State(state): State<Arc<AppState>>,
    Query(query): Query<HistoryQuery>,
) -> Result<Json<Value>, ApiError> {
    if query.workspace.is_empty() {
        return Err(ApiError(PebbleError::InvalidPath(
            "workspace parameter is required".into(),
        )));
    }
    let page = query
        .page
        .as_deref()
        .and_then(|value| value.parse::<usize>().ok())
        .filter(|page| *page > 0)
        .unwrap_or(1);
    let limit = query
        .limit
        .as_deref()
        .and_then(|value| value.parse::<usize>().ok())
        .filter(|limit| *limit > 0)
        .unwrap_or(50);
    let status = query
        .status
        .as_deref()
        .and_then(|value| value.parse::<i64>().ok())
        .filter(|status| *status > 0);
    let (entries, total) = state
        .history
        .list(
            &query.workspace,
            page,
            limit,
            status,
            query.method.as_deref(),
            query.search.as_deref(),
            query.since.as_deref(),
        )
        .await
        .map_err(history_error)?;
    Ok(Json(
        json!({ "entries": entries, "total": total, "page": page, "limit": limit }),
    ))
}

async fn history_get(
    State(state): State<Arc<AppState>>,
    AxumPath(id): AxumPath<i64>,
) -> Result<Json<Value>, ApiError> {
    match state.history.get(id).await.map_err(history_error)? {
        Some(entry) => Ok(Json(entry)),
        None => Err(ApiError(PebbleError::Io(std::io::Error::from(
            std::io::ErrorKind::NotFound,
        )))),
    }
}

async fn history_delete(
    State(state): State<Arc<AppState>>,
    AxumPath(id): AxumPath<i64>,
) -> Result<Json<Value>, ApiError> {
    state.history.delete(id).await.map_err(history_error)?;
    Ok(Json(json!({ "status": "deleted" })))
}

async fn history_clear(
    State(state): State<Arc<AppState>>,
    Query(query): Query<HistoryQuery>,
) -> Result<Json<Value>, ApiError> {
    if query.workspace.is_empty() {
        return Err(ApiError(PebbleError::InvalidPath(
            "workspace parameter is required".into(),
        )));
    }
    state
        .history
        .clear(&query.workspace)
        .await
        .map_err(history_error)?;
    Ok(Json(json!({ "status": "cleared" })))
}

async fn run_git(workspace: &str, args: &[String]) -> Result<String, String> {
    if workspace.trim().is_empty() {
        return Err("workspace path is required".into());
    }
    let root = fs::canonicalize(workspace).map_err(|error| error.to_string())?;
    if !root.is_dir() {
        return Err("workspace path is not a directory".into());
    }
    let mut command = Command::new("git");
    command.arg("-C").arg(root).args(args).kill_on_drop(true);
    let output = tokio::time::timeout(Duration::from_secs(30), command.output())
        .await
        .map_err(|_| "git command timed out after 30 seconds".to_owned())?
        .map_err(|error| format!("could not run git: {error}"))?;
    if output.status.success() {
        return Ok(String::from_utf8_lossy(&output.stdout).into_owned());
    }
    let stderr = String::from_utf8_lossy(&output.stderr).trim().to_owned();
    let stdout = String::from_utf8_lossy(&output.stdout).trim().to_owned();
    Err(if stderr.is_empty() { stdout } else { stderr })
}

fn git_error(error: String) -> ApiError {
    ApiError(PebbleError::InvalidPath(error))
}

async fn git_is_repo(workspace: &str) -> bool {
    run_git(
        workspace,
        &["rev-parse".into(), "--is-inside-work-tree".into()],
    )
    .await
    .is_ok_and(|value| value.trim() == "true")
}

async fn git_status(Query(query): Query<GitQuery>) -> Result<Json<Value>, ApiError> {
    if !git_is_repo(&query.workspace_path).await {
        return Ok(Json(
            json!({ "isRepo": false, "branch": "", "ahead": 0, "behind": 0, "dirty": false, "files": [] }),
        ));
    }
    let branch = run_git(
        &query.workspace_path,
        &["branch".into(), "--show-current".into()],
    )
    .await
    .unwrap_or_default()
    .trim()
    .to_owned();
    let branch = if branch.is_empty() {
        run_git(
            &query.workspace_path,
            &["rev-parse".into(), "--short".into(), "HEAD".into()],
        )
        .await
        .map(|short| format!("detached@{}", short.trim()))
        .unwrap_or_else(|_| "main".into())
    } else {
        branch
    };
    let counts = run_git(
        &query.workspace_path,
        &[
            "rev-list".into(),
            "--left-right".into(),
            "--count".into(),
            "HEAD...@{upstream}".into(),
        ],
    )
    .await
    .unwrap_or_default();
    let mut count_parts = counts.split_whitespace();
    let ahead = count_parts
        .next()
        .and_then(|value| value.parse::<i64>().ok())
        .unwrap_or(0);
    let behind = count_parts
        .next()
        .and_then(|value| value.parse::<i64>().ok())
        .unwrap_or(0);
    let output = run_git(
        &query.workspace_path,
        &["status".into(), "--porcelain=v1".into(), "-uall".into()],
    )
    .await
    .unwrap_or_default();
    let files: Vec<Value> = output
        .lines()
        .filter(|line| line.len() >= 3)
        .filter_map(|line| {
            let chars: Vec<char> = line.chars().take(2).collect();
            if chars.len() < 2 {
                return None;
            }
            let mut path = line
                .get(3..)
                .unwrap_or("")
                .trim()
                .trim_matches('"')
                .to_owned();
            let mut old = String::new();
            if let Some((before, after)) = path.split_once(" -> ") {
                old = before.trim_matches('"').to_owned();
                path = after.trim_matches('"').to_owned();
            }
            if path.is_empty() {
                return None;
            }
            let code: String = chars.iter().collect::<String>().trim().to_owned();
            let staged = chars[0] != ' ' && chars[0] != '?';
            Some(if old.is_empty() {
                json!({ "path": path, "code": code, "staged": staged })
            } else {
                json!({ "path": path, "code": code, "staged": staged, "old": old })
            })
        })
        .collect();
    Ok(Json(
        json!({ "isRepo": true, "branch": branch, "ahead": ahead, "behind": behind, "dirty": !files.is_empty(), "files": files }),
    ))
}

async fn git_branches(Query(query): Query<GitQuery>) -> Result<Json<Value>, ApiError> {
    if !git_is_repo(&query.workspace_path).await {
        return Err(git_error("not a git repository".into()));
    }
    let output = run_git(
        &query.workspace_path,
        &[
            "branch".into(),
            "--list".into(),
            "--no-color".into(),
            "-vv".into(),
        ],
    )
    .await
    .map_err(git_error)?;
    let branches: Vec<Value> = output
        .lines()
        .filter_map(|line| {
            let current = line.starts_with('*');
            let content = line.trim().trim_start_matches('*').trim();
            let name = content.split_whitespace().next()?;
            let remote = content.find('[').and_then(|start| {
                content[start + 1..]
                    .find(']')
                    .map(|end| content[start + 1..start + 1 + end].to_owned())
            });
            Some(json!({ "name": name, "current": current, "remote": remote.unwrap_or_default() }))
        })
        .collect();
    Ok(Json(json!(branches)))
}

fn validate_git_relative_path(path: &str) -> Result<(), ApiError> {
    let path = Path::new(path);
    if path.as_os_str().is_empty()
        || path.is_absolute()
        || path
            .components()
            .any(|part| !matches!(part, std::path::Component::Normal(_)))
    {
        return Err(git_error(
            "invalid path: it must stay inside the workspace".into(),
        ));
    }
    Ok(())
}

fn is_protected_git_secret(path: &str) -> bool {
    Path::new(path)
        .file_name()
        .and_then(|name| name.to_str())
        .is_some_and(|name| name.ends_with(".secret.env.json"))
}

async fn git_diff(Query(query): Query<GitQuery>) -> Result<Json<Value>, ApiError> {
    if !git_is_repo(&query.workspace_path).await {
        return Err(git_error("not a git repository".into()));
    }
    let path = query
        .path
        .as_deref()
        .filter(|path| !path.is_empty())
        .or(query.file.as_deref())
        .unwrap_or("");
    validate_git_relative_path(path)?;
    let root =
        fs::canonicalize(&query.workspace_path).map_err(|error| git_error(error.to_string()))?;
    let head = run_git(
        &query.workspace_path,
        &["show".into(), format!("HEAD:{path}")],
    )
    .await
    .unwrap_or_default();
    let working = fs::read(root.join(path))
        .map(|bytes| String::from_utf8_lossy(&bytes).into_owned())
        .unwrap_or_default();
    if head == working {
        return Ok(Json(json!([])));
    }
    if path.ends_with(".pebble.json") || path.ends_with(".request.json") {
        let before = serde_json::from_str::<Value>(&head).ok();
        let after = serde_json::from_str::<Value>(&working).ok();
        if before.is_some() || after.is_some() {
            let before = before.unwrap_or_else(|| json!({}));
            let after = after.unwrap_or_else(|| json!({}));
            let mut entries = Vec::new();
            for field in ["name", "method", "url", "headers", "body", "scripts"] {
                let old = before.get(field).cloned().unwrap_or(Value::Null);
                let new = after.get(field).cloned().unwrap_or(Value::Null);
                let before_text = format_git_diff_field(field, &old);
                let after_text = format_git_diff_field(field, &new);
                if before_text != after_text {
                    entries.push(json!({
                        "field": field,
                        "before": before_text,
                        "after": after_text,
                        "type": if field == "headers" { "kv" } else if field == "body" { "json" } else if field == "scripts" { "js" } else { "text" }
                    }));
                }
            }
            if !entries.is_empty() {
                return Ok(Json(json!(entries)));
            }
        }
    }
    Ok(Json(
        json!([{ "field": "content", "before": head, "after": working, "type": "text" }]),
    ))
}

fn format_git_diff_field(field: &str, value: &Value) -> String {
    match field {
        "name" | "method" | "url" => value.as_str().unwrap_or("").to_owned(),
        "headers" => format_git_key_values(value.get("headers").unwrap_or(value)),
        "body" => {
            let body = value.get("body").unwrap_or(value);
            let body_type = body.get("type").and_then(Value::as_str).unwrap_or("");
            if matches!(body_type, "" | "none") {
                return String::new();
            }
            if let Some(raw) = body
                .get("raw")
                .and_then(Value::as_str)
                .filter(|raw| !raw.is_empty())
            {
                return raw.to_owned();
            }
            for key in ["formData", "urlEncoded"] {
                if let Some(values) = body
                    .get(key)
                    .and_then(Value::as_array)
                    .filter(|values| !values.is_empty())
                {
                    return format_git_key_values(&Value::Array(values.clone()));
                }
            }
            body.pointer("/graphql/query")
                .and_then(Value::as_str)
                .unwrap_or("")
                .to_owned()
        }
        "scripts" => {
            let scripts = value.get("scripts").unwrap_or(value);
            let mut parts = Vec::new();
            if let Some(source) = scripts
                .get("preRequest")
                .and_then(Value::as_str)
                .filter(|source| !source.is_empty())
            {
                parts.push(format!("// Pre-Request Script\n{source}"));
            }
            if let Some(source) = scripts
                .get("postResponse")
                .and_then(Value::as_str)
                .filter(|source| !source.is_empty())
            {
                parts.push(format!("// Post-Response Script\n{source}"));
            }
            parts.join("\n\n")
        }
        _ if value.is_string() => value.as_str().unwrap_or_default().to_owned(),
        _ if value.is_null() => String::new(),
        _ => serde_json::to_string_pretty(value).unwrap_or_default(),
    }
}

fn format_git_key_values(value: &Value) -> String {
    let Some(rows) = value.as_array() else {
        return String::new();
    };
    if rows.is_empty() {
        return String::new();
    }
    let mut output = String::from("[\n");
    for (index, row) in rows.iter().enumerate() {
        if index > 0 {
            output.push_str(",\n");
        }
        output.push_str("  {\n");
        let key = serde_json::to_string(row.get("key").and_then(Value::as_str).unwrap_or(""))
            .unwrap_or_else(|_| "\"\"".into());
        let value = serde_json::to_string(row.get("value").and_then(Value::as_str).unwrap_or(""))
            .unwrap_or_else(|_| "\"\"".into());
        let enabled = row.get("enabled").and_then(Value::as_bool).unwrap_or(false);
        output.push_str(&format!(
            "    \"key\": {key},\n    \"value\": {value},\n    \"enabled\": {enabled}"
        ));
        if let Some(kind) = row
            .get("type")
            .and_then(Value::as_str)
            .filter(|kind| !kind.is_empty())
        {
            output.push_str(&format!(
                ",\n    \"type\": {}",
                serde_json::to_string(kind).unwrap_or_default()
            ));
        }
        if let Some(secret) = row
            .get("secret")
            .and_then(Value::as_bool)
            .filter(|secret| *secret)
        {
            output.push_str(&format!(",\n    \"secret\": {secret}"));
        }
        output.push_str("\n  }");
    }
    output.push_str("\n]");
    output
}

async fn git_log(Query(query): Query<GitQuery>) -> Result<Json<Value>, ApiError> {
    if !git_is_repo(&query.workspace_path).await {
        return Err(git_error("not a git repository".into()));
    }
    let limit = query.limit.filter(|limit| *limit > 0).unwrap_or(20);
    let output = run_git(
        &query.workspace_path,
        &[
            "log".into(),
            format!("-n{limit}"),
            "--pretty=format:%H%x09%h%x09%an%x09%ad%x09%s".into(),
            "--date=short".into(),
        ],
    )
    .await
    .unwrap_or_default();
    let logs: Vec<Value> = output.lines().filter_map(|line| {
        let parts: Vec<&str> = line.splitn(5, '\t').collect();
        (parts.len() == 5).then(|| json!({ "hash": parts[0], "short": parts[1], "author": parts[2], "date": parts[3], "subject": parts[4] }))
    }).collect();
    Ok(Json(json!(logs)))
}

async fn git_stage(
    Query(query): Query<GitQuery>,
    Json(payload): Json<Value>,
) -> Result<Json<Value>, ApiError> {
    if !git_is_repo(&query.workspace_path).await {
        return Err(git_error("not a git repository".into()));
    }
    let paths = git_payload_paths(&payload)?;
    for path in &paths {
        validate_git_relative_path(path)?;
        if is_protected_git_secret(path) {
            return Err(git_error(format!(
                "refusing to stage protected secret file {path:?}: secret files must never be committed"
            )));
        }
        if run_git(
            &query.workspace_path,
            &["check-ignore".into(), "--".into(), path.clone()],
        )
        .await
        .is_ok_and(|output| !output.trim().is_empty())
        {
            return Err(git_error(format!(
                "refusing to stage ignored file {path:?}: protected by .gitignore"
            )));
        }
    }
    if !paths.is_empty() {
        let mut args = vec!["add".to_owned(), "--".to_owned()];
        args.extend(paths);
        run_git(&query.workspace_path, &args)
            .await
            .map_err(git_error)?;
    }
    Ok(Json(json!({ "success": true })))
}

async fn git_unstage(
    Query(query): Query<GitQuery>,
    Json(payload): Json<Value>,
) -> Result<Json<Value>, ApiError> {
    if !git_is_repo(&query.workspace_path).await {
        return Err(git_error("not a git repository".into()));
    }
    let paths = git_payload_paths(&payload)?;
    for path in &paths {
        validate_git_relative_path(path)?;
    }
    if !paths.is_empty() {
        let mut args = vec!["restore".to_owned(), "--staged".to_owned(), "--".to_owned()];
        args.extend(paths);
        run_git(&query.workspace_path, &args)
            .await
            .map_err(git_error)?;
    }
    Ok(Json(json!({ "success": true })))
}

fn git_payload_paths(payload: &Value) -> Result<Vec<String>, ApiError> {
    payload
        .get("paths")
        .and_then(Value::as_array)
        .ok_or_else(|| git_error("paths must be an array".into()))?
        .iter()
        .map(|path| {
            path.as_str()
                .map(str::to_owned)
                .ok_or_else(|| git_error("every path must be a string".into()))
        })
        .collect()
}

async fn git_commit(
    Query(query): Query<GitQuery>,
    Json(payload): Json<Value>,
) -> Result<Json<Value>, ApiError> {
    if !git_is_repo(&query.workspace_path).await {
        return Err(git_error("not a git repository".into()));
    }
    let message = payload
        .get("message")
        .and_then(Value::as_str)
        .unwrap_or("")
        .trim();
    if message.is_empty() {
        return Err(git_error("commit message cannot be empty".into()));
    }
    run_git(
        &query.workspace_path,
        &["commit".into(), "-m".into(), message.to_owned()],
    )
    .await
    .map_err(git_error)?;
    Ok(Json(json!({ "success": true })))
}

async fn git_checkout(
    Query(query): Query<GitQuery>,
    Json(payload): Json<Value>,
) -> Result<Json<Value>, ApiError> {
    if !git_is_repo(&query.workspace_path).await {
        return Err(git_error("not a git repository".into()));
    }
    let branch = payload
        .get("branch")
        .and_then(Value::as_str)
        .unwrap_or("")
        .trim();
    validate_git_branch_name(&query.workspace_path, branch).await?;
    let dirty = git_status(Query(GitQuery {
        workspace_path: query.workspace_path.clone(),
        path: None,
        file: None,
        limit: None,
    }))
    .await?
    .0["dirty"]
        == true;
    let stashed = if dirty {
        run_git(
            &query.workspace_path,
            &[
                "stash".into(),
                "push".into(),
                "-u".into(),
                "-m".into(),
                format!("pebblepost-auto-stash-{}", chrono::Utc::now().timestamp()),
            ],
        )
        .await
        .map_err(|error| {
            git_error(format!(
                "failed to auto-stash changes before checkout: {error}"
            ))
        })?;
        true
    } else {
        false
    };
    if let Err(error) = run_git(
        &query.workspace_path,
        &["checkout".into(), branch.to_owned()],
    )
    .await
    {
        if stashed {
            let _ = run_git(&query.workspace_path, &["stash".into(), "pop".into()]).await;
        }
        return Err(git_error(format!(
            "failed to checkout branch {branch:?}: {error}"
        )));
    }
    let message = if stashed {
        match run_git(&query.workspace_path, &["stash".into(), "pop".into()]).await {
            Ok(_) => {
                format!("Switched to {branch} (uncommitted changes auto-stashed and restored)")
            }
            Err(error) => format!("Switched to {branch} (stash popped with conflicts: {error})"),
        }
    } else {
        format!("Switched to {branch}")
    };
    Ok(Json(
        json!({ "success": true, "message": message, "stashed": stashed }),
    ))
}

async fn git_create_branch(
    Query(query): Query<GitQuery>,
    Json(payload): Json<Value>,
) -> Result<Json<Value>, ApiError> {
    let name = payload
        .get("name")
        .and_then(Value::as_str)
        .unwrap_or("")
        .trim();
    if !git_is_repo(&query.workspace_path).await {
        return Err(git_error("not a git repository".into()));
    }
    validate_git_branch_name(&query.workspace_path, name).await?;
    run_git(
        &query.workspace_path,
        &["checkout".into(), "-b".into(), name.to_owned()],
    )
    .await
    .map_err(git_error)?;
    Ok(Json(json!({ "success": true, "name": name })))
}

async fn validate_git_branch_name(workspace: &str, branch: &str) -> Result<(), ApiError> {
    if branch.trim().is_empty() {
        return Err(git_error("branch name cannot be empty".into()));
    }
    if branch.chars().any(|character| {
        matches!(
            character,
            ';' | '|' | '&' | '$' | '`' | '"' | '\'' | '<' | '>'
        )
    }) {
        return Err(git_error(format!(
            "invalid branch name {branch:?}: contains illegal shell characters"
        )));
    }
    run_git(
        workspace,
        &[
            "check-ref-format".into(),
            "--branch".into(),
            branch.to_owned(),
        ],
    )
    .await
    .map_err(|error| git_error(format!("invalid git branch name {branch:?}: {error}")))?;
    Ok(())
}

fn translate_git_error(error: &str) -> String {
    let lower = error.to_ascii_lowercase();
    if lower.contains("authentication failed") || lower.contains("permission denied (publickey)") {
        "Git authentication failed. Please configure your SSH keys or git credential helper.".into()
    } else if lower.contains("conflict") {
        "Merge conflict detected. Please resolve conflicts before continuing.".into()
    } else if lower.contains("non-fast-forward") || lower.contains("fetch first") {
        "Push rejected: remote has newer commits. Please pull before pushing.".into()
    } else {
        error.to_owned()
    }
}

async fn git_pull(Query(query): Query<GitQuery>) -> Response {
    if !git_is_repo(&query.workspace_path).await {
        return (
            StatusCode::BAD_REQUEST,
            Json(json!({ "success": false, "output": "", "error": "not a git repository" })),
        )
            .into_response();
    }
    match run_git(&query.workspace_path, &["pull".into()]).await {
        Ok(output) => Json(json!({ "success": true, "output": output })).into_response(),
        Err(error) => (
            StatusCode::BAD_REQUEST,
            Json(json!({ "success": false, "output": "", "error": translate_git_error(&error) })),
        )
            .into_response(),
    }
}

async fn git_push(Query(query): Query<GitQuery>) -> Response {
    if !git_is_repo(&query.workspace_path).await {
        return (
            StatusCode::BAD_REQUEST,
            Json(json!({ "success": false, "output": "", "error": "not a git repository" })),
        )
            .into_response();
    }
    let status = git_status(Query(GitQuery {
        workspace_path: query.workspace_path.clone(),
        path: None,
        file: None,
        limit: None,
    }))
    .await;
    let mut args = vec!["push".to_owned()];
    if let Ok(status) = status
        && status.0["ahead"].as_i64().unwrap_or(0) > 0
        && status.0["behind"].as_i64().unwrap_or(0) == 0
    {
        args.extend([
            "-u".into(),
            "origin".into(),
            status.0["branch"].as_str().unwrap_or("").to_owned(),
        ]);
    }
    match run_git(&query.workspace_path, &args).await {
        Ok(output) => Json(json!({ "success": true, "output": output })).into_response(),
        Err(error) => (
            StatusCode::BAD_REQUEST,
            Json(json!({ "success": false, "output": "", "error": translate_git_error(&error) })),
        )
            .into_response(),
    }
}

async fn git_init(Query(query): Query<GitQuery>) -> Result<Json<Value>, ApiError> {
    run_git(&query.workspace_path, &["init".into()])
        .await
        .map_err(git_error)?;
    Ok(Json(json!({ "success": true })))
}

async fn auth_middleware(
    State(state): State<Arc<AppState>>,
    request: Request<axum::body::Body>,
    next: Next,
) -> Response {
    let path = request.uri().path();
    if request.method() == Method::OPTIONS
        || path == "/api/health"
        || path == "/health"
        || !path.starts_with("/api/")
        || state.token.is_empty()
    {
        return next.run(request).await;
    }
    let bearer = request
        .headers()
        .get(header::AUTHORIZATION)
        .and_then(|value| value.to_str().ok())
        .and_then(|value| value.strip_prefix("Bearer "));
    let query_token_matches = request
        .uri()
        .query()
        .and_then(|query| reqwest::Url::parse(&format!("http://localhost/?{query}")).ok())
        .is_some_and(|url| {
            url.query_pairs()
                .any(|(key, value)| key == "token" && value.as_ref() == state.token.as_str())
        });
    if bearer == Some(state.token.as_str()) || query_token_matches {
        next.run(request).await
    } else {
        (
            StatusCode::UNAUTHORIZED,
            Json(json!({ "error": "unauthorized" })),
        )
            .into_response()
    }
}

async fn security_headers(request: Request<axum::body::Body>, next: Next) -> Response {
    let mut response = next.run(request).await;
    let headers = response.headers_mut();
    headers.insert(
        "x-content-type-options",
        HeaderValue::from_static("nosniff"),
    );
    headers.insert("x-frame-options", HeaderValue::from_static("DENY"));
    headers.insert(
        "referrer-policy",
        HeaderValue::from_static("strict-origin-when-cross-origin"),
    );
    headers.insert(
        "content-security-policy",
        HeaderValue::from_static("default-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob:"),
    );
    response
}

async fn host_header(request: Request<axum::body::Body>, next: Next) -> Response {
    let health_path = request.uri().path() == "/api/health";
    let host = request
        .headers()
        .get(header::HOST)
        .and_then(|value| value.to_str().ok())
        .unwrap_or("");
    if health_path || is_local_host(host) {
        next.run(request).await
    } else {
        (
            StatusCode::FORBIDDEN,
            Json(json!({ "error": "forbidden: invalid host header" })),
        )
            .into_response()
    }
}

struct ApiError(PebbleError);

impl From<PebbleError> for ApiError {
    fn from(error: PebbleError) -> Self {
        Self(error)
    }
}

impl From<std::io::Error> for ApiError {
    fn from(error: std::io::Error) -> Self {
        Self(PebbleError::Io(error))
    }
}

impl From<serde_json::Error> for ApiError {
    fn from(error: serde_json::Error) -> Self {
        Self(PebbleError::Json(error))
    }
}

impl IntoResponse for ApiError {
    fn into_response(self) -> Response {
        let message = self.0.to_string();
        let status = match &self.0 {
            PebbleError::InvalidPath(_) => StatusCode::BAD_REQUEST,
            PebbleError::Io(error) if error.kind() == std::io::ErrorKind::NotFound => {
                StatusCode::NOT_FOUND
            }
            _ => StatusCode::INTERNAL_SERVER_ERROR,
        };
        (status, Json(json!({ "error": message }))).into_response()
    }
}

fn is_local_origin(origin: &str) -> bool {
    let lower = origin.to_ascii_lowercase();
    if matches!(
        lower.as_str(),
        "tauri://localhost" | "http://tauri.localhost"
    ) {
        return true;
    }
    [
        "http://localhost",
        "https://localhost",
        "http://127.0.0.1",
        "https://127.0.0.1",
    ]
    .iter()
    .any(|prefix| {
        lower.strip_prefix(prefix).is_some_and(|suffix| {
            suffix.is_empty() || suffix.starts_with(':') || suffix.starts_with('/')
        })
    })
}

fn is_local_host(host: &str) -> bool {
    let normalized = host.trim().to_ascii_lowercase();
    let host = if normalized.starts_with('[') {
        let end = normalized.find(']').unwrap_or(normalized.len());
        normalized.get(1..end).unwrap_or("")
    } else if normalized.matches(':').count() == 1 {
        normalized.split(':').next().unwrap_or("")
    } else {
        normalized.as_str()
    };
    host == "localhost" || host.ends_with(".localhost") || host == "127.0.0.1" || host == "::1"
}

fn content_type(path: &Path) -> &'static str {
    match path
        .extension()
        .and_then(|extension| extension.to_str())
        .unwrap_or("")
    {
        "html" => "text/html; charset=utf-8",
        "js" | "mjs" => "text/javascript; charset=utf-8",
        "css" => "text/css; charset=utf-8",
        "json" | "map" => "application/json; charset=utf-8",
        "svg" => "image/svg+xml",
        "png" => "image/png",
        "jpg" | "jpeg" => "image/jpeg",
        "webp" => "image/webp",
        "ico" => "image/x-icon",
        "woff" => "font/woff",
        "woff2" => "font/woff2",
        _ => "application/octet-stream",
    }
}

#[cfg(test)]
mod runner_parity_tests {
    use super::*;

    #[test]
    fn runner_glob_matches_wildcards_ranges_escapes_and_path_boundaries() {
        assert!(glob_match(
            "users/*.pebble.json",
            "users/01-list.pebble.json"
        ));
        assert!(!glob_match("users/*.json", "users/admin/01-list.json"));
        assert!(glob_match("[0-9][0-9]-*.json", "02-create.json"));
        assert!(glob_match(r"literal\?.json", "literal?.json"));
        assert!(!glob_match("[0-9]-*.json", "x-create.json"));
    }

    #[test]
    fn runner_filters_combine_name_folder_and_tag_conditions() {
        let request = json!({ "name": "Create user", "tags": ["Auth", "smoke"] });
        assert!(runner_request_matches(
            &request,
            "users/create.pebble.json",
            "create*",
            "users",
            "auth"
        ));
        assert!(!runner_request_matches(
            &request,
            "users/create.pebble.json",
            "create*",
            "admin",
            "auth"
        ));
        assert!(!runner_request_matches(
            &request,
            "users/create.pebble.json",
            "create*",
            "users",
            "regression"
        ));
    }

    #[test]
    fn runner_data_parser_accepts_wrapped_json_records() {
        let rows = parse_runner_json(r#"{"items":[null,{"name":"Ada"}]}"#).unwrap();
        assert_eq!(rows, vec![json!({ "name": "Ada" })]);
        assert!(parse_runner_json(r#"[{"name":"Ada"},null]"#).is_err());
    }

    #[test]
    fn runner_report_preserves_assertion_failure_details() {
        let error = runner_result_error(&json!({
            "tests": [{ "name": "status", "passed": false, "message": "expected 200 to equal 404" }]
        }))
        .unwrap();
        assert_eq!(error, "status: expected 200 to equal 404");
    }

    #[tokio::test]
    async fn runner_dry_run_visits_requests_in_order_and_reports_skipped_steps() {
        let nonce = chrono::Utc::now().timestamp_nanos_opt().unwrap_or_default();
        let root = std::env::temp_dir().join(format!(
            "pebblepost-runner-parity-{}-{nonce}",
            std::process::id()
        ));
        std::fs::create_dir_all(&root).unwrap();
        let first = pebblepost_core::save_request(
            &root,
            &root.join("01-first.pebble.json"),
            json!({
                "name": "Get token", "method": "GET", "url": "https://example.test/first",
                "auth": {"type":"none"}, "headers": [], "body": {"type":"none"}
            }),
        )
        .unwrap();
        let second = pebblepost_core::save_request(
            &root,
            &root.join("02-user.pebble.json"),
            json!({
                "name": "Get user", "method": "GET", "url": "https://example.test/users/{{token}}",
                "auth": {"type":"none"}, "headers": [], "body": {"type":"none"}
            }),
        )
        .unwrap();
        let data_dir = root.join("runner-state");
        std::fs::create_dir_all(&data_dir).unwrap();
        let app = Arc::new(AppState {
            token: String::new(),
            frontend_dir: PathBuf::new(),
            history: Arc::new(HistoryStore::open(&data_dir).unwrap()),
            active_runs: Arc::new(Mutex::new(HashMap::new())),
            mock_manager: Arc::new(mock_api::MockManager::default()),
            stream_manager: Arc::new(stream_api::StreamManager::default()),
            grpc_manager: Arc::new(grpc_api::GrpcManager::default()),
            updates: Arc::new(update_api::UpdateService::new(&data_dir)),
        });
        let payload: RunnerPayload = serde_json::from_value(json!({
            "workspacePath": root,
            "requestPaths": [first.to_string_lossy(), second.to_string_lossy()],
            "dryRun": true,
            "iterations": 2,
            "dataRows": [{"token":"first"},{"token":"second"}]
        }))
        .unwrap();
        let (stop_sender, stop) = oneshot::channel();
        let mut runner = RunnerStream {
            app,
            payload,
            run_id: "dry-run-test".into(),
            stop,
            iteration: 1,
            request_index: 0,
            executions: 0,
            max_executions: 10,
            runtime_vars: HashMap::new(),
            results: Vec::new(),
            started: Instant::now(),
            final_summary: None,
            terminal_stage: 0,
            bailed: false,
            needs_delay: false,
            announced: false,
        };
        assert!(runner_next(&mut runner).await.is_some()); // start
        for _ in 0..4 {
            assert!(runner_next(&mut runner).await.is_some());
        }
        assert!(runner_next(&mut runner).await.is_some()); // summary
        let summary = runner.final_summary.take().unwrap();
        assert_eq!(summary["dryRun"], true);
        assert_eq!(summary["totalIterations"], 2);
        assert_eq!(summary["totalRequests"], 4);
        assert_eq!(summary["skippedRequests"], 4);
        assert_eq!(summary["iterations"][0]["dataRow"]["token"], "first");
        assert_eq!(summary["iterations"][1]["dataRow"]["token"], "second");
        assert_eq!(summary["results"][0]["relPath"], "01-first.pebble.json");
        assert_eq!(summary["results"][1]["relPath"], "02-user.pebble.json");
        assert_eq!(summary["results"][2]["iteration"], 2);
        assert_eq!(summary["results"][0]["skipped"], true);
        drop(stop_sender);
        drop(runner);
        std::fs::remove_dir_all(root).unwrap();
    }

    #[cfg(unix)]
    #[test]
    fn runner_relative_paths_handle_symlinked_workspace_roots() {
        let root = std::env::temp_dir().join(format!(
            "pebblepost-runner-paths-{}-{}",
            std::process::id(),
            chrono::Utc::now().timestamp_nanos_opt().unwrap_or_default()
        ));
        let actual_root = root.join("actual");
        let alias_root = root.join("alias");
        std::fs::create_dir_all(&actual_root).unwrap();
        std::os::unix::fs::symlink(&actual_root, &alias_root).unwrap();
        let request = actual_root.join("01-first.pebble.json");
        std::fs::write(&request, "{}").unwrap();

        assert_eq!(
            runner_relative_path(&alias_root, &request),
            "01-first.pebble.json"
        );

        std::fs::remove_dir_all(root).unwrap();
    }
}
