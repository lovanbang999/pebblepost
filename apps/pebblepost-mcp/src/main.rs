use std::collections::HashMap;
use std::path::{Component, Path, PathBuf};
use std::sync::Arc;

use clap::Parser;
use pebblepost_core::{
    get_workspace_info, list_environments, read_request, safe_workspace_path, save_request,
    scan_workspace, validate_workspace_root,
};
use pebblepost_server::{LocalMock, run_local_api, run_local_runner};
use rmcp::{
    ServerHandler, ServiceExt, handler::server::router::tool::ToolRouter,
    handler::server::wrapper::Parameters, model::*, schemars, tool, tool_handler, tool_router,
    transport::stdio,
};
use schemars::JsonSchema;
use serde::Deserialize;
use serde_json::{Map, Value, json};
use tokio::sync::Mutex;

#[derive(Debug, Parser)]
#[command(
    name = "pebblepost-mcp",
    version,
    about = "MCP server for PebblePost workspaces"
)]
struct Args {
    /// PebblePost workspace made available to the connected MCP client.
    #[arg(long, default_value = ".")]
    workspace: PathBuf,
}

#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
struct PathInput {
    /// Workspace-relative request file, such as collections/api/users.pebble.json.
    path: String,
}

#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
struct ListRequestsInput {
    /// Optional workspace-relative collection folder. Defaults to all collections.
    collection_path: Option<String>,
}

#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
struct SaveRequestInput {
    /// Workspace-relative destination under collections/. The .pebble.json suffix is added if omitted.
    path: String,
    /// Complete PebblePost request JSON object.
    request: Value,
}

#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
struct EnvironmentInput {
    /// Optional environment name. Defaults to the active environment.
    environment_name: Option<String>,
}

#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
struct SendRequestInput {
    /// Workspace-relative request file under collections/.
    path: String,
    /// Optional environment name. Defaults to the active environment.
    environment_name: Option<String>,
    /// Optional request timeout in milliseconds, capped at 120 seconds.
    timeout_ms: Option<u64>,
}

#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
struct RunCollectionInput {
    /// Workspace-relative folder under collections/.
    target_path: String,
    /// Optional request file paths. When omitted, run every request in target_path.
    request_paths: Option<Vec<String>>,
    /// Optional environment name. Defaults to the active environment.
    environment_name: Option<String>,
    /// Run without sending requests when true.
    dry_run: Option<bool>,
    /// Number of iterations, clamped to 1–20.
    iterations: Option<usize>,
    /// Delay between iterations in milliseconds, capped at 60 seconds.
    delay_ms: Option<u64>,
    /// Stop after the first failed assertion or request.
    bail: Option<bool>,
    /// Request timeout in milliseconds, capped at 120 seconds.
    timeout_ms: Option<u64>,
    /// Retry count, clamped to 0–5.
    retry_count: Option<usize>,
}

#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
struct GenerateAssertionsInput {
    /// Workspace-relative request file used in the latest send_request call.
    path: String,
    /// Optional JSON field paths (dot notation) to assert are present in the response.
    json_paths: Option<Vec<String>>,
}

#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
struct SyncOpenApiInput {
    /// One of: diff, apply, or link.
    action: String,
    /// Workspace-relative collection folder.
    folder_path: String,
    /// OpenAPI URL or a spec file path relative to the collection folder.
    spec_location: Option<String>,
    /// For apply, explicit endpoint IDs returned by diff. Must not be empty.
    accepted_endpoints: Option<Vec<String>>,
    /// Whether to overwrite conflicting edits during apply. Defaults to false.
    force: Option<bool>,
}

#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
struct StartMockInput {
    /// Workspace-relative collection folder or request folder with saved response examples.
    target_path: String,
    /// Local loopback port. Defaults to 8080.
    port: Option<u16>,
}

#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
struct MockOverrideInput {
    /// Route ID from start_mock_server's routes list.
    route_id: String,
    /// Mock response override, for example {"statusCode":200,"body":{"ok":true}}.
    response: Value,
}

#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
struct MockLogsInput {
    /// Return logs with an ID greater than this cursor.
    cursor: Option<u64>,
}

#[derive(Clone)]
struct PebblePostMcp {
    tool_router: ToolRouter<Self>,
    workspace: PathBuf,
    mock: Arc<Mutex<Option<LocalMock>>>,
    last_responses: Arc<Mutex<HashMap<String, Value>>>,
}

