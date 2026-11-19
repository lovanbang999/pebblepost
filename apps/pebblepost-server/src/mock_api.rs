use std::collections::HashMap;
use std::fs;
use std::path::Path;
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Instant;

use axum::body::{Body, Bytes, to_bytes};
use axum::extract::State;
use axum::http::{HeaderMap, HeaderName, HeaderValue, Request, StatusCode, header};
use axum::response::sse::{Event, KeepAlive};
use axum::response::{IntoResponse, Response, Sse};
use axum::routing::get;
use axum::{Json, Router};
use futures_util::stream;
use pebblepost_core::{read_request, safe_workspace_path};
use serde_json::{Value, json};
use tokio::sync::{broadcast, oneshot};
use tokio::task::JoinHandle;

const MAX_LOGS: usize = 500;
const MAX_MATCH_BODY_BYTES: usize = 2 * 1024 * 1024;

#[derive(Clone)]
struct MockRoute {
    id: String,
    name: String,
    file_path: String,
    method: String,
    path: String,
    matcher: Value,
    response_template: Value,
    examples: Vec<Value>,
}

#[derive(Clone)]
struct MockService {
    routes: Vec<MockRoute>,
    global_delay_ms: u64,
    global_status: u16,
    global_error_rate: f64,
    overrides: Arc<Mutex<HashMap<String, Value>>>,
    logs: Arc<Mutex<Vec<Value>>>,
    log_events: broadcast::Sender<Value>,
    stop_events: broadcast::Sender<()>,
    sequence: Arc<AtomicU64>,
}

#[derive(Default)]
struct MockRuntime {
    host: String,
    port: u16,
    target: String,
    running: bool,
    service: Option<MockService>,
    shutdown: Option<oneshot::Sender<()>>,
    task: Option<JoinHandle<()>>,
}

#[derive(Default)]
pub struct MockManager {
    runtime: Mutex<MockRuntime>,
    lifecycle: tokio::sync::Mutex<()>,
}

struct MockStartError {
    status: StatusCode,
    message: String,
}

impl From<String> for MockStartError {
    fn from(message: String) -> Self {
        Self {
            status: StatusCode::BAD_REQUEST,
            message,
        }
    }
}

impl From<&str> for MockStartError {
    fn from(message: &str) -> Self {
        Self::from(message.to_owned())
    }
}

impl std::fmt::Display for MockStartError {
    fn fmt(&self, formatter: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        formatter.write_str(&self.message)
    }
}

pub struct LocalMock {
    manager: Arc<MockManager>,
    status: Value,
}

impl LocalMock {
    pub async fn start(payload: Value) -> Result<Self, String> {
        let manager = Arc::new(MockManager::default());
        let status = start_inner(&manager, payload)
            .await
            .map_err(|error| error.to_string())?;
        Ok(Self { manager, status })
    }

    pub fn status(&self) -> &Value {
        &self.status
    }

    pub fn logs_after(&self, cursor: u64) -> Vec<Value> {
        let runtime = self
            .manager
            .runtime
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        runtime
            .service
            .as_ref()
            .map(|service| {
                service
                    .logs
                    .lock()
                    .unwrap_or_else(std::sync::PoisonError::into_inner)
                    .iter()
                    .filter(|log| {
                        log.get("id")
                            .and_then(Value::as_str)
                            .and_then(|id| id.parse::<u64>().ok())
                            .is_some_and(|id| id > cursor)
                    })
                    .cloned()
                    .collect()
            })
            .unwrap_or_default()
    }

    pub fn set_override(&self, route_id: &str, value: Value) -> Result<(), String> {
        let runtime = self
            .manager
            .runtime
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let Some(service) = runtime.service.as_ref().filter(|_| runtime.running) else {
            return Err("Mock server is not running".into());
        };
        if !service.routes.iter().any(|route| route.id == route_id) {
            return Err(format!("Mock route {route_id:?} was not found"));
        }
        service
            .overrides
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .insert(route_id.to_owned(), value);
        Ok(())
    }

    pub async fn stop(&self) {
        let _lifecycle = self.manager.lifecycle.lock().await;
        stop_runtime(&self.manager).await;
    }
}

pub async fn start(State(app): State<Arc<super::AppState>>, body: Bytes) -> Response {
    let payload = if body.is_empty() {
        json!({})
    } else {
        match serde_json::from_slice::<Value>(&body) {
            Ok(payload) => payload,
            Err(error) => {
                return error_response(
                    StatusCode::BAD_REQUEST,
                    format!("Invalid JSON payload: {error}"),
                );
            }
        }
    };
    match start_inner(&app.mock_manager, payload).await {
        Ok(status) => Json(status).into_response(),
        Err(error) => error_response(error.status, error.message),
    }
}

async fn start_inner(manager: &Arc<MockManager>, payload: Value) -> Result<Value, MockStartError> {
    let _lifecycle = manager.lifecycle.lock().await;
    let workspace = payload
        .get("workspacePath")
        .and_then(Value::as_str)
        .unwrap_or(".");
    let target_text = payload
        .get("path")
        .and_then(Value::as_str)
        .filter(|value| !value.is_empty())
        .unwrap_or(workspace);
    let root =
        fs::canonicalize(workspace).map_err(|error| format!("Cannot access workspace: {error}"))?;
    let requested = Path::new(target_text);
    let requested = if requested.is_absolute() {
        requested.to_path_buf()
    } else {
        root.join(requested)
    };
    let target = safe_workspace_path(&root, &requested).map_err(|error| error.to_string())?;
    if !target.is_dir() && !target.to_string_lossy().ends_with(".pebble.json") {
        return Err("Mock target must be a collection folder or request file".into());
    }
    // The Go handler stops an existing instance before loading the replacement routes.
    stop_runtime(manager).await;
    let mut routes = Vec::new();
    collect_routes(&root, &target, &mut routes).map_err(|error| error.to_string())?;
    if routes.is_empty() {
        return Err("No requests with saved response examples were found".into());
    }
    routes.sort_by(|left, right| left.file_path.cmp(&right.file_path));
    routes.sort_by(|left, right| {
        let l = route_specificity(&left.path);
        let r = route_specificity(&right.path);
        r.cmp(&l)
            .then_with(|| route_segment_count(&right.path).cmp(&route_segment_count(&left.path)))
    });
    let host = payload
        .get("host")
        .and_then(Value::as_str)
        .filter(|host| !host.trim().is_empty())
        .unwrap_or("127.0.0.1")
        .to_owned();
    let requested_port = payload.get("port").and_then(Value::as_i64).unwrap_or(8080);
    let port = if requested_port <= 0 {
        8080
    } else {
        u16::try_from(requested_port).map_err(|_| MockStartError {
            status: StatusCode::INTERNAL_SERVER_ERROR,
            message: format!("Failed to start mock server: invalid port {requested_port}"),
        })?
    };
    let listener = tokio::net::TcpListener::bind((host.as_str(), port))
        .await
        .map_err(|error| MockStartError {
            status: StatusCode::INTERNAL_SERVER_ERROR,
            message: format!(
                "Failed to start mock server: failed to bind mock server on {host}:{port}: {error}"
            ),
        })?;
    let address = listener.local_addr().map_err(|error| error.to_string())?;
    let overrides: HashMap<String, Value> = payload
        .get("overrides")
        .and_then(Value::as_object)
        .map(|items| {
            items
                .iter()
                .map(|(key, value)| (key.clone(), value.clone()))
                .collect()
        })
        .unwrap_or_default();
    let route_summaries = routes
        .iter()
        .map(|route| route_summary(route, overrides.get(&route.id)))
        .collect::<Vec<_>>();
    let (log_events, _) = broadcast::channel(50);
    let (stop_events, _) = broadcast::channel(1);
    let service = MockService {
        routes,
        global_delay_ms: payload
            .get("globalDelayMs")
            .and_then(Value::as_u64)
            .unwrap_or(0),
        global_status: payload
            .get("globalStatusCode")
            .and_then(Value::as_u64)
            .unwrap_or(0)
            .min(599) as u16,
        global_error_rate: payload
            .get("globalErrorRate")
            .and_then(Value::as_f64)
            .unwrap_or(0.0)
            .clamp(0.0, 1.0),
        overrides: Arc::new(Mutex::new(overrides)),
        logs: Arc::new(Mutex::new(Vec::new())),
        log_events,
        stop_events,
        sequence: Arc::new(AtomicU64::new(0)),
    };
    let (shutdown_tx, shutdown_rx) = oneshot::channel();
    let app_router = Router::new()
        .route(
            "/",
            get(mock_request)
                .post(mock_request)
                .put(mock_request)
                .patch(mock_request)
                .delete(mock_request)
                .options(mock_request)
                .head(mock_request),
        )
        .fallback(mock_request)
        .with_state(service.clone());
    let task = tokio::spawn(async move {
        let _ = axum::serve(listener, app_router)
            .with_graceful_shutdown(async {
                let _ = shutdown_rx.await;
            })
            .await;
    });
    let mut runtime = manager
        .runtime
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner);
    runtime.host = host.clone();
    runtime.port = address.port();
    runtime.target = target.to_string_lossy().into_owned();
    runtime.running = true;
    runtime.service = Some(service);
    runtime.shutdown = Some(shutdown_tx);
    runtime.task = Some(task);
    Ok(status_value(&runtime, route_summaries))
}

pub async fn stop(State(app): State<Arc<super::AppState>>) -> Response {
    let _lifecycle = app.mock_manager.lifecycle.lock().await;
    stop_runtime(&app.mock_manager).await;
    let runtime = app
        .mock_manager
        .runtime
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner);
    let routes = runtime
        .service
        .as_ref()
        .map(service_route_summaries)
        .unwrap_or_default();
    Json(status_value(&runtime, routes)).into_response()
}

async fn stop_runtime(manager: &MockManager) {
    let (shutdown, task, stop_events) = {
        let mut runtime = manager
            .runtime
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        runtime.running = false;
        let stop_events = runtime
            .service
            .as_ref()
            .map(|service| service.stop_events.clone());
        (runtime.shutdown.take(), runtime.task.take(), stop_events)
    };
    if let Some(stop_events) = stop_events {
        let _ = stop_events.send(());
    }
    if let Some(shutdown) = shutdown {
        let _ = shutdown.send(());
    }
    if let Some(mut task) = task
        && tokio::time::timeout(std::time::Duration::from_secs(2), &mut task)
            .await
            .is_err()
    {
        task.abort();
        let _ = task.await;
    }
}

pub async fn status(State(app): State<Arc<super::AppState>>) -> Json<Value> {
    let runtime = app
        .mock_manager
        .runtime
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner);
    let routes = runtime
        .service
        .as_ref()
        .map(service_route_summaries)
        .unwrap_or_default();
    Json(status_value(&runtime, routes))
}

pub async fn logs(State(app): State<Arc<super::AppState>>) -> Json<Value> {
    let runtime = app
        .mock_manager
        .runtime
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner);
    let logs = runtime
        .service
        .as_ref()
        .map(|service| {
            service
                .logs
                .lock()
                .unwrap_or_else(std::sync::PoisonError::into_inner)
                .clone()
        })
        .unwrap_or_default();
    Json(json!(logs))
}

pub async fn logs_stream(
    State(app): State<Arc<super::AppState>>,
) -> Sse<impl futures_util::Stream<Item = Result<Event, std::convert::Infallible>>> {
    struct StreamState {
        initial_status: Option<Value>,
        logs: Option<broadcast::Receiver<Value>>,
        stopped: Option<broadcast::Receiver<()>>,
    }
    let (initial_status, logs, stopped) = {
        let runtime = app
            .mock_manager
            .runtime
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        match runtime.service.as_ref() {
            Some(service) => {
                let url = status_value(&runtime, service_route_summaries(service))
                    .get("url")
                    .cloned()
                    .unwrap_or_else(|| json!(""));
                (
                    Some(json!({ "type": "status", "running": runtime.running, "url": url })),
                    Some(service.log_events.subscribe()),
                    Some(service.stop_events.subscribe()),
                )
            }
            None => (
                Some(json!({ "type": "status", "running": false })),
                None,
                None,
            ),
        }
    };
    let events = stream::unfold(
        StreamState {
            initial_status,
            logs,
            stopped,
        },
        |mut state| async move {
            if let Some(status) = state.initial_status.take() {
                if !status
                    .get("running")
                    .and_then(Value::as_bool)
                    .unwrap_or(false)
                {
                    state.logs = None;
                    state.stopped = None;
                }
                return Some((Ok(Event::default().data(status.to_string())), state));
            }
            loop {
                enum StreamEvent {
                    Log(Result<Value, broadcast::error::RecvError>),
                    Stopped,
                }
                let event = match (state.logs.as_mut(), state.stopped.as_mut()) {
                    (Some(logs), Some(stopped)) => tokio::select! {
                        result = logs.recv() => StreamEvent::Log(result),
                        _ = stopped.recv() => StreamEvent::Stopped,
                    },
                    _ => return None,
                };
                match event {
                    StreamEvent::Log(Ok(log)) => {
                        return Some((Ok(Event::default().data(log.to_string())), state));
                    }
                    StreamEvent::Log(Err(broadcast::error::RecvError::Lagged(_))) => continue,
                    StreamEvent::Log(Err(broadcast::error::RecvError::Closed))
                    | StreamEvent::Stopped => return None,
                }
            }
        },
    );
    Sse::new(events).keep_alive(KeepAlive::default())
}