impl PebblePostMcp {
    fn new(workspace: PathBuf) -> Self {
        Self {
            tool_router: Self::tool_router(),
            workspace,
            mock: Arc::new(Mutex::new(None)),
            last_responses: Arc::new(Mutex::new(HashMap::new())),
        }
    }

    fn request_path(&self, path: &str, add_suffix: bool) -> Result<PathBuf, String> {
        let path = clean_collection_path(path, false)?;
        let mut path = PathBuf::from(path);
        if add_suffix && !path.to_string_lossy().ends_with(".pebble.json") {
            let file_name = path
                .file_name()
                .and_then(|part| part.to_str())
                .ok_or_else(|| "request path must include a file name".to_owned())?;
            if file_name.ends_with(".json") {
                return Err("request paths must use the .pebble.json suffix".into());
            }
            path.set_file_name(format!("{file_name}.pebble.json"));
        } else if !add_suffix && !path.to_string_lossy().ends_with(".pebble.json") {
            return Err("request paths must use the .pebble.json suffix".into());
        }
        if path
            .file_name()
            .and_then(|part| part.to_str())
            .is_some_and(|name| name == "_folder.pebble.json")
        {
            return Err("folder metadata cannot be used as a request".into());
        }
        Ok(path)
    }

    fn collection_folder(&self, path: &str) -> Result<PathBuf, String> {
        let relative = clean_collection_path(path, true)?;
        let absolute = safe_workspace_path(&self.workspace, Path::new(&relative))
            .map_err(|error| error.to_string())?;
        if !absolute.is_dir() {
            return Err("collection_path must point to a folder".into());
        }
        Ok(absolute)
    }

    fn checked_spec_location(&self, folder: &Path, location: &str) -> Result<String, String> {
        let location = location.trim();
        if location.starts_with("https://") || location.starts_with("http://") {
            return Ok(location.to_owned());
        }
        let relative = Path::new(location);
        if location.is_empty()
            || relative.is_absolute()
            || location.contains('\\')
            || relative
                .components()
                .any(|part| !matches!(part, Component::Normal(_)))
        {
            return Err(
                "local OpenAPI spec paths must be relative to the collection folder".into(),
            );
        }
        let candidate = folder.join(relative);
        safe_workspace_path(&self.workspace, &candidate).map_err(|error| error.to_string())?;
        Ok(relative.to_string_lossy().into_owned())
    }
}