pub async fn update_override(State(app): State<Arc<super::AppState>>, body: Bytes) -> Response {
    let payload = match serde_json::from_slice::<Value>(&body) {
        Ok(payload) => payload,
        Err(error) => {
            return error_response(StatusCode::BAD_REQUEST, format!("Invalid payload: {error}"));
        }
    };
    let route_id = payload.get("routeId").and_then(Value::as_str).unwrap_or("");
    let runtime = app
        .mock_manager
        .runtime
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner);
    let Some(service) = runtime.service.as_ref().filter(|_| runtime.running) else {
        return error_response(StatusCode::BAD_REQUEST, "Mock server is not running");
    };
    let mut overrides = service
        .overrides
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner);
    match payload.get("override") {
        Some(value) if value.is_object() => {
            overrides.insert(route_id.to_owned(), value.clone());
        }
        _ => {
            overrides.remove(route_id);
        }
    }
    Json(json!({ "success": true, "routeId": route_id })).into_response()
}

async fn mock_request(State(service): State<MockService>, request: Request<Body>) -> Response {
    let started_at = chrono::Utc::now();
    let started = Instant::now();
    let (parts, body_stream) = request.into_parts();
    let body_bytes = match to_bytes(body_stream, MAX_MATCH_BODY_BYTES).await {
        Ok(body) => body,
        Err(error) => {
            let request = Request::from_parts(parts, Body::empty());
            let method = request.method().as_str().to_owned();
            let path = request.uri().path().to_owned();
            let query = request.uri().query().unwrap_or("").to_owned();
            let message = format!("Mock request body exceeds the matching limit: {error}");
            record_log(
                &service,
                &request,
                &method,
                &path,
                &query,
                413,
                false,
                "",
                "",
                started_at,
                started.elapsed().as_millis() as u64,
                Some(message.clone()),
                true,
            );
            return error_response(StatusCode::PAYLOAD_TOO_LARGE, message);
        }
    };
    let request = Request::from_parts(parts, Body::from(body_bytes.clone()));
    let method = request.method().as_str().to_owned();
    let path = request.uri().path().to_owned();
    let query = request.uri().query().unwrap_or("").to_owned();
    if method.eq_ignore_ascii_case("OPTIONS") {
        let mut response = StatusCode::NO_CONTENT.into_response();
        set_cors_headers(&mut response);
        return response;
    }
    let matched = service.routes.iter().find_map(|route| {
        route_matches_request(
            route,
            &method,
            &path,
            &query,
            request.headers(),
            &body_bytes,
        )
        .map(|params| (route, params))
    });
    let Some((route, path_params)) = matched else {
        let routes = service.routes.iter().map(|route| json!({
            "method": route.method,
            "path": route.path,
            "examples": route.examples.iter().filter_map(|example| example.get("name").and_then(Value::as_str)).collect::<Vec<_>>()
        })).collect::<Vec<_>>();
        let body = json!({ "error": "Not Found", "method": method, "path": path, "message": format!("No saved mock example found for {method} {path}"), "availableRoutes": routes }).to_string();
        record_log(
            &service,
            &request,
            &method,
            &path,
            &query,
            404,
            false,
            "",
            "",
            started_at,
            started.elapsed().as_millis() as u64,
            Some("No route matched request method and path".into()),
            true,
        );
        let mut response = (
            StatusCode::NOT_FOUND,
            [(header::CONTENT_TYPE, "application/json")],
            body,
        )
            .into_response();
        set_cors_headers(&mut response);
        return response;
    };
    let route_id = route.id.clone();
    let example = pick_example(&route.examples, request.headers())
        .cloned()
        .unwrap_or_else(|| json!({}));
    let override_value = {
        let overrides = service
            .overrides
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        overrides
            .get(&route_id)
            .or_else(|| overrides.get(&format!("{}:{}", route.method, route.path)))
            .cloned()
            .unwrap_or(Value::Null)
    };
    let delay_ms = override_value
        .get("delayMs")
        .and_then(Value::as_u64)
        .filter(|delay| *delay > 0)
        .unwrap_or(service.global_delay_ms);
    if delay_ms > 0 {
        tokio::time::sleep(std::time::Duration::from_millis(delay_ms)).await;
    }
    let error_rate = override_value
        .get("errorRate")
        .and_then(Value::as_f64)
        .filter(|rate| *rate > 0.0)
        .unwrap_or(service.global_error_rate)
        .clamp(0.0, 1.0);
    let injected_error = random_unit() < error_rate;
    let configured_status = override_value
        .get("statusCode")
        .and_then(Value::as_u64)
        .filter(|code| *code > 0)
        .map(|code| code.min(599) as u16)
        .or_else(|| (service.global_status > 0).then_some(service.global_status));
    let status = if injected_error {
        configured_status.unwrap_or(500)
    } else {
        configured_status.unwrap_or_else(|| {
            example
                .get("statusCode")
                .and_then(Value::as_u64)
                .unwrap_or(200)
                .clamp(100, 599) as u16
        })
    };
    let template = override_value
        .get("responseTemplate")
        .filter(|value| value.is_object())
        .or_else(|| {
            route
                .response_template
                .as_object()
                .map(|_| &route.response_template)
        });
    let body_source = template
        .and_then(|value| value.get("body"))
        .or_else(|| override_value.get("body"))
        .or_else(|| example.get("body"));
    let body_source = body_source
        .and_then(|value| {
            value
                .as_str()
                .map(str::to_owned)
                .or_else(|| (!value.is_null()).then(|| value.to_string()))
        })
        .unwrap_or_default();
    let body = if injected_error {
        json!({ "error": "Simulated mock failure via error-injection", "status": status, "errorRate": error_rate }).to_string()
    } else {
        render_template(&body_source, &request, &path_params, &body_bytes)
    };
    let example_name = if injected_error {
        "error-injection".to_owned()
    } else {
        example
            .get("name")
            .and_then(Value::as_str)
            .unwrap_or("")
            .to_owned()
    };
    let error_message = injected_error
        .then(|| format!("Simulated error injection triggered (rate: {error_rate:.2})"));
    record_log(
        &service,
        &request,
        &method,
        &path,
        &query,
        status,
        true,
        &route_id,
        &example_name,
        started_at,
        started.elapsed().as_millis() as u64,
        error_message,
        !injected_error,
    );
    let mut response = Response::new(Body::from(body));
    *response.status_mut() = StatusCode::from_u16(status).unwrap_or(StatusCode::OK);
    let example_content_type = if injected_error {
        "application/json"
    } else {
        template
            .and_then(|value| value.get("contentType"))
            .and_then(Value::as_str)
            .or_else(|| example.get("contentType").and_then(Value::as_str))
            .unwrap_or("application/json")
    };
    if let Ok(value) = HeaderValue::from_str(example_content_type) {
        response.headers_mut().insert(header::CONTENT_TYPE, value);
    }
    if !injected_error {
        if let Some(headers) = template
            .and_then(|value| value.get("headers"))
            .and_then(Value::as_object)
        {
            for (key, value) in headers {
                if key.eq_ignore_ascii_case("content-length") {
                    continue;
                }
                let rendered = render_template(
                    &value
                        .as_str()
                        .map(str::to_owned)
                        .unwrap_or_else(|| value.to_string()),
                    &request,
                    &path_params,
                    &body_bytes,
                );
                if let (Ok(key), Ok(value)) = (
                    HeaderName::from_bytes(key.as_bytes()),
                    HeaderValue::from_str(&rendered),
                ) {
                    response.headers_mut().append(key, value);
                }
            }
        } else if let Some(headers) = override_value.get("headers").and_then(Value::as_object) {
            for (key, value) in headers {
                if key.eq_ignore_ascii_case("content-length") {
                    continue;
                }
                let rendered = render_template(
                    &value
                        .as_str()
                        .map(str::to_owned)
                        .unwrap_or_else(|| value.to_string()),
                    &request,
                    &path_params,
                    &body_bytes,
                );
                if let (Ok(key), Ok(value)) = (
                    HeaderName::from_bytes(key.as_bytes()),
                    HeaderValue::from_str(&rendered),
                ) {
                    response.headers_mut().append(key, value);
                }
            }
        } else if let Some(headers) = example.get("headers").and_then(Value::as_array) {
            for item in headers {
                if let (Some(key), Some(value)) = (
                    item.get("key").and_then(Value::as_str),
                    item.get("value").and_then(Value::as_str),
                ) {
                    if key.eq_ignore_ascii_case("content-length") {
                        continue;
                    }
                    let rendered = render_template(value, &request, &path_params, &body_bytes);
                    if let (Ok(key), Ok(value)) = (
                        HeaderName::from_bytes(key.as_bytes()),
                        HeaderValue::from_str(&rendered),
                    ) {
                        response.headers_mut().append(key, value);
                    }
                }
            }
        }
    }
    set_cors_headers(&mut response);
    response
}