#[tool_router(router = tool_router)]
impl PebblePostMcp {
    /// List the top-level collections in the configured PebblePost workspace.
    #[tool(
        name = "list_collections",
        description = "List collections and request counts in the configured PebblePost workspace."
    )]
    async fn list_collections(&self) -> Result<rmcp::Json<Value>, String> {
        let scan = scan_workspace(&self.workspace).map_err(|error| error.to_string())?;
        let collections = scan
            .tree
            .iter()
            .filter(|node| node.is_dir)
            .map(|node| {
                json!({
                    "name": node.display_name,
                    "path": node.rel_path,
                    "requestCount": count_requests(&node.children),
                })
            })
            .collect::<Vec<_>>();
        Ok(rmcp::Json(json!({ "collections": collections })))
    }

    /// List request files, optionally within one collection folder.
    #[tool(
        name = "list_requests",
        description = "List saved requests by workspace-relative path, method, and name. Can be limited to one collection folder."
    )]
    async fn list_requests(
        &self,
        Parameters(input): Parameters<ListRequestsInput>,
    ) -> Result<rmcp::Json<Value>, String> {
        let scan = scan_workspace(&self.workspace).map_err(|error| error.to_string())?;
        let prefix = if let Some(path) = input.collection_path.as_deref() {
            let folder = self.collection_folder(path)?;
            let relative = folder
                .strip_prefix(&self.workspace)
                .map_err(|_| "collection folder is outside the workspace".to_owned())?;
            Some(relative.to_string_lossy().replace('\\', "/"))
        } else {
            None
        };
        let mut requests = Vec::new();
        collect_requests(&scan.tree, prefix.as_deref(), &mut requests);
        Ok(rmcp::Json(json!({ "requests": requests })))
    }

    /// Read a request file while redacting credentials and secret fields.
    #[tool(
        name = "get_request",
        description = "Read a PebblePost request. Secret values and sensitive fields are redacted in the returned JSON."
    )]
    async fn get_request(
        &self,
        Parameters(input): Parameters<PathInput>,
    ) -> Result<rmcp::Json<Value>, String> {
        let path = self.request_path(&input.path, false)?;
        let request = read_request(&self.workspace, &path).map_err(|error| error.to_string())?;
        Ok(rmcp::Json(redact_sensitive(request)))
    }

    /// Create or update a request in an existing collection folder.
    #[tool(
        name = "create_or_update_request",
        description = "Create or update one request JSON file under collections/. The parent collection folder must already exist."
    )]
    async fn create_or_update_request(
        &self,
        Parameters(input): Parameters<SaveRequestInput>,
    ) -> Result<rmcp::Json<Value>, String> {
        let path = self.request_path(&input.path, true)?;
        let request = input.request;
        if !request.is_object() {
            return Err("request must be a JSON object".into());
        }
        for field in ["name", "method", "url"] {
            if request
                .get(field)
                .and_then(Value::as_str)
                .is_none_or(|value| value.trim().is_empty())
            {
                return Err(format!("request.{field} must be a non-empty string"));
            }
        }
        save_request(&self.workspace, &path, request.clone()).map_err(|error| error.to_string())?;
        Ok(rmcp::Json(json!({
            "saved": true,
            "path": path.to_string_lossy().replace('\\', "/"),
            "name": request.get("name"),
            "method": request.get("method"),
        })))
    }

    /// Inspect environment names and variable metadata without returning values.
    #[tool(
        name = "inspect_environment",
        description = "List environment variable names, enabled/secret flags, and types without exposing any variable values."
    )]
    async fn inspect_environment(
        &self,
        input: Parameters<EnvironmentInput>,
    ) -> Result<rmcp::Json<Value>, String> {
        let workspace = get_workspace_info(&self.workspace).map_err(|error| error.to_string())?;
        let environments = list_environments(&self.workspace).map_err(|error| error.to_string())?;
        let selected = input
            .0
            .environment_name
            .as_deref()
            .unwrap_or(&workspace.active_environment);
        let environments = environments
            .into_iter()
            .filter(|environment| {
                input.0.environment_name.is_none()
                    || environment.name.eq_ignore_ascii_case(selected)
            })
            .map(|environment| {
                json!({
                    "name": environment.name,
                    "active": environment.name.eq_ignore_ascii_case(&workspace.active_environment),
                    "variables": environment.variables.into_iter().map(|variable| json!({
                        "key": variable.key,
                        "enabled": variable.enabled,
                        "secret": variable.secret,
                        "type": variable.r#type,
                    })).collect::<Vec<_>>(),
                })
            })
            .collect::<Vec<_>>();
        Ok(rmcp::Json(json!({
            "activeEnvironment": workspace.active_environment,
            "environments": environments,
        })))
    }

    /// Execute one saved request through PebblePost's request engine.
    #[tool(
        name = "send_request",
        description = "Execute a saved request using PebblePost's HTTP, gRPC, or stream request engine. Workspace trust settings remain in force; response secrets are redacted."
    )]
    async fn send_request(
        &self,
        Parameters(input): Parameters<SendRequestInput>,
    ) -> Result<rmcp::Json<Value>, String> {
        let path = self.request_path(&input.path, false)?;
        let absolute =
            safe_workspace_path(&self.workspace, &path).map_err(|error| error.to_string())?;
        let relative = absolute
            .strip_prefix(&self.workspace)
            .map_err(|_| "request path is outside the workspace".to_owned())?
            .to_string_lossy()
            .replace('\\', "/");
        let workspace = get_workspace_info(&self.workspace).map_err(|error| error.to_string())?;
        let request = read_request(&self.workspace, &path).map_err(|error| error.to_string())?;
        let response = run_local_api(
            "POST",
            "/api/request/execute",
            &[],
            Some(json!({
                "workspacePath": self.workspace,
                "path": relative,
                "request": request,
                "environmentName": input.environment_name,
                "trusted": workspace.trusted,
                "timeoutMs": input.timeout_ms.unwrap_or(30_000).clamp(1, 120_000),
            })),
        )
        .await?;
        let response = redact_sensitive(response);
        self.last_responses
            .lock()
            .await
            .insert(relative, response.clone());
        Ok(rmcp::Json(response))
    }

    /// Run a collection folder and return PebblePost's runner summary.
    #[tool(
        name = "run_collection",
        description = "Run all or selected requests from a collection folder using the PebblePost runner. Execution counts and timeouts are bounded."
    )]
    async fn run_collection(
        &self,
        Parameters(input): Parameters<RunCollectionInput>,
    ) -> Result<rmcp::Json<Value>, String> {
        let target = self.collection_folder(&input.target_path)?;
        let target_path = target.to_string_lossy().into_owned();
        let workspace = get_workspace_info(&self.workspace).map_err(|error| error.to_string())?;
        let mut request_paths = Vec::new();
        for request in input.request_paths.unwrap_or_default() {
            let relative = self.request_path(&request, false)?;
            let absolute = safe_workspace_path(&self.workspace, &relative)
                .map_err(|error| error.to_string())?;
            if !absolute.starts_with(&target) {
                return Err(format!(
                    "request {request:?} is outside the selected target folder"
                ));
            }
            request_paths.push(absolute.to_string_lossy().into_owned());
        }
        let response = run_local_runner(json!({
            "workspacePath": self.workspace,
            "targetPath": target_path,
            "requestPaths": request_paths,
            "environmentName": input.environment_name,
            "dryRun": input.dry_run.unwrap_or(false),
            "iterations": input.iterations.unwrap_or(1).clamp(1, 20),
            "delayMs": input.delay_ms.unwrap_or(0).min(60_000),
            "bail": input.bail.unwrap_or(false),
            "trusted": workspace.trusted,
            "timeoutMs": input.timeout_ms.unwrap_or(30_000).clamp(1, 120_000),
            "retryCount": input.retry_count.unwrap_or(0).min(5),
            "maxExecutions": 500,
        }))
        .await?;
        Ok(rmcp::Json(redact_sensitive(response)))
    }

    /// Generate and save post-response status and JSON field assertions from a prior send_request response.
    #[tool(
        name = "generate_assertions",
        description = "Generate PebblePost post-response tests from the last send_request result for a request path, then save them to scripts.postResponse."
    )]
    async fn generate_assertions(
        &self,
        Parameters(input): Parameters<GenerateAssertionsInput>,
    ) -> Result<rmcp::Json<Value>, String> {
        let path = self.request_path(&input.path, false)?;
        let absolute =
            safe_workspace_path(&self.workspace, &path).map_err(|error| error.to_string())?;
        let relative = absolute
            .strip_prefix(&self.workspace)
            .map_err(|_| "request path is outside the workspace".to_owned())?
            .to_string_lossy()
            .replace('\\', "/");
        let response = self
            .last_responses
            .lock()
            .await
            .get(&relative)
            .cloned()
            .ok_or_else(|| {
                "send this request with send_request before generating assertions".to_owned()
            })?;
        let status = response
            .get("statusCode")
            .and_then(Value::as_u64)
            .ok_or_else(|| {
                "the previous response did not include an HTTP status code".to_owned()
            })?;
        if !(100..=599).contains(&status) {
            return Err("the previous request did not return an HTTP response".into());
        }
        let body_json = response
            .get("body")
            .and_then(Value::as_str)
            .and_then(|body| serde_json::from_str::<Value>(body).ok());
        let mut script = format!(
            "pm.test(\"Response status is {status}\", () => {{\n  pm.expect(pm.response.status).toBe({status});\n}});"
        );
        for path in input.json_paths.unwrap_or_default() {
            let segments = validate_json_path(&path)?;
            if body_json
                .as_ref()
                .and_then(|body| value_at_path(body, &segments))
                .is_none()
            {
                return Err(format!(
                    "response JSON does not contain field path {path:?}"
                ));
            }
            let expression = segments
                .iter()
                .map(|segment| format!("[{segment:?}]"))
                .collect::<String>();
            script.push_str(&format!(
                "\n\npm.test(\"Response includes {}\", () => {{\n  pm.expect(pm.response.json(){expression}).toBeDefined();\n}});",
                path.replace('"', "\\\"")
            ));
        }
        let mut request =
            read_request(&self.workspace, &path).map_err(|error| error.to_string())?;
        let scripts = request
            .as_object_mut()
            .ok_or_else(|| "request JSON must be an object".to_owned())?
            .entry("scripts")
            .or_insert_with(|| json!({}));
        let scripts = scripts
            .as_object_mut()
            .ok_or_else(|| "request.scripts must be an object".to_owned())?;
        let existing = scripts
            .get("postResponse")
            .and_then(Value::as_str)
            .filter(|value| !value.trim().is_empty())
            .unwrap_or("");
        scripts.insert(
            "postResponse".into(),
            Value::String(if existing.is_empty() {
                script.clone()
            } else {
                format!("{existing}\n\n{script}")
            }),
        );
        save_request(&self.workspace, &path, request).map_err(|error| error.to_string())?;
        Ok(rmcp::Json(
            json!({ "saved": true, "path": relative, "generatedScript": script }),
        ))
    }

    /// Compare, link, or apply a selected set of OpenAPI changes to a collection.
    #[tool(
        name = "sync_openapi",
        description = "Compare or link an OpenAPI 3.x spec, or apply only explicitly selected endpoint IDs from a prior diff. Local spec files must be inside the collection folder."
    )]
    async fn sync_openapi(
        &self,
        Parameters(input): Parameters<SyncOpenApiInput>,
    ) -> Result<rmcp::Json<Value>, String> {
        let folder = self.collection_folder(&input.folder_path)?;
        let folder_text = folder.to_string_lossy().into_owned();
        let spec_location = input
            .spec_location
            .as_deref()
            .map(|location| self.checked_spec_location(&folder, location))
            .transpose()?;
        let result = match input.action.as_str() {
            "diff" => {
                let mut query = vec![("folder".into(), folder_text)];
                if let Some(spec) = spec_location {
                    query.push(("spec".into(), spec));
                }
                run_local_api("GET", "/api/sync/diff", &query, None).await?
            }
            "link" => {
                let spec =
                    spec_location.ok_or_else(|| "spec_location is required for link".to_owned())?;
                let response = run_local_api(
                    "POST",
                    "/api/sync/link",
                    &[],
                    Some(json!({ "folderPath": folder_text, "specLocation": spec })),
                )
                .await?;
                redact_sensitive(response)
            }
            "apply" => {
                let accepted = input.accepted_endpoints.unwrap_or_default();
                if accepted.is_empty() {
                    return Err(
                        "apply requires explicit accepted_endpoints from sync_openapi diff".into(),
                    );
                }
                run_local_api(
                    "POST",
                    "/api/sync/apply",
                    &[],
                    Some(json!({
                        "folderPath": folder_text,
                        "specLocation": spec_location,
                        "acceptedEndpoints": accepted,
                        "force": input.force.unwrap_or(false),
                    })),
                )
                .await?
            }
            action => {
                return Err(format!(
                    "unsupported sync action {action:?}; use diff, link, or apply"
                ));
            }
        };
        Ok(rmcp::Json(redact_sensitive(result)))
    }

    /// Start the example-based mock server on localhost for a collection folder.
    #[tool(
        name = "start_mock_server",
        description = "Start PebblePost's example-response mock server bound only to 127.0.0.1. The selected folder must contain saved response examples."
    )]
    async fn start_mock_server(
        &self,
        Parameters(input): Parameters<StartMockInput>,
    ) -> Result<rmcp::Json<Value>, String> {
        let target = self.collection_folder(&input.target_path)?;
        let old = self.mock.lock().await.take();
        if let Some(old) = old {
            old.stop().await;
        }
        let mock = LocalMock::start(json!({
            "workspacePath": self.workspace,
            "path": target,
            "host": "127.0.0.1",
            "port": input.port.unwrap_or(8080),
        }))
        .await?;
        let status = redact_sensitive(mock.status().clone());
        *self.mock.lock().await = Some(mock);
        Ok(rmcp::Json(status))
    }

    /// Change the response returned by one route in the running mock server.
    #[tool(
        name = "add_mock_override",
        description = "Set a response override for a route ID returned by start_mock_server."
    )]
    async fn add_mock_override(
        &self,
        Parameters(input): Parameters<MockOverrideInput>,
    ) -> Result<rmcp::Json<Value>, String> {
        if !input.response.is_object() {
            return Err("response override must be a JSON object".into());
        }
        let guard = self.mock.lock().await;
        let mock = guard
            .as_ref()
            .ok_or_else(|| "start_mock_server must be called first".to_owned())?;
        mock.set_override(&input.route_id, input.response)
            .map_err(|error| error.to_string())?;
        Ok(rmcp::Json(
            json!({ "success": true, "routeId": input.route_id }),
        ))
    }

    /// Return mock server logs after an optional cursor.
    #[tool(
        name = "get_mock_logs",
        description = "Get recent request logs from the running PebblePost mock server."
    )]
    async fn get_mock_logs(
        &self,
        Parameters(input): Parameters<MockLogsInput>,
    ) -> Result<rmcp::Json<Value>, String> {
        let guard = self.mock.lock().await;
        let mock = guard
            .as_ref()
            .ok_or_else(|| "start_mock_server must be called first".to_owned())?;
        let logs = mock.logs_after(input.cursor.unwrap_or(0));
        Ok(rmcp::Json(redact_sensitive(json!({ "logs": logs }))))
    }

    /// Stop the mock server started by this MCP process.
    #[tool(
        name = "stop_mock_server",
        description = "Stop the mock server started by this PebblePost MCP process."
    )]
    async fn stop_mock_server(&self) -> Result<rmcp::Json<Value>, String> {
        let mock = self.mock.lock().await.take();
        if let Some(mock) = mock {
            mock.stop().await;
            Ok(rmcp::Json(json!({ "stopped": true })))
        } else {
            Ok(rmcp::Json(
                json!({ "stopped": false, "message": "No mock server is running" }),
            ))
        }
    }
}