fn pick_example<'a>(examples: &'a [Value], headers: &HeaderMap) -> Option<&'a Value> {
    if examples.is_empty() {
        return None;
    }
    let requested = headers
        .get("x-mock-example")
        .and_then(|value| value.to_str().ok())
        .map(str::trim)
        .filter(|value| !value.is_empty())
        .map(str::to_owned)
        .or_else(|| prefer_example(headers));
    requested
        .and_then(|name| {
            examples.iter().find(|example| {
                example
                    .get("name")
                    .and_then(Value::as_str)
                    .is_some_and(|example_name| example_name.eq_ignore_ascii_case(&name))
            })
        })
        .or_else(|| examples.first())
}

fn prefer_example(headers: &HeaderMap) -> Option<String> {
    let value = headers.get("prefer")?.to_str().ok()?;
    value.split([',', ';']).find_map(|part| {
        let value = part.trim().strip_prefix("example=")?.trim();
        let value = value
            .strip_prefix('"')
            .and_then(|value| value.strip_suffix('"'))
            .unwrap_or(value);
        (!value.is_empty()).then(|| value.to_owned())
    })
}

fn set_cors_headers(response: &mut Response) {
    let headers = response.headers_mut();
    headers.insert(
        HeaderName::from_static("access-control-allow-origin"),
        HeaderValue::from_static("*"),
    );
    headers.insert(
        HeaderName::from_static("access-control-allow-methods"),
        HeaderValue::from_static("GET, POST, PUT, DELETE, PATCH, OPTIONS, HEAD"),
    );
    headers.insert(
        HeaderName::from_static("access-control-allow-headers"),
        HeaderValue::from_static("*"),
    );
    headers.insert(
        HeaderName::from_static("access-control-expose-headers"),
        HeaderValue::from_static("*"),
    );
}

fn collect_routes(
    root: &Path,
    target: &Path,
    out: &mut Vec<MockRoute>,
) -> Result<(), std::io::Error> {
    if target.is_file() {
        if let Ok(request) = read_request(root, target) {
            add_route(target, &request, out);
        }
        return Ok(());
    }
    for entry in fs::read_dir(target)? {
        let entry = entry?;
        let path = entry.path();
        let name = entry.file_name().to_string_lossy().into_owned();
        if entry.file_type()?.is_dir() {
            if !name.starts_with('.') && name != ".pebble" {
                collect_routes(root, &path, out)?;
            }
        } else if name.ends_with(".pebble.json")
            && name != "_folder.pebble.json"
            && let Ok(request) = read_request(root, &path)
        {
            add_route(&path, &request, out);
        }
    }
    Ok(())
}

fn add_route(path: &Path, request: &Value, out: &mut Vec<MockRoute>) {
    let examples = request
        .get("examples")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    if examples.is_empty() {
        return;
    }
    let method = request
        .get("method")
        .and_then(Value::as_str)
        .unwrap_or("")
        .trim();
    let method = if method.is_empty() {
        "GET".to_owned()
    } else {
        method.to_ascii_uppercase()
    };
    let route_path = url_path(request.get("url").and_then(Value::as_str).unwrap_or("/"));
    let matcher = request
        .get("mockMatcher")
        .or_else(|| request.get("matcher"))
        .or_else(|| request.pointer("/mock/match"))
        .cloned()
        .unwrap_or_else(|| json!({}));
    let response_template = request
        .pointer("/mock/responseTemplate")
        .or_else(|| request.get("mockResponseTemplate"))
        .cloned()
        .unwrap_or_else(|| json!({}));
    out.push(MockRoute {
        id: format!("{method}:{route_path}"),
        name: request
            .get("name")
            .and_then(Value::as_str)
            .unwrap_or("")
            .into(),
        file_path: path.to_string_lossy().into_owned(),
        method,
        path: route_path,
        matcher,
        response_template,
        examples,
    });
}

fn url_path(url: &str) -> String {
    let mut raw = url.trim().split(['?', '#']).next().unwrap_or("").trim();
    if raw.starts_with("{{") {
        if let Some(end) = raw.find("}}") {
            raw = raw[end + 2..].trim();
        }
        if raw.is_empty() {
            return "/".into();
        }
        return clean_route_path(if raw.starts_with('/') {
            raw.to_owned()
        } else {
            format!("/{raw}")
        });
    }
    if let Some((_, authority_and_path)) = raw.split_once("://") {
        let path = authority_and_path
            .find('/')
            .map(|index| &authority_and_path[index..])
            .unwrap_or("/");
        return clean_route_path(path.to_owned());
    }
    if raw.starts_with('/') {
        return clean_route_path(raw.to_owned());
    }
    if let Some(slash) = raw.find('/') {
        let prefix = &raw[..slash];
        if prefix.contains('.') || prefix.contains(':') {
            return clean_route_path(raw[slash..].to_owned());
        }
    }
    clean_route_path(format!("/{raw}"))
}

fn clean_route_path(path: String) -> String {
    let segments = path
        .split('/')
        .filter(|segment| !segment.is_empty())
        .collect::<Vec<_>>();
    if segments.is_empty() {
        "/".into()
    } else {
        format!("/{}", segments.join("/"))
    }
}

fn match_route_params(pattern: &str, actual: &str) -> Option<HashMap<String, String>> {
    let expected = pattern.trim_matches('/').split('/').collect::<Vec<_>>();
    let received = actual.trim_matches('/').split('/').collect::<Vec<_>>();
    let mut params = HashMap::new();
    if pattern == "/" && (actual == "/" || actual.is_empty()) {
        return Some(params);
    }
    let mut index = 0;
    while index < expected.len() {
        let segment = expected[index].trim();
        if segment == "*" {
            return Some(params);
        }
        let is_param = is_route_parameter(segment);
        if index >= received.len()
            || (!is_param && !segment.eq_ignore_ascii_case(received[index].trim()))
        {
            return None;
        }
        if is_param {
            let name = segment
                .trim_start_matches(':')
                .trim_start_matches('{')
                .trim_end_matches('}');
            params.insert(name.to_owned(), received[index].to_owned());
        }
        index += 1;
    }
    (index == received.len()).then_some(params)
}

fn route_matches_request(
    route: &MockRoute,
    method: &str,
    path: &str,
    query: &str,
    headers: &HeaderMap,
    body: &[u8],
) -> Option<HashMap<String, String>> {
    if !route.method.eq_ignore_ascii_case(method) {
        return None;
    }
    let params: HashMap<String, String> =
        if let Some(pattern) = route.matcher.get("pathRegex").and_then(Value::as_str) {
            let regex = regex::Regex::new(pattern).ok()?;
            let captures = regex.captures(path)?;
            regex
                .capture_names()
                .flatten()
                .filter_map(|name| {
                    captures
                        .name(name)
                        .map(|value| (name.to_owned(), value.as_str().to_owned()))
                })
                .collect()
        } else {
            match_route_params(&route.path, path)?
        };
    if let Some(required) = route.matcher.get("method").and_then(Value::as_str)
        && !required.eq_ignore_ascii_case(method)
    {
        return None;
    }
    if let Some(expected) = route.matcher.get("headers").and_then(Value::as_object) {
        for (key, value) in expected {
            let actual = headers.get(key).and_then(|value| value.to_str().ok());
            if !constraint_matches(value, actual) {
                return None;
            }
        }
    }
    if let Some(expected) = route.matcher.get("query").and_then(Value::as_object) {
        let parsed = reqwest::Url::parse(&format!("http://mock.local/?{query}")).ok()?;
        let actual = parsed
            .query_pairs()
            .map(|(key, value)| (key.into_owned(), value.into_owned()))
            .collect::<Vec<_>>();
        for (key, constraint) in expected {
            let values = actual
                .iter()
                .filter(|(actual_key, _)| actual_key == key)
                .map(|(_, value)| value.as_str())
                .collect::<Vec<_>>();
            if !constraint_matches_any(constraint, &values) {
                return None;
            }
        }
    }
    if let Some(schema) = route
        .matcher
        .get("bodySchema")
        .or_else(|| route.matcher.get("jsonSchema"))
    {
        let json_body = serde_json::from_slice::<Value>(body).ok()?;
        if !matches_json_schema(&json_body, schema) {
            return None;
        }
    }
    Some(params)
}

fn constraint_matches(expected: &Value, actual: Option<&str>) -> bool {
    if let Some(exists) = expected.get("exists").and_then(Value::as_bool) {
        return actual.is_some() == exists;
    }
    let Some(actual) = actual else {
        return false;
    };
    if let Some(pattern) = expected.get("regex").and_then(Value::as_str) {
        return regex::Regex::new(pattern).is_ok_and(|regex| regex.is_match(actual));
    }
    if let Some(value) = expected
        .as_str()
        .and_then(|value| value.strip_prefix("regex:"))
    {
        return regex::Regex::new(value).is_ok_and(|regex| regex.is_match(actual));
    }
    expected.as_str().is_none_or(|expected| expected == actual)
}