#[tool_handler(router = self.tool_router)]
impl ServerHandler for PebblePostMcp {
    fn get_info(&self) -> ServerConfig {
        ServerConfig::new(ServerCapabilities::builder().enable_tools().build())
            .with_server_info(Implementation::new("pebblepost-mcp", env!("CARGO_PKG_VERSION")))
            .with_instructions(format!(
                "This server can access only the PebblePost workspace at {}. Environment values and request secrets are redacted from tool outputs. Request scripts run only when allowed by the workspace trust setting.",
                self.workspace.display()
            ))
    }
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = Args::parse();
    let workspace = validate_workspace_root(&args.workspace)?;
    let handler = PebblePostMcp::new(workspace);
    let server = handler.clone().serve(stdio()).await?;
    server.waiting().await?;
    if let Some(mock) = handler.mock.lock().await.take() {
        mock.stop().await;
    }
    Ok(())
}

fn clean_collection_path(path: &str, allow_folder: bool) -> Result<String, String> {
    if path.is_empty()
        || path.starts_with('/')
        || path.contains('\\')
        || path.contains('\0')
        || path.contains(':')
    {
        return Err("paths must be relative paths under collections/".into());
    }
    let parts = Path::new(path).components().collect::<Vec<_>>();
    if parts
        .iter()
        .any(|part| !matches!(part, Component::Normal(_)))
        || parts.first().and_then(|part| match part {
            Component::Normal(value) => value.to_str(),
            _ => None,
        }) != Some("collections")
    {
        return Err("paths must be normal workspace-relative paths under collections/".into());
    }
    if !allow_folder && parts.len() < 2 {
        return Err("request path must name a file inside a collection".into());
    }
    Ok(path.replace('\\', "/"))
}