fn constraint_matches_any(expected: &Value, actual: &[&str]) -> bool {
    if let Some(exists) = expected.get("exists").and_then(Value::as_bool) {
        return (!actual.is_empty()) == exists;
    }
    actual
        .iter()
        .any(|value| constraint_matches(expected, Some(value)))
}

fn matches_json_schema(value: &Value, schema: &Value) -> bool {
    if let Some(allowed) = schema.get("enum").and_then(Value::as_array)
        && !allowed.contains(value)
    {
        return false;
    }
    if let Some(constant) = schema.get("const")
        && constant != value
    {
        return false;
    }
    if let Some(expected_type) = schema.get("type") {
        let type_matches = expected_type
            .as_str()
            .is_some_and(|name| json_schema_type_matches(value, name))
            || expected_type.as_array().is_some_and(|names| {
                names
                    .iter()
                    .filter_map(Value::as_str)
                    .any(|name| json_schema_type_matches(value, name))
            });
        if !type_matches {
            return false;
        }
    }
    if let Some(required) = schema.get("required").and_then(Value::as_array)
        && required
            .iter()
            .filter_map(Value::as_str)
            .any(|key| value.get(key).is_none())
    {
        return false;
    }
    if let Some(properties) = schema.get("properties").and_then(Value::as_object) {
        for (key, property_schema) in properties {
            if let Some(property) = value.get(key)
                && !matches_json_schema(property, property_schema)
            {
                return false;
            }
        }
        if schema.get("additionalProperties").and_then(Value::as_bool) == Some(false)
            && value
                .as_object()
                .is_some_and(|object| object.keys().any(|key| !properties.contains_key(key)))
        {
            return false;
        }
    }
    if let Some(items) = schema.get("items")
        && let Some(values) = value.as_array()
        && values.iter().any(|item| !matches_json_schema(item, items))
    {
        return false;
    }
    if let Some(text) = value.as_str() {
        if schema
            .get("minLength")
            .and_then(Value::as_u64)
            .is_some_and(|length| text.chars().count() < length as usize)
            || schema
                .get("maxLength")
                .and_then(Value::as_u64)
                .is_some_and(|length| text.chars().count() > length as usize)
        {
            return false;
        }
        if let Some(pattern) = schema.get("pattern").and_then(Value::as_str)
            && !regex::Regex::new(pattern).is_ok_and(|regex| regex.is_match(text))
        {
            return false;
        }
    }
    if let Some(number) = value.as_f64()
        && (schema
            .get("minimum")
            .and_then(Value::as_f64)
            .is_some_and(|minimum| number < minimum)
            || schema
                .get("maximum")
                .and_then(Value::as_f64)
                .is_some_and(|maximum| number > maximum))
    {
        return false;
    }
    if let Some(items) = value.as_array()
        && (schema
            .get("minItems")
            .and_then(Value::as_u64)
            .is_some_and(|length| items.len() < length as usize)
            || schema
                .get("maxItems")
                .and_then(Value::as_u64)
                .is_some_and(|length| items.len() > length as usize))
    {
        return false;
    }
    true
}

fn json_schema_type_matches(value: &Value, name: &str) -> bool {
    match name {
        "object" => value.is_object(),
        "array" => value.is_array(),
        "string" => value.is_string(),
        "integer" => value.as_i64().is_some() || value.as_u64().is_some(),
        "number" => value.is_number(),
        "boolean" => value.is_boolean(),
        "null" => value.is_null(),
        _ => true,
    }
}

fn render_template(
    template: &str,
    request: &Request<Body>,
    path_params: &HashMap<String, String>,
    body: &[u8],
) -> String {
    let body_json = serde_json::from_slice::<Value>(body).unwrap_or(Value::Null);
    let mut output = String::with_capacity(template.len());
    let mut rest = template;
    while let Some(start) = rest.find("{{") {
        output.push_str(&rest[..start]);
        let after = &rest[start + 2..];
        let Some(end) = after.find("}}") else {
            output.push_str(&rest[start..]);
            return output;
        };
        let raw_key = after[..end].trim();
        let key = raw_key.strip_prefix("request.").unwrap_or(raw_key);
        let value = if key == "method" {
            Some(request.method().as_str().to_owned())
        } else if key == "url" {
            Some(request.uri().to_string())
        } else if let Some(name) = key.strip_prefix("path.") {
            path_params.get(name).cloned()
        } else if let Some(name) = key.strip_prefix("query.") {
            request
                .uri()
                .query()
                .and_then(|query| reqwest::Url::parse(&format!("http://mock.local/?{query}")).ok())
                .and_then(|url| {
                    url.query_pairs()
                        .find(|(key, _)| key == name)
                        .map(|(_, value)| value.into_owned())
                })
        } else if let Some(name) = key
            .strip_prefix("header.")
            .or_else(|| key.strip_prefix("headers."))
        {
            request
                .headers()
                .get(name)
                .and_then(|value| value.to_str().ok())
                .map(str::to_owned)
        } else if let Some(pointer) = key.strip_prefix("body.") {
            body_json
                .pointer(&format!(
                    "/{}",
                    pointer
                        .split('.')
                        .map(|part| part.replace('~', "~0").replace('/', "~1"))
                        .collect::<Vec<_>>()
                        .join("/")
                ))
                .map(template_value)
        } else {
            None
        };
        if let Some(value) = value {
            output.push_str(&value);
        } else {
            output.push_str(&rest[start..start + end + 4]);
        }
        rest = &after[end + 2..];
    }
    output.push_str(rest);
    output
}

fn template_value(value: &Value) -> String {
    value
        .as_str()
        .map(str::to_owned)
        .unwrap_or_else(|| value.to_string())
}

fn is_route_parameter(segment: &str) -> bool {
    (segment.starts_with(':') && segment.len() > 1)
        || (segment.starts_with('{') && segment.ends_with('}') && segment.len() > 2)
}

fn route_specificity(path: &str) -> isize {
    if path == "/" {
        return 1000;
    }
    let segments = path.trim_matches('/').split('/').collect::<Vec<_>>();
    let literals = segments
        .iter()
        .filter(|segment| !is_route_parameter(segment.trim()) && segment.trim() != "*")
        .count();
    let params = segments
        .iter()
        .filter(|segment| is_route_parameter(segment.trim()))
        .count();
    let wildcards = segments
        .iter()
        .filter(|segment| segment.trim() == "*")
        .count();
    (literals * 100 + segments.len()) as isize - (params * 10 + wildcards * 25) as isize
}