fn count_requests(nodes: &[pebblepost_core::TreeNode]) -> usize {
    nodes
        .iter()
        .map(|node| {
            if node.is_dir {
                count_requests(&node.children)
            } else {
                1
            }
        })
        .sum()
}

fn collect_requests(
    nodes: &[pebblepost_core::TreeNode],
    prefix: Option<&str>,
    output: &mut Vec<Value>,
) {
    for node in nodes {
        if node.is_dir {
            collect_requests(&node.children, prefix, output);
        } else if prefix.is_none_or(|prefix| {
            node.rel_path == prefix || node.rel_path.starts_with(&format!("{prefix}/"))
        }) {
            output.push(json!({
                "name": node.display_name,
                "path": node.rel_path,
                "method": node.method,
            }));
        }
    }
}

fn redact_sensitive(value: Value) -> Value {
    match value {
        Value::Object(object) => {
            let secret_pair = object
                .get("key")
                .and_then(Value::as_str)
                .is_some_and(is_sensitive_key)
                || object.get("secret").and_then(Value::as_bool) == Some(true);
            let json_body = object
                .get("type")
                .and_then(Value::as_str)
                .is_some_and(|kind| kind.eq_ignore_ascii_case("json"));
            let mut redacted = Map::new();
            for (key, value) in object {
                let safe_value = if is_sensitive_key(&key) || (secret_pair && key == "value") {
                    Value::String("[REDACTED]".into())
                } else if key.to_ascii_lowercase().ends_with("url")
                    || key.eq_ignore_ascii_case("specLocation")
                {
                    value
                        .as_str()
                        .map(redact_url)
                        .map(Value::String)
                        .unwrap_or_else(|| redact_sensitive(value))
                } else if key == "query" {
                    value
                        .as_str()
                        .map(redact_query)
                        .map(Value::String)
                        .unwrap_or_else(|| redact_sensitive(value))
                } else if key == "body" {
                    value
                        .as_str()
                        .and_then(redact_json_text)
                        .map(Value::String)
                        .unwrap_or_else(|| redact_sensitive(value))
                } else if key == "raw" && json_body {
                    value
                        .as_str()
                        .and_then(redact_json_text)
                        .map(Value::String)
                        .unwrap_or_else(|| redact_sensitive(value))
                } else {
                    redact_sensitive(value)
                };
                redacted.insert(key, safe_value);
            }
            Value::Object(redacted)
        }
        Value::Array(items) => Value::Array(items.into_iter().map(redact_sensitive).collect()),
        value => value,
    }
}