fn route_segment_count(path: &str) -> usize {
    path.trim_matches('/')
        .split('/')
        .filter(|segment| !segment.is_empty())
        .count()
}

fn route_summary(route: &MockRoute, override_value: Option<&Value>) -> Value {
    let mut summary = json!({ "id": route.id, "requestName": route.name, "filePath": route.file_path, "method": route.method, "pathPattern": route.path, "exampleCount": route.examples.len(), "exampleNames": route.examples.iter().filter_map(|example| example.get("name").and_then(Value::as_str)).collect::<Vec<_>>() });
    if let Some(override_value) = override_value {
        summary["override"] = override_value.clone();
    }
    summary
}

fn service_route_summaries(service: &MockService) -> Vec<Value> {
    let overrides = service
        .overrides
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner);
    service
        .routes
        .iter()
        .map(|route| route_summary(route, overrides.get(&route.id)))
        .collect()
}

fn status_value(runtime: &MockRuntime, routes: Vec<Value>) -> Value {
    if runtime.service.is_none() {
        return json!({ "running": false, "host": "", "port": 0, "url": "", "target": "", "routesCount": 0, "routes": null, "nonLoopback": false });
    }
    let overrides = runtime
        .service
        .as_ref()
        .map(|service| {
            service
                .overrides
                .lock()
                .unwrap_or_else(std::sync::PoisonError::into_inner)
                .clone()
        })
        .unwrap_or_default();
    let host = if runtime.host.is_empty() {
        "127.0.0.1"
    } else {
        &runtime.host
    };
    let port = if runtime.port == 0 {
        8080
    } else {
        runtime.port
    };
    let url_host = if host.contains(':') && !host.starts_with('[') {
        format!("[{host}]")
    } else {
        host.to_owned()
    };
    let non_loopback = host != "127.0.0.1" && host != "localhost" && host != "::1";
    let mut status = json!({ "running": runtime.running, "host": host, "port": port, "url": if runtime.running { format!("http://{url_host}:{port}") } else { String::new() }, "target": runtime.target, "routesCount": routes.len(), "routes": routes, "overrides": overrides, "nonLoopback": non_loopback });
    if non_loopback {
        status["warning"] = json!(format!(
            "Mock server bound to non-loopback host {host}. This exposes your mock endpoints externally."
        ));
    }
    status
}

#[allow(clippy::too_many_arguments)]
fn record_log(
    service: &MockService,
    request: &Request<Body>,
    method: &str,
    path: &str,
    query: &str,
    status: u16,
    matched: bool,
    route_id: &str,
    example: &str,
    started_at: chrono::DateTime<chrono::Utc>,
    duration_ms: u64,
    error_message: Option<String>,
    include_headers: bool,
) {
    let id = service.sequence.fetch_add(1, Ordering::Relaxed) + 1;
    let mut log = json!({ "id": id.to_string(), "timestamp": started_at.to_rfc3339(), "method": method, "path": path, "statusCode": status, "durationMs": duration_ms, "matched": matched });
    if !query.is_empty() {
        log["query"] = json!(query);
    }
    if include_headers {
        let headers = request
            .headers()
            .iter()
            .filter(|(key, _)| {
                !matches!(
                    key.as_str(),
                    "authorization" | "cookie" | "proxy-authorization" | "set-cookie"
                )
            })
            .filter_map(|(key, value)| {
                value
                    .to_str()
                    .ok()
                    .map(|value| (key.to_string(), value.to_owned()))
            })
            .collect::<HashMap<_, _>>();
        log["headers"] = json!(headers);
    }
    if !route_id.is_empty() {
        log["matchedRoute"] = json!(route_id);
    }
    if !example.is_empty() {
        log["matchedExample"] = json!(example);
    }
    if let Some(error_message) = error_message {
        log["errorMessage"] = json!(error_message);
    }
    let mut logs = service
        .logs
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner);
    logs.push(log);
    let excess = logs.len().saturating_sub(MAX_LOGS);
    if excess > 0 {
        logs.drain(0..excess);
    }
    if let Some(log) = logs.last() {
        let _ = service.log_events.send(log.clone());
    }
}

fn random_unit() -> f64 {
    let mut bytes = [0_u8; 8];
    if getrandom::fill(&mut bytes).is_err() {
        return 0.0;
    }
    u64::from_le_bytes(bytes) as f64 / u64::MAX as f64
}

fn error_response(status: StatusCode, message: impl Into<String>) -> Response {
    (status, Json(json!({ "error": message.into() }))).into_response()
}

#[cfg(test)]
mod tests {
    use super::*;

    fn route(matcher: Value) -> MockRoute {
        MockRoute {
            id: "POST:/users/{id}".into(),
            name: "Update user".into(),
            file_path: "users/update.pebble.json".into(),
            method: "POST".into(),
            path: "/users/:id".into(),
            matcher,
            response_template: json!({}),
            examples: Vec::new(),
        }
    }

    #[test]
    fn advanced_matcher_checks_path_regex_headers_query_and_body_schema() {
        let route = route(json!({
            "pathRegex": "^/users/(?P<id>[0-9]+)$",
            "headers": { "x-tenant": "acme", "x-trace": { "regex": "^req-[0-9]+$" } },
            "query": { "active": "true", "region": { "regex": "^(us|eu)-[a-z]+$" } },
            "bodySchema": { "type": "object", "required": ["name"], "properties": { "name": { "type": "string" }, "age": { "type": "integer" } } }
        }));
        let mut headers = HeaderMap::new();
        headers.insert("x-tenant", HeaderValue::from_static("acme"));
        headers.insert("x-trace", HeaderValue::from_static("req-42"));
        let params = route_matches_request(
            &route,
            "POST",
            "/users/123",
            "active=true&region=us-west",
            &headers,
            br#"{"name":"Lin","age":31}"#,
        )
        .unwrap();
        assert_eq!(params.get("id").map(String::as_str), Some("123"));
        assert!(
            route_matches_request(
                &route,
                "POST",
                "/users/not-a-number",
                "active=true&region=us-west",
                &headers,
                br#"{"name":"Lin","age":31}"#
            )
            .is_none()
        );
        assert!(
            route_matches_request(
                &route,
                "POST",
                "/users/123",
                "active=false&region=us-west",
                &headers,
                br#"{"name":"Lin","age":31}"#
            )
            .is_none()
        );
        assert!(
            route_matches_request(
                &route,
                "POST",
                "/users/123",
                "active=true&region=us-west",
                &headers,
                br#"{"age":"31"}"#
            )
            .is_none()
        );
    }

    #[test]
    fn response_template_interpolates_path_query_header_and_json_body_values() {
        let request = Request::builder()
            .method("POST")
            .uri("/users/123?expand=roles")
            .header("x-tenant", "acme")
            .body(Body::empty())
            .unwrap();
        let params = HashMap::from([("id".to_owned(), "123".to_owned())]);
        let body = br#"{"user":{"name":"Lin"}}"#;
        let rendered = render_template(
            "{{request.method}} {{request.path.id}}?{{request.query.expand}} {{request.headers.x-tenant}} {{request.body.user.name}}",
            &request,
            &params,
            body,
        );
        assert_eq!(rendered, "POST 123?roles acme Lin");
    }

    #[test]
    fn matcher_handles_url_normalization_path_params_wildcards_and_method_case() {
        let cases = [
            (
                "https://api.example.test/users/:id?limit=10#top",
                "/users/:id",
            ),
            ("{{BASE_URL}}/v1/items/{itemId}", "/v1/items/{itemId}"),
            ("api.example.test/health", "/health"),
            ("///multiple///slashes//here", "/multiple/slashes/here"),
            ("", "/"),
        ];
        for (input, expected) in cases {
            assert_eq!(url_path(input), expected, "{input:?}");
        }

        assert_eq!(
            match_route_params("/users/:id", "/users/42")
                .unwrap()
                .get("id")
                .map(String::as_str),
            Some("42")
        );
        assert_eq!(
            match_route_params("/items/{itemId}", "/items/item_abc")
                .unwrap()
                .get("itemId")
                .map(String::as_str),
            Some("item_abc")
        );
        assert!(match_route_params("/users/*", "/users/42/details").is_some());
        assert!(match_route_params("/users/profile", "/organizations/profile").is_none());

        let route = route(json!({}));
        assert!(
            route_matches_request(&route, "post", "/users/42", "", &HeaderMap::new(), b"")
                .is_some()
        );
        assert!(
            route_matches_request(&route, "GET", "/users/42", "", &HeaderMap::new(), b"").is_none()
        );
    }

    #[test]
    fn mock_example_selection_supports_headers_prefer_and_fallback() {
        let examples = vec![
            json!({"name":"Default 200","statusCode":200}),
            json!({"name":"Error 404","statusCode":404}),
            json!({"name":"Unauthorized","statusCode":401}),
        ];
        assert_eq!(
            pick_example(&examples, &HeaderMap::new()).unwrap()["name"],
            "Default 200"
        );

        let mut headers = HeaderMap::new();
        headers.insert("x-mock-example", HeaderValue::from_static("Error 404"));
        assert_eq!(
            pick_example(&examples, &headers).unwrap()["statusCode"],
            404
        );
        headers.remove("x-mock-example");
        headers.insert(
            "prefer",
            HeaderValue::from_static("return=minimal, example=\"Unauthorized\""),
        );
        assert_eq!(
            pick_example(&examples, &headers).unwrap()["statusCode"],
            401
        );
        headers.insert("x-mock-example", HeaderValue::from_static("missing"));
        assert_eq!(
            pick_example(&examples, &headers).unwrap()["name"],
            "Default 200"
        );
    }

    #[tokio::test]
    async fn mock_handler_applies_runtime_override_templates_and_records_match_details() {
        let mut route = route(json!({}));
        route.examples = vec![json!({
            "name":"Created", "statusCode":201, "contentType":"application/json",
            "body":"{\"id\":\"{{request.path.id}}\"}"
        })];
        let (log_events, _) = broadcast::channel(4);
        let (stop_events, _) = broadcast::channel(1);
        let overrides = Arc::new(Mutex::new(HashMap::from([(
            route.id.clone(),
            json!({
                "statusCode":202,
                "responseTemplate":{
                    "contentType":"application/json",
                    "body":"{\"id\":\"{{request.path.id}}\",\"version\":\"{{request.query.version}}\"}",
                    "headers":{"x-mock-version":"{{request.query.version}}"}
                }
            }),
        )])));
        let service = MockService {
            routes: vec![route],
            global_delay_ms: 0,
            global_status: 0,
            global_error_rate: 0.0,
            overrides,
            logs: Arc::new(Mutex::new(Vec::new())),
            log_events,
            stop_events,
            sequence: Arc::new(AtomicU64::new(0)),
        };
        let request = Request::builder()
            .method("POST")
            .uri("/users/42?version=v3")
            .header("authorization", "secret-token")
            .body(Body::from(r#"{"name":"Ada"}"#))
            .unwrap();
        let response = mock_request(State(service.clone()), request).await;
        assert_eq!(response.status(), StatusCode::ACCEPTED);
        assert_eq!(response.headers()[header::CONTENT_TYPE], "application/json");
        assert_eq!(response.headers()["x-mock-version"], "v3");
        let body = to_bytes(response.into_body(), 4096).await.unwrap();
        assert_eq!(
            serde_json::from_slice::<Value>(&body).unwrap(),
            json!({"id":"42","version":"v3"})
        );

        {
            let logs = service.logs.lock().unwrap();
            assert_eq!(logs.len(), 1);
            assert_eq!(logs[0]["matched"], true);
            assert_eq!(logs[0]["statusCode"], 202);
            assert_eq!(logs[0]["matchedRoute"], "POST:/users/{id}");
            assert!(logs[0]["timestamp"].as_str().is_some());
            assert!(logs[0]["headers"]["authorization"].is_null());
        }

        let miss = Request::builder()
            .method("POST")
            .uri("/missing")
            .body(Body::empty())
            .unwrap();
        let response = mock_request(State(service.clone()), miss).await;
        assert_eq!(response.status(), StatusCode::NOT_FOUND);
        let logs = service.logs.lock().unwrap();
        assert_eq!(logs.len(), 2);
        assert_eq!(logs[1]["matched"], false);
        assert_eq!(
            logs[1]["errorMessage"],
            "No route matched request method and path"
        );
    }
}