fn is_sensitive_key(key: &str) -> bool {
    let key = key.to_ascii_lowercase().replace(['-', ' '], "_");
    [
        "secret",
        "password",
        "token",
        "authorization",
        "cookie",
        "api_key",
        "apikey",
        "access_key",
        "accesskey",
        "private_key",
        "privatekey",
        "credential",
    ]
    .iter()
    .any(|marker| key.contains(marker))
}

fn redact_url(url: &str) -> String {
    let Some(scheme_end) = url.find("://") else {
        return redact_query(url);
    };
    let authority_start = scheme_end + 3;
    let authority_end = url[authority_start..]
        .find(['/', '?', '#'])
        .map(|offset| authority_start + offset)
        .unwrap_or(url.len());
    let authority = &url[authority_start..authority_end];
    let safe_authority = authority
        .rfind('@')
        .map(|at| format!("[REDACTED]@{}", &authority[at + 1..]))
        .unwrap_or_else(|| authority.to_owned());
    let suffix = &url[authority_end..];
    format!(
        "{}://{}{}",
        &url[..scheme_end],
        safe_authority,
        redact_query(suffix)
    )
}

fn redact_query(query: &str) -> String {
    let (prefix, parameters) = query
        .find('?')
        .map(|query_start| (&query[..=query_start], &query[query_start + 1..]))
        .unwrap_or(("", query));
    let (parameters, fragment) = parameters
        .split_once('#')
        .map(|(parameters, fragment)| (parameters, format!("#{fragment}")))
        .unwrap_or((parameters, String::new()));
    let safe_query = parameters
        .split('&')
        .map(|parameter| {
            let Some((key, value)) = parameter.split_once('=') else {
                return parameter.to_owned();
            };
            if is_sensitive_key(key) && !(value.starts_with("{{") && value.ends_with("}}")) {
                format!("{key}=[REDACTED]")
            } else {
                parameter.to_owned()
            }
        })
        .collect::<Vec<_>>()
        .join("&");
    format!("{prefix}{safe_query}{fragment}")
}

fn redact_json_text(text: &str) -> Option<String> {
    let trimmed = text.trim_start();
    if !trimmed.starts_with('{') && !trimmed.starts_with('[') {
        return None;
    }
    let value = serde_json::from_str::<Value>(text).ok()?;
    serde_json::to_string(&redact_sensitive(value)).ok()
}

fn validate_json_path(path: &str) -> Result<Vec<String>, String> {
    let segments = path.split('.').map(str::to_owned).collect::<Vec<_>>();
    if segments.is_empty()
        || segments.iter().any(|segment| {
            segment.is_empty()
                || !segment.chars().enumerate().all(|(index, character)| {
                    character == '_'
                        || character == '$'
                        || character.is_ascii_alphanumeric()
                            && (index > 0 || !character.is_ascii_digit())
                })
        })
    {
        return Err(format!(
            "JSON field path {path:?} must use dot-separated JavaScript identifiers"
        ));
    }
    Ok(segments)
}

fn value_at_path<'a>(value: &'a Value, segments: &[String]) -> Option<&'a Value> {
    segments
        .iter()
        .try_fold(value, |current, segment| current.get(segment))
}
