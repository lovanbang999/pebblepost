use std::collections::HashMap;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use axum::Json;
use axum::extract::{Query, State};
use axum::http::{HeaderName, HeaderValue, StatusCode};
use axum::response::sse::{Event, KeepAlive};
use axum::response::{IntoResponse, Response, Sse};
use base64::Engine;
use futures_util::{SinkExt, StreamExt, stream};
use pebblepost_core::{build_variable_map, interpolate_value, list_environments, resolve_request};
use serde::Deserialize;
use serde_json::{Value, json};
use tokio::sync::{mpsc, oneshot};
use tokio_tungstenite::tungstenite::Message;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;

use crate::AppState;

#[derive(Default)]
pub(super) struct StreamManager {
    sessions: Mutex<HashMap<String, StreamSession>>,
    next_generation: AtomicU64,
}

struct StreamSession {
    protocol: String,
    status: Arc<Mutex<Value>>,
    outgoing: Option<mpsc::Sender<OutgoingMessage>>,
    connected: Option<Arc<AtomicBool>>,
    stop: oneshot::Sender<()>,
    finished: oneshot::Receiver<()>,
    generation: u64,
}

struct OutgoingMessage {
    message: Message,
    payload: String,
    kind: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub(super) struct ConnectPayload {
    stream_id: String,
    workspace_path: Option<String>,
    environment_name: Option<String>,
    path: Option<String>,
    request: Value,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct SendPayload {
    pub(crate) stream_id: String,
    pub(crate) payload: String,
    #[serde(default = "default_message_type", rename = "type")]
    pub(crate) message_type: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct DisconnectPayload {
    pub(crate) stream_id: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct StatusQuery {
    pub(crate) stream_id: Option<String>,
}

fn default_message_type() -> String {
    "text".to_owned()
}

pub(super) async fn status(
    State(state): State<Arc<AppState>>,
    Query(query): Query<StatusQuery>,
) -> Response {
    let Some(stream_id) = query
        .stream_id
        .filter(|stream_id| !stream_id.trim().is_empty())
    else {
        return api_error(
            StatusCode::BAD_REQUEST,
            "streamId query parameter is required",
        );
    };
    let sessions = state
        .stream_manager
        .sessions
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner);
    let status = sessions
        .get(&stream_id)
        .map(|session| {
            session
                .status
                .lock()
                .unwrap_or_else(std::sync::PoisonError::into_inner)
                .clone()
        })
        .unwrap_or_else(|| {
            json!({
                "streamId": stream_id,
                "protocol": "",
                "state": "disconnected",
                "url": "",
                "reconnectCount": 0,
                "totalSent": 0,
                "totalReceived": 0,
                "evictedCount": 0
            })
        });
    Json(status).into_response()
}

fn close_code_description(code: u16) -> String {
    match code {
        1000 => "Normal Closure (1000)".to_owned(),
        1001 => "Going Away (1001)".to_owned(),
        1002 => "Protocol Error (1002)".to_owned(),
        1003 => "Unsupported Data (1003)".to_owned(),
        1005 => "No Status Received (1005)".to_owned(),
        1006 => "Abnormal Closure (1006)".to_owned(),
        1007 => "Invalid Frame Payload Data (1007)".to_owned(),
        1008 => "Policy Violation (1008)".to_owned(),
        1009 => "Message Too Big (1009)".to_owned(),
        1010 => "Mandatory Extension (1010)".to_owned(),
        1011 => "Internal Server Error (1011)".to_owned(),
        1015 => "TLS Handshake Failed (1015)".to_owned(),
        code if code > 0 => format!("Close Code {code}"),
        _ => String::new(),
    }
}

fn is_graphql_ws(request: &Value) -> bool {
    request.get("protocol").and_then(Value::as_str) == Some("graphql-ws")
        || request
            .pointer("/body/graphql/query")
            .and_then(Value::as_str)
            .is_some_and(|query| query.trim_start().starts_with("subscription"))
        || request
            .pointer("/stream/subprotocols")
            .and_then(Value::as_array)
            .is_some_and(|items| {
                items.iter().any(|item| {
                    item.as_str()
                        .is_some_and(|protocol| protocol == "graphql-transport-ws")
                })
            })
}

fn graphql_connection_init(connection_params: &Value) -> String {
    if connection_params
        .as_object()
        .is_some_and(|params| !params.is_empty())
    {
        json!({ "type": "connection_init", "payload": connection_params }).to_string()
    } else {
        json!({ "type": "connection_init" }).to_string()
    }
}

fn graphql_subscribe_message(
    id: &str,
    query: &str,
    variables: &Value,
    operation_name: &str,
) -> String {
    let mut payload = json!({ "query": query });
    if variables.as_object().is_some_and(|items| !items.is_empty()) {
        payload["variables"] = variables.clone();
    }
    if !operation_name.trim().is_empty() {
        payload["operationName"] = json!(operation_name.trim());
    }
    json!({ "id": id, "type": "subscribe", "payload": payload }).to_string()
}

fn graphql_outgoing_message(payload: &str) -> (Message, String, String) {
    if serde_json::from_str::<Value>(payload)
        .ok()
        .is_some_and(|value| value.get("type").is_some_and(|kind| !kind.is_null()))
    {
        return (
            Message::Text(payload.to_owned().into()),
            payload.to_owned(),
            "text".into(),
        );
    }
    let id = chrono::Utc::now()
        .timestamp_nanos_opt()
        .unwrap_or_else(|| chrono::Utc::now().timestamp_micros() * 1_000);
    let message = graphql_subscribe_message(&id.to_string(), payload, &json!({}), "");
    (
        Message::Text(message.clone().into()),
        message,
        "text".into(),
    )
}

pub(super) async fn connect(
    State(state): State<Arc<AppState>>,
    Json(payload): Json<ConnectPayload>,
) -> Response {
    if payload.stream_id.trim().is_empty() {
        return api_error(StatusCode::BAD_REQUEST, "streamId is required");
    }
    let request = match resolve_stream_request(&payload) {
        Ok(request) => request,
        Err(error) => return api_error(StatusCode::BAD_REQUEST, &error),
    };
    let is_sse = request.get("protocol").and_then(Value::as_str) == Some("sse")
        || request.get("method").and_then(Value::as_str) == Some("SSE");
    let protocol = if is_sse {
        "sse"
    } else if is_graphql_ws(&request) {
        "graphql-ws"
    } else {
        "websocket"
    };
    let (events_tx, events_rx) = mpsc::channel::<(String, String)>(256);
    let (stop_tx, stop_rx) = oneshot::channel();

    let mut session = match protocol {
        "sse" => {
            match connect_sse(&payload.stream_id, &request, events_tx.clone(), stop_rx).await {
                Ok(session) => session,
                Err(error) => return api_error(StatusCode::BAD_GATEWAY, &error),
            }
        }
        _ => {
            match connect_websocket(&payload.stream_id, &request, events_tx.clone(), stop_rx).await
            {
                Ok(session) => session,
                Err(error) => return api_error(StatusCode::BAD_GATEWAY, &error),
            }
        }
    };
    session.stop = stop_tx;

    let generation = state
        .stream_manager
        .next_generation
        .fetch_add(1, Ordering::Relaxed);
    session.generation = generation;
    let stream_id = payload.stream_id;
    let finished = std::mem::replace(&mut session.finished, oneshot::channel().1);
    let previous = state
        .stream_manager
        .sessions
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner)
        .insert(stream_id.clone(), session);
    if let Some(previous) = previous {
        let _ = previous.stop.send(());
    }
    let manager = state.stream_manager.clone();
    tokio::spawn(async move {
        let _ = finished.await;
        let mut sessions = manager
            .sessions
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        if sessions
            .get(&stream_id)
            .is_some_and(|session| session.generation == generation)
        {
            sessions.remove(&stream_id);
        }
    });

    let events = stream::unfold(events_rx, |mut receiver| async move {
        receiver.recv().await.map(|(event, data)| {
            (
                Ok::<Event, std::convert::Infallible>(Event::default().event(event).data(data)),
                receiver,
            )
        })
    });
    Sse::new(events)
        .keep_alive(
            KeepAlive::new()
                .interval(Duration::from_secs(15))
                .text("keep-alive"),
        )
        .into_response()
}

async fn connect_websocket(
    stream_id: &str,
    request: &Value,
    events: mpsc::Sender<(String, String)>,
    mut stop: oneshot::Receiver<()>,
) -> Result<StreamSession, String> {
    let graphql_ws = is_graphql_ws(request);
    let url = stream_url(request)?;
    if !matches!(url.scheme(), "ws" | "wss") {
        return Err("WebSocket URLs must use ws:// or wss://".into());
    }
    let mut handshake = url
        .as_str()
        .into_client_request()
        .map_err(|error| format!("invalid WebSocket handshake: {error}"))?;
    append_request_headers(request, handshake.headers_mut()).await?;
    if graphql_ws {
        handshake.headers_mut().insert(
            "sec-websocket-protocol",
            HeaderValue::from_static("graphql-transport-ws"),
        );
    } else if let Some(protocols) = request
        .pointer("/stream/subprotocols")
        .and_then(Value::as_array)
    {
        let protocols = protocols
            .iter()
            .filter_map(Value::as_str)
            .map(str::trim)
            .filter(|protocol| !protocol.is_empty())
            .collect::<Vec<_>>();
        if !protocols.is_empty() {
            let value = HeaderValue::from_bytes(protocols.join(", ").as_bytes())
                .map_err(|error| format!("invalid WebSocket subprotocol: {error}"))?;
            handshake
                .headers_mut()
                .insert("sec-websocket-protocol", value);
        }
    }
    let timeout_ms = request
        .pointer("/settings/connectTimeoutMs")
        .and_then(Value::as_u64)
        .filter(|value| *value > 0)
        .unwrap_or(10_000);
    let (socket, response) = tokio::time::timeout(
        Duration::from_millis(timeout_ms),
        tokio_tungstenite::connect_async(handshake.clone()),
    )
    .await
    .map_err(|_| format!("WebSocket connection timed out after {timeout_ms} ms"))?
    .map_err(|error| format!("WebSocket connection failed: {error}"))?;
    let negotiated_protocol = response
        .headers()
        .get("sec-websocket-protocol")
        .and_then(|value| value.to_str().ok())
        .unwrap_or("")
        .to_owned();
    let ping_interval_ms = request
        .pointer("/stream/pingIntervalMs")
        .and_then(Value::as_u64)
        .filter(|value| *value > 0);
    let auto_reconnect = request
        .pointer("/stream/autoReconnect")
        .and_then(Value::as_bool)
        .unwrap_or(false);
    let max_reconnect_attempts = request
        .pointer("/stream/maxReconnectAttempts")
        .and_then(Value::as_u64)
        .filter(|value| *value > 0)
        .unwrap_or(5);
    let reconnect_interval = Duration::from_millis(
        request
            .pointer("/stream/reconnectIntervalMs")
            .and_then(Value::as_u64)
            .filter(|value| *value > 0)
            .unwrap_or(1_000),
    );
    let connect_timeout = Duration::from_millis(timeout_ms);
    let graphql_query = request
        .pointer("/body/graphql/query")
        .and_then(Value::as_str)
        .unwrap_or("")
        .to_owned();
    let graphql_variables = request
        .pointer("/body/graphql/variables")
        .and_then(Value::as_str)
        .filter(|raw| !raw.trim().is_empty())
        .and_then(|raw| serde_json::from_str::<Value>(raw).ok())
        .filter(|value| value.is_object())
        .unwrap_or_else(|| json!({}));
    let graphql_operation_name = request
        .pointer("/body/graphql/operationName")
        .and_then(Value::as_str)
        .unwrap_or("")
        .trim()
        .to_owned();
    let connection_params = request
        .pointer("/stream/connectionParams")
        .cloned()
        .unwrap_or_else(|| json!({}));
    let connected = Arc::new(AtomicBool::new(true));
    let reconnect_handshake = handshake.clone();
    let (outgoing, mut outgoing_rx) = mpsc::channel::<OutgoingMessage>(128);
    let events_for_task = events.clone();
    let url_text = url.to_string();
    let stream_id = stream_id.to_owned();
    let protocol = if graphql_ws {
        "graphql-ws"
    } else {
        "websocket"
    }
    .to_owned();
    let mut initial_status = json!({
        "streamId": stream_id,
        "protocol": protocol,
        "state": "connected",
        "url": url_text,
        "reconnectCount": 0,
        "totalSent": 0,
        "totalReceived": 0,
        "evictedCount": 0
    });
    if !negotiated_protocol.is_empty() {
        initial_status["subprotocol"] = json!(negotiated_protocol);
    }
    let status = Arc::new(Mutex::new(initial_status));
    let status_for_task = status.clone();
    let (finished_sender, finished_receiver) = oneshot::channel();
    let connected_for_task = connected.clone();
    tokio::spawn(async move {
        let mut index = 0usize;
        let mut sent = 0u64;
        let mut received = 0u64;
        let mut reconnect_count = 0u64;
        let mut close_code = 0u16;
        let mut close_reason = String::new();
        let mut negotiated_protocol = negotiated_protocol;
        let mut socket = Some(socket);
        let mut stopped = false;

        'session: loop {
            if socket.is_none() {
                if !auto_reconnect {
                    break;
                }
                if reconnect_count >= max_reconnect_attempts {
                    emit_log(
                        &events_for_task,
                        &mut index,
                        "system",
                        "error",
                        &format!("Auto-reconnect failed after {reconnect_count} attempts"),
                        true,
                    )
                    .await;
                    break;
                }
                let attempt = reconnect_count + 1;
                let mut delay = reconnect_interval.min(Duration::from_secs(30));
                for _ in 1..attempt {
                    delay = delay.mul_f64(1.5).min(Duration::from_secs(30));
                }
                reconnect_count = attempt;
                connected_for_task.store(false, Ordering::Release);
                emit_status(
                    &events_for_task,
                    &status_for_task,
                    &stream_id,
                    &url_text,
                    "websocket",
                    "reconnecting",
                    sent,
                    received,
                    reconnect_count,
                    &negotiated_protocol,
                    Some((close_code, close_reason.clone())),
                )
                .await;
                emit_log(&events_for_task, &mut index, "system", "open", &format!("Attempting to reconnect ({attempt}/{max_reconnect_attempts}) in {delay:?}..."), false).await;
                tokio::select! {
                    _ = &mut stop => { break 'session; },
                    _ = tokio::time::sleep(delay) => {},
                }
                let reconnect = tokio::time::timeout(
                    connect_timeout,
                    tokio_tungstenite::connect_async(reconnect_handshake.clone()),
                );
                tokio::select! {
                    _ = &mut stop => { break 'session; },
                    result = reconnect => match result {
                        Ok(Ok((new_socket, response))) => {
                            negotiated_protocol = response.headers()
                                .get("sec-websocket-protocol")
                                .and_then(|value| value.to_str().ok())
                                .unwrap_or("")
                                .to_owned();
                            socket = Some(new_socket);
                            close_code = 0;
                            close_reason.clear();
                            connected_for_task.store(true, Ordering::Release);
                            continue 'session;
                        }
                        Ok(Err(error)) => {
                            close_code = 1006;
                            close_reason = error.to_string();
                            emit_log(&events_for_task, &mut index, "system", "error", &format!("Connection failed: {error}"), true).await;
                        }
                        Err(_) => {
                            close_code = 1006;
                            close_reason = format!("WebSocket connection timed out after {} ms", connect_timeout.as_millis());
                            emit_log(&events_for_task, &mut index, "system", "error", &close_reason, true).await;
                        }
                    }
                }
                continue 'session;
            }

            let current_socket = socket.take().expect("socket was checked above");
            let (mut sink, mut source) = current_socket.split();
            let open_message = if negotiated_protocol.is_empty() {
                format!("Connected to {url_text}")
            } else {
                format!("Connected to {url_text} (Subprotocol: {negotiated_protocol})")
            };
            emit_log(
                &events_for_task,
                &mut index,
                "system",
                "open",
                &open_message,
                false,
            )
            .await;
            if graphql_ws {
                let init = graphql_connection_init(&connection_params);
                if let Err(error) = sink.send(Message::Text(init.clone().into())).await {
                    close_code = 1006;
                    close_reason = error.to_string();
                    connected_for_task.store(false, Ordering::Release);
                    emit_log(
                        &events_for_task,
                        &mut index,
                        "system",
                        "error",
                        &format!("Failed to send connection_init: {error}"),
                        true,
                    )
                    .await;
                    socket = None;
                    continue 'session;
                }
                sent += 1;
                emit_log(&events_for_task, &mut index, "send", "text", &init, false).await;
            }
            let status_protocol = if graphql_ws {
                "graphql-ws"
            } else {
                "websocket"
            };
            emit_status(
                &events_for_task,
                &status_for_task,
                &stream_id,
                &url_text,
                status_protocol,
                "connected",
                sent,
                received,
                reconnect_count,
                &negotiated_protocol,
                None,
            )
            .await;
            let mut ping_interval = ping_interval_ms
                .map(|interval| tokio::time::interval(Duration::from_millis(interval)));
            if let Some(interval) = ping_interval.as_mut() {
                interval.tick().await;
            }
            let mut reconnect_after_disconnect = false;
            'connection: loop {
                tokio::select! {
                    _ = &mut stop => {
                        if graphql_ws {
                            let complete = json!({ "id": "1", "type": "complete" }).to_string();
                            let _ = sink.send(Message::Text(complete.into())).await;
                        }
                        let _ = sink.send(Message::Close(None)).await;
                        close_code = 1000;
                        close_reason = "Disconnected by user".into();
                        connected_for_task.store(false, Ordering::Release);
                        emit_log(&events_for_task, &mut index, "system", "close", &close_reason, false).await;
                        stopped = true;
                        break 'connection;
                    }
                    outgoing = outgoing_rx.recv() => {
                        let Some(outgoing) = outgoing else { break 'session; };
                        let (message, payload, kind) = if graphql_ws && outgoing.kind == "text" {
                            graphql_outgoing_message(&outgoing.payload)
                        } else {
                            (outgoing.message, outgoing.payload, outgoing.kind)
                        };
                        if let Err(error) = sink.send(message).await {
                            close_code = 1006;
                            close_reason = error.to_string();
                            connected_for_task.store(false, Ordering::Release);
                            emit_log(&events_for_task, &mut index, "system", "error", &format!("WebSocket send failed: {error}"), true).await;
                            reconnect_after_disconnect = true;
                            break 'connection;
                        }
                        sent += 1;
                        emit_log(&events_for_task, &mut index, "send", &kind, &payload, false).await;
                    }
                    _ = next_ping_tick(&mut ping_interval) => {
                        if let Err(error) = sink.send(Message::Ping(b"ping".to_vec().into())).await {
                            close_code = 1006;
                            close_reason = error.to_string();
                            connected_for_task.store(false, Ordering::Release);
                            emit_log(&events_for_task, &mut index, "system", "error", &format!("WebSocket ping failed: {error}"), true).await;
                            reconnect_after_disconnect = true;
                            break 'connection;
                        }
                        emit_log(&events_for_task, &mut index, "send", "ping", "ping", false).await;
                    }
                    incoming = source.next() => {
                        match incoming {
                            Some(Ok(Message::Text(text))) => {
                                received += 1;
                                let payload = text.to_string();
                                if graphql_ws {
                                    let parsed_msg = serde_json::from_str::<Value>(&payload).ok();
                                    let msg_kind = parsed_msg.as_ref().and_then(|m| m.get("type")).and_then(Value::as_str).map(|k| k.to_owned());
                                    match msg_kind.as_deref() {
                                        Some("connection_ack") => {
                                            emit_log(&events_for_task, &mut index, "receive", "text", &payload, false).await;
                                            if !graphql_query.trim().is_empty() {
                                                let subscription = graphql_subscribe_message("1", &graphql_query, &graphql_variables, &graphql_operation_name);
                                                if let Err(error) = sink.send(Message::Text(subscription.clone().into())).await {
                                                    close_code = 1006;
                                                    close_reason = error.to_string();
                                                    connected_for_task.store(false, Ordering::Release);
                                                    emit_log(&events_for_task, &mut index, "system", "error", &format!("Failed to send subscribe message: {error}"), true).await;
                                                    reconnect_after_disconnect = true;
                                                    break 'connection;
                                                }
                                                sent += 1;
                                                emit_log(&events_for_task, &mut index, "send", "text", &subscription, false).await;
                                            }
                                        }
                                        Some("error") => emit_log(&events_for_task, &mut index, "receive", "error", &payload, true).await,
                                        Some("ping") => {
                                            emit_log(&events_for_task, &mut index, "receive", "ping", &payload, false).await;
                                            let pong = json!({ "type": "pong" }).to_string();
                                            if let Err(error) = sink.send(Message::Text(pong.clone().into())).await {
                                                close_code = 1006;
                                                close_reason = error.to_string();
                                                connected_for_task.store(false, Ordering::Release);
                                                reconnect_after_disconnect = true;
                                                break 'connection;
                                            }
                                            sent += 1;
                                            emit_log(&events_for_task, &mut index, "send", "pong", &pong, false).await;
                                        }
                                        Some("pong") => emit_log(&events_for_task, &mut index, "receive", "pong", &payload, false).await,
                                        _ => emit_log(&events_for_task, &mut index, "receive", "text", &payload, false).await,
                                    }
                                } else {
                                    emit_log(&events_for_task, &mut index, "receive", "text", &payload, false).await;
                                }
                            }
                            Some(Ok(Message::Binary(bytes))) => {
                                received += 1;
                                let text = String::from_utf8_lossy(&bytes).into_owned();
                                emit_log(&events_for_task, &mut index, "receive", "binary", &text, false).await;
                            }
                            Some(Ok(Message::Ping(bytes))) => {
                                let payload = String::from_utf8_lossy(&bytes).into_owned();
                                emit_log(&events_for_task, &mut index, "receive", "ping", &payload, false).await;
                                if let Err(error) = sink.send(Message::Pong(bytes)).await {
                                    close_code = 1006;
                                    close_reason = error.to_string();
                                    connected_for_task.store(false, Ordering::Release);
                                    reconnect_after_disconnect = true;
                                    break 'connection;
                                }
                            }
                            Some(Ok(Message::Pong(bytes))) => {
                                let payload = String::from_utf8_lossy(&bytes).into_owned();
                                emit_log(&events_for_task, &mut index, "receive", "pong", &payload, false).await;
                            }
                            Some(Ok(Message::Close(frame))) => {
                                connected_for_task.store(false, Ordering::Release);
                                if let Some(frame) = frame {
                                    close_code = frame.code.into();
                                    close_reason = frame.reason.to_string();
                                } else {
                                    close_code = 1005;
                                    close_reason.clear();
                                }
                                let description = close_code_description(close_code);
                                emit_log(&events_for_task, &mut index, "system", "close", &format!("Connection closed: {description} (Reason: {close_reason})"), false).await;
                                reconnect_after_disconnect = true;
                                break 'connection;
                            }
                            Some(Ok(Message::Frame(_))) => {}
                            Some(Err(error)) => {
                                close_code = 1006;
                                close_reason = error.to_string();
                                connected_for_task.store(false, Ordering::Release);
                                let description = close_code_description(close_code);
                                emit_log(&events_for_task, &mut index, "system", "close", &format!("Connection closed: {description} (Reason: {close_reason})"), false).await;
                                reconnect_after_disconnect = true;
                                break 'connection;
                            }
                            None => {
                                close_code = 1006;
                                close_reason = "connection closed".into();
                                connected_for_task.store(false, Ordering::Release);
                                let description = close_code_description(close_code);
                                emit_log(&events_for_task, &mut index, "system", "close", &format!("Connection closed: {description} (Reason: {close_reason})"), false).await;
                                reconnect_after_disconnect = true;
                                break 'connection;
                            }
                        }
                    }
                }
            }
            if stopped {
                break 'session;
            }
            if reconnect_after_disconnect {
                connected_for_task.store(false, Ordering::Release);
                socket = None;
                continue 'session;
            }
            break 'session;
        }
        connected_for_task.store(false, Ordering::Release);
        let status_protocol = if graphql_ws {
            "graphql-ws"
        } else {
            "websocket"
        };
        emit_status(
            &events_for_task,
            &status_for_task,
            &stream_id,
            &url_text,
            status_protocol,
            "disconnected",
            sent,
            received,
            reconnect_count,
            &negotiated_protocol,
            Some((close_code, close_reason)),
        )
        .await;
        let _ = finished_sender.send(());
    });
    Ok(StreamSession {
        protocol,
        status,
        outgoing: Some(outgoing),
        connected: Some(connected),
        stop: oneshot::channel().0,
        finished: finished_receiver,
        generation: 0,
    })
}

async fn next_ping_tick(interval: &mut Option<tokio::time::Interval>) {
    match interval {
        Some(interval) => {
            interval.tick().await;
        }
        None => std::future::pending::<()>().await,
    }
}

async fn connect_sse(
    stream_id: &str,
    request: &Value,
    events: mpsc::Sender<(String, String)>,
    mut stop: oneshot::Receiver<()>,
) -> Result<StreamSession, String> {
    let url = stream_url(request)?;
    if url.scheme() != "http" && url.scheme() != "https" {
        return Err("SSE URLs must use http:// or https://".into());
    }
    let settings = request.get("settings").cloned().unwrap_or(Value::Null);
    let mut builder = reqwest::Client::builder()
        .connect_timeout(Duration::from_millis(10_000))
        .danger_accept_invalid_certs(
            !settings
                .get("verifySSL")
                .and_then(Value::as_bool)
                .unwrap_or(true),
        );
    if let Some(proxy) = settings
        .get("proxyUrl")
        .and_then(Value::as_str)
        .filter(|value| !value.is_empty())
    {
        builder = builder.proxy(
            reqwest::Proxy::all(proxy).map_err(|error| format!("invalid proxy URL: {error}"))?,
        );
    }
    let client = builder.build().map_err(|error| error.to_string())?;
    let mut header_map = reqwest::header::HeaderMap::new();
    append_request_headers(request, &mut header_map).await?;
    let outgoing = sse_request(&client, &url, &header_map, None);
    let response = outgoing
        .send()
        .await
        .map_err(|error| format!("SSE connection failed: {error}"))?;
    if !response.status().is_success() {
        return Err(format!(
            "SSE server responded with HTTP {}",
            response.status()
        ));
    }
    let auto_reconnect = request
        .pointer("/stream/autoReconnect")
        .and_then(Value::as_bool)
        .unwrap_or(false);
    let max_reconnect_attempts = request
        .pointer("/stream/maxReconnectAttempts")
        .and_then(Value::as_u64)
        .filter(|value| *value > 0)
        .unwrap_or(5);
    let reconnect_interval = Duration::from_millis(
        request
            .pointer("/stream/reconnectIntervalMs")
            .and_then(Value::as_u64)
            .filter(|value| *value > 0)
            .unwrap_or(1_000),
    );
    let url_text = url.to_string();
    let event_tx = events.clone();
    let stream_id = stream_id.to_owned();
    let client_for_task = client.clone();
    let request_headers = header_map.clone();
    let status = Arc::new(Mutex::new(json!({
        "streamId": stream_id,
        "protocol": "sse",
        "state": "connected",
        "url": url_text,
        "reconnectCount": 0,
        "totalSent": 0,
        "totalReceived": 0,
        "evictedCount": 0
    })));
    let status_for_task = status.clone();
    let (finished_sender, finished_receiver) = oneshot::channel();
    tokio::spawn(async move {
        let mut index = 0usize;
        let mut received = 0u64;
        let mut reconnect_count = 0u64;
        let mut last_event_id = String::new();
        let mut current_response = Some(response);
        let mut stopped = false;
        loop {
            if current_response.is_none() {
                if !auto_reconnect {
                    break;
                }
                if reconnect_count >= max_reconnect_attempts {
                    emit_log(
                        &event_tx,
                        &mut index,
                        "system",
                        "error",
                        &format!("Auto-reconnect failed after {reconnect_count} attempts"),
                        true,
                    )
                    .await;
                    break;
                }
                let attempt = reconnect_count + 1;
                let mut delay = reconnect_interval.min(Duration::from_secs(30));
                for _ in 1..attempt {
                    delay = delay.mul_f64(1.5).min(Duration::from_secs(30));
                }
                reconnect_count = attempt;
                emit_status(
                    &event_tx,
                    &status_for_task,
                    &stream_id,
                    &url_text,
                    "sse",
                    "reconnecting",
                    0,
                    received,
                    reconnect_count,
                    "",
                    None,
                )
                .await;
                emit_log(&event_tx, &mut index, "system", "open", &format!("Attempting to reconnect SSE ({attempt}/{max_reconnect_attempts}) in {delay:?}..."), false).await;
                tokio::select! {
                    _ = &mut stop => { stopped = true; break; },
                    _ = tokio::time::sleep(delay) => {},
                }
                let last_id = (!last_event_id.is_empty()).then_some(last_event_id.as_str());
                tokio::select! {
                    _ = &mut stop => { stopped = true; break; },
                    response = sse_request(&client_for_task, &url, &request_headers, last_id).send() => match response {
                        Ok(response) if response.status().is_success() => {
                            current_response = Some(response);
                            continue;
                        }
                        Ok(response) => {
                            emit_log(&event_tx, &mut index, "system", "error", &format!("SSE server responded with HTTP {}", response.status()), true).await;
                        }
                        Err(error) => {
                            emit_log(&event_tx, &mut index, "system", "error", &format!("SSE connection failed: {error}"), true).await;
                        }
                    }
                }
                continue;
            }

            let response = current_response.take().expect("checked above");
            emit_log(
                &event_tx,
                &mut index,
                "system",
                "open",
                &format!("Connected to SSE {url_text}"),
                false,
            )
            .await;
            emit_status(
                &event_tx,
                &status_for_task,
                &stream_id,
                &url_text,
                "sse",
                "connected",
                0,
                received,
                reconnect_count,
                "",
                None,
            )
            .await;
            let mut bytes = response.bytes_stream();
            let mut pending = Vec::<u8>::new();
            let mut should_reconnect = false;
            loop {
                tokio::select! {
                    _ = &mut stop => { stopped = true; break; },
                    chunk = bytes.next() => match chunk {
                        Some(Ok(chunk)) => {
                            pending.extend_from_slice(&chunk);
                            while let Some(end) = find_sse_boundary(&pending) {
                                let block = pending.drain(..end.0).collect::<Vec<_>>();
                                pending.drain(..end.1);
                                if let Some((event_type, data, event_id)) = parse_sse_event(&block) {
                                    if let Some(event_id) = event_id.filter(|id| !id.is_empty()) {
                                        last_event_id = event_id;
                                    }
                                    received += 1;
                                    emit_log(&event_tx, &mut index, "receive", &event_type, &data, false).await;
                                    emit_status(&event_tx, &status_for_task, &stream_id, &url_text, "sse", "connected", 0, received, reconnect_count, "", None).await;
                                }
                            }
                            if pending.len() > 1024 * 1024 {
                                emit_log(&event_tx, &mut index, "system", "error", "SSE event exceeded the 1 MiB limit", true).await;
                                should_reconnect = true;
                                break;
                            }
                        }
                        Some(Err(error)) => {
                            emit_log(&event_tx, &mut index, "system", "error", &format!("SSE stream interrupted: {error}"), true).await;
                            should_reconnect = true;
                            break;
                        }
                        None => {
                            emit_log(&event_tx, &mut index, "system", "close", "SSE stream ended by server", false).await;
                            should_reconnect = true;
                            break;
                        }
                    }
                }
            }
            if stopped || !should_reconnect {
                break;
            }
            current_response = None;
        }
        if stopped {
            emit_log(
                &event_tx,
                &mut index,
                "system",
                "close",
                "Disconnected by user",
                false,
            )
            .await;
        }
        emit_status(
            &event_tx,
            &status_for_task,
            &stream_id,
            &url_text,
            "sse",
            "disconnected",
            0,
            received,
            reconnect_count,
            "",
            None,
        )
        .await;
        let _ = finished_sender.send(());
    });
    Ok(StreamSession {
        protocol: "sse".to_owned(),
        status,
        outgoing: None,
        connected: None,
        stop: oneshot::channel().0,
        finished: finished_receiver,
        generation: 0,
    })
}

pub(super) async fn execute(request: &Value, timeout_override_ms: Option<u64>) -> Value {
    let started = std::time::Instant::now();
    let started_at = chrono::Utc::now().to_rfc3339();
    let is_sse = request.get("protocol").and_then(Value::as_str) == Some("sse")
        || request.get("method").and_then(Value::as_str) == Some("SSE");
    let stream_timeout = request
        .pointer("/stream/timeoutMs")
        .and_then(Value::as_u64)
        .filter(|value| *value > 0)
        .or(timeout_override_ms.filter(|value| *value > 0))
        .or_else(|| {
            request
                .pointer("/settings/timeoutMs")
                .and_then(Value::as_u64)
                .filter(|value| *value > 0)
        })
        .unwrap_or(5_000);
    let timeout = Duration::from_millis(stream_timeout);
    let stream_id = format!("runner-{}", chrono::Utc::now().timestamp_micros());
    let (events_tx, mut events_rx) = mpsc::channel::<(String, String)>(256);
    let (stop_sender, stop_receiver) = oneshot::channel();

    let connection = async {
        if is_sse {
            connect_sse(&stream_id, request, events_tx, stop_receiver).await
        } else {
            connect_websocket(&stream_id, request, events_tx, stop_receiver).await
        }
    };
    let session = match tokio::time::timeout(timeout, connection).await {
        Ok(Ok(session)) => session,
        Ok(Err(error)) => {
            return stream_execution_failure(&error, &started_at, started.elapsed(), Vec::new(), 0);
        }
        Err(_) => {
            return stream_execution_failure(
                &format!("stream connection timed out after {stream_timeout} ms"),
                &started_at,
                started.elapsed(),
                Vec::new(),
                0,
            );
        }
    };

    let max_entries = request
        .pointer("/stream/maxLogEntries")
        .and_then(Value::as_u64)
        .filter(|value| *value > 0)
        .unwrap_or(1_000)
        .min(usize::MAX as u64) as usize;
    let max_bytes = request
        .pointer("/stream/maxLogBytes")
        .and_then(Value::as_i64)
        .filter(|value| *value > 0)
        .unwrap_or(5 * 1024 * 1024);
    let max_wait_messages = request
        .pointer("/stream/maxWaitMessages")
        .and_then(Value::as_u64)
        .filter(|value| *value > 0)
        .unwrap_or(0) as usize;
    let StreamSession {
        outgoing,
        mut finished,
        ..
    } = session;

    if let Some(sender) = outgoing
        && let Some(messages) = request
            .pointer("/stream/outgoingMessages")
            .and_then(Value::as_array)
    {
        for item in messages {
            let payload = item
                .get("payload")
                .and_then(Value::as_str)
                .unwrap_or("")
                .to_owned();
            let kind = item
                .get("type")
                .and_then(Value::as_str)
                .filter(|kind| !kind.is_empty())
                .unwrap_or("text")
                .to_owned();
            let message = match kind.as_str() {
                "text" => Message::Text(payload.clone().into()),
                "binary" => Message::Binary(
                    base64::engine::general_purpose::STANDARD
                        .decode(payload.as_bytes())
                        .unwrap_or_else(|_| payload.as_bytes().to_vec())
                        .into(),
                ),
                "ping" => Message::Ping(payload.as_bytes().to_vec().into()),
                "pong" => Message::Pong(payload.as_bytes().to_vec().into()),
                _ => continue,
            };
            let _ = sender
                .send(OutgoingMessage {
                    message,
                    payload,
                    kind,
                })
                .await;
        }
    }

    let mut logs = std::collections::VecDeque::new();
    let mut current_bytes = 0_i64;
    let mut evicted = 0_usize;
    let mut received = 0_usize;
    let mut last_payload = String::new();
    let mut close_code = 0_i64;
    let mut close_reason = String::new();
    let mut finished_early = false;
    let remaining = timeout.saturating_sub(started.elapsed());
    let deadline = tokio::time::sleep(remaining);
    tokio::pin!(deadline);
    loop {
        tokio::select! {
            _ = &mut deadline => break,
            event = events_rx.recv() => {
                let Some((event, data)) = event else { break; };
                let terminal = collect_execution_event(&event, &data, &mut logs, &mut current_bytes, max_entries, max_bytes, &mut evicted, &mut received, &mut last_payload, &mut close_code, &mut close_reason);
                if terminal || (max_wait_messages > 0 && received >= max_wait_messages) { break; }
            }
            _ = &mut finished => {
                finished_early = true;
                break;
            }
        }
    }

    if !finished_early {
        let _ = stop_sender.send(());
        let shutdown = tokio::time::sleep(Duration::from_millis(500));
        tokio::pin!(shutdown);
        tokio::select! {
            _ = &mut finished => {},
            _ = &mut shutdown => {},
        }
    }
    while let Ok((event, data)) = events_rx.try_recv() {
        let _ = collect_execution_event(
            &event,
            &data,
            &mut logs,
            &mut current_bytes,
            max_entries,
            max_bytes,
            &mut evicted,
            &mut received,
            &mut last_payload,
            &mut close_code,
            &mut close_reason,
        );
    }

    json!({
        "statusCode": 200,
        "statusText": "OK",
        "headers": {},
        "body": last_payload,
        "size": last_payload.len(),
        "timing": { "dnsLookupMs": 0, "tcpConnectMs": 0, "tlsHandshakeMs": 0, "ttfbMs": 0, "downloadMs": 0, "totalDurationMs": started.elapsed().as_secs_f64() * 1000.0 },
        "streamLogs": logs,
        "streamCloseCode": close_code,
        "streamCloseReason": close_reason,
        "streamEvicted": evicted,
        "tests": [],
        "logs": [],
        "executedAt": started_at,
    })
}

#[allow(clippy::too_many_arguments)]
fn collect_execution_event(
    event: &str,
    data: &str,
    logs: &mut std::collections::VecDeque<Value>,
    current_bytes: &mut i64,
    max_entries: usize,
    max_bytes: i64,
    evicted: &mut usize,
    received: &mut usize,
    last_payload: &mut String,
    close_code: &mut i64,
    close_reason: &mut String,
) -> bool {
    let Ok(value) = serde_json::from_str::<Value>(data) else {
        return false;
    };
    if event == "log" {
        let log_type = value.get("type").and_then(Value::as_str).unwrap_or("");
        let terminal = value
            .get("isError")
            .and_then(Value::as_bool)
            .unwrap_or(false)
            || log_type == "close";
        if value.get("direction").and_then(Value::as_str) == Some("receive") {
            *received += 1;
            *last_payload = value
                .get("payload")
                .and_then(Value::as_str)
                .unwrap_or("")
                .to_owned();
        }
        let size = value
            .get("size")
            .and_then(Value::as_i64)
            .filter(|size| *size > 0)
            .unwrap_or_else(|| {
                value
                    .get("payload")
                    .and_then(Value::as_str)
                    .unwrap_or("")
                    .len() as i64
            });
        *current_bytes += size;
        logs.push_back(value);
        while logs.len() > max_entries || (*current_bytes > max_bytes && logs.len() > 1) {
            if let Some(removed) = logs.pop_front() {
                let removed_size = removed
                    .get("size")
                    .and_then(Value::as_i64)
                    .filter(|size| *size > 0)
                    .unwrap_or_else(|| {
                        removed
                            .get("payload")
                            .and_then(Value::as_str)
                            .unwrap_or("")
                            .len() as i64
                    });
                *current_bytes = (*current_bytes - removed_size).max(0);
                *evicted += 1;
            }
        }
        terminal
    } else if event == "status"
        && value.get("state").and_then(Value::as_str) == Some("disconnected")
    {
        *close_code = value.get("closeCode").and_then(Value::as_i64).unwrap_or(0);
        *close_reason = value
            .get("closeReason")
            .and_then(Value::as_str)
            .unwrap_or("")
            .to_owned();
        false
    } else {
        false
    }
}

fn stream_execution_failure(
    message: &str,
    executed_at: &str,
    elapsed: Duration,
    logs: Vec<Value>,
    evicted: usize,
) -> Value {
    json!({
        "statusCode": 0,
        "statusText": "Connection Failed",
        "headers": {},
        "body": "",
        "size": 0,
        "timing": { "dnsLookupMs": 0, "tcpConnectMs": 0, "tlsHandshakeMs": 0, "ttfbMs": 0, "downloadMs": 0, "totalDurationMs": elapsed.as_secs_f64() * 1000.0 },
        "streamLogs": logs,
        "streamEvicted": evicted,
        "tests": [],
        "logs": [],
        "executedAt": executed_at,
        "error": message,
    })
}

pub(super) async fn send(
    State(state): State<Arc<AppState>>,
    Json(payload): Json<SendPayload>,
) -> Response {
    let (sender, connected) = {
        let sessions = state
            .stream_manager
            .sessions
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let Some(session) = sessions.get(&payload.stream_id) else {
            return api_error(
                StatusCode::NOT_FOUND,
                "WebSocket session not found or is receive-only",
            );
        };
        (session.outgoing.clone(), session.connected.clone())
    };
    let Some(sender) = sender else {
        return api_error(
            StatusCode::NOT_FOUND,
            "WebSocket session not found or is receive-only",
        );
    };
    if connected
        .as_ref()
        .is_some_and(|connected| !connected.load(Ordering::Acquire))
    {
        return api_error(StatusCode::CONFLICT, "WebSocket session is disconnected");
    }
    let kind = match payload.message_type.as_str() {
        "binary" | "ping" | "pong" => payload.message_type.as_str(),
        _ => "text",
    }
    .to_owned();
    let message = match kind.as_str() {
        "text" => Message::Text(payload.payload.clone().into()),
        "binary" => {
            let bytes = base64::engine::general_purpose::STANDARD
                .decode(payload.payload.as_bytes())
                .unwrap_or_else(|_| payload.payload.as_bytes().to_vec());
            Message::Binary(bytes.into())
        }
        "ping" => Message::Ping(payload.payload.as_bytes().to_vec().into()),
        "pong" => Message::Pong(payload.payload.as_bytes().to_vec().into()),
        _ => unreachable!("message kind was normalized above"),
    };
    let outgoing = OutgoingMessage {
        message,
        payload: payload.payload,
        kind,
    };
    match sender.send(outgoing).await {
        Ok(()) => Json(json!({ "success": true })).into_response(),
        Err(_) => api_error(StatusCode::CONFLICT, "WebSocket session is disconnected"),
    }
}

pub(super) async fn disconnect(
    State(state): State<Arc<AppState>>,
    Json(payload): Json<DisconnectPayload>,
) -> Response {
    let session = state
        .stream_manager
        .sessions
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner)
        .remove(&payload.stream_id);
    match session {
        Some(session) => {
            let protocol = session.protocol;
            let _ = session.stop.send(());
            Json(json!({ "success": true, "protocol": protocol })).into_response()
        }
        None => (
            StatusCode::NOT_FOUND,
            Json(json!({ "error": "stream session not found" })),
        )
            .into_response(),
    }
}

fn resolve_stream_request(payload: &ConnectPayload) -> Result<Value, String> {
    let mut request = payload.request.clone();
    if let (Some(workspace), Some(path)) =
        (payload.workspace_path.as_deref(), payload.path.as_deref())
        && !path.is_empty()
        && let Ok(resolved) = resolve_request(
            std::path::Path::new(workspace),
            std::path::Path::new(path),
            payload.environment_name.as_deref(),
        )
        && let Some(resolved_request) = resolved.get("request")
    {
        request = resolved_request.clone();
    }
    let environments = payload
        .workspace_path
        .as_deref()
        .and_then(|workspace| list_environments(std::path::Path::new(workspace)).ok());
    let environment = payload.environment_name.as_deref().and_then(|name| {
        environments
            .as_ref()?
            .iter()
            .find(|item| item.name.eq_ignore_ascii_case(name))
    });
    let variables = build_variable_map(environment, &HashMap::new());
    interpolate_value(&mut request, &variables)?;
    Ok(request)
}

fn stream_url(request: &Value) -> Result<reqwest::Url, String> {
    let raw = request.get("url").and_then(Value::as_str).unwrap_or("");
    let mut url =
        reqwest::Url::parse(raw).map_err(|error| format!("invalid stream URL: {error}"))?;
    if let Some(params) = request.get("params").and_then(Value::as_array) {
        url.query_pairs_mut()
            .extend_pairs(params.iter().filter_map(|param| {
                if !param
                    .get("enabled")
                    .and_then(Value::as_bool)
                    .unwrap_or(false)
                {
                    return None;
                }
                let key = param.get("key")?.as_str()?;
                let value = param.get("value").and_then(Value::as_str).unwrap_or("");
                Some((key.to_owned(), value.to_owned()))
            }));
    }
    let auth = request.get("auth").cloned().unwrap_or(Value::Null);
    if auth
        .get("type")
        .and_then(Value::as_str)
        .is_some_and(|kind| kind.eq_ignore_ascii_case("apiKey"))
        && auth.get("addTo").and_then(Value::as_str) == Some("query")
        && let Some(key) = auth
            .get("key")
            .and_then(Value::as_str)
            .filter(|key| !key.is_empty())
    {
        url.query_pairs_mut()
            .append_pair(key, auth.get("value").and_then(Value::as_str).unwrap_or(""));
    }
    Ok(url)
}

async fn append_request_headers(
    request: &Value,
    headers: &mut axum::http::HeaderMap,
) -> Result<(), String> {
    if let Some(items) = request.get("headers").and_then(Value::as_array) {
        for item in items.iter().filter(|item| {
            item.get("enabled")
                .and_then(Value::as_bool)
                .unwrap_or(false)
        }) {
            let key = item.get("key").and_then(Value::as_str).unwrap_or("");
            let value = item.get("value").and_then(Value::as_str).unwrap_or("");
            if key.is_empty() {
                continue;
            }
            let name = HeaderName::from_bytes(key.as_bytes())
                .map_err(|error| format!("invalid header name: {error}"))?;
            let value = HeaderValue::from_bytes(value.as_bytes())
                .map_err(|error| format!("invalid value for header {key}: {error}"))?;
            headers.append(name, value);
        }
    }
    let auth = request.get("auth").cloned().unwrap_or(Value::Null);
    let auth_type = auth.get("type").and_then(Value::as_str).unwrap_or("none");
    let oauth2_token = if auth_type.eq_ignore_ascii_case("oauth2") {
        crate::oauth2_api::get_token(&auth)
            .await
            .ok()
            .and_then(|token| {
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
        "bearer" | "oauth2" => {
            let token = oauth2_token.as_deref().or_else(|| {
                auth.get("token")
                    .and_then(Value::as_str)
                    .filter(|token| !token.trim().is_empty())
            });
            if let Some(token) = token {
                let value = if token.trim().to_ascii_lowercase().starts_with("bearer ") {
                    token.trim().to_owned()
                } else {
                    format!("Bearer {}", token.trim())
                };
                headers.insert(
                    "authorization",
                    HeaderValue::from_bytes(value.as_bytes()).map_err(|error| error.to_string())?,
                );
            }
        }
        "basic" => {
            let user = auth.get("username").and_then(Value::as_str).unwrap_or("");
            let password = auth.get("password").and_then(Value::as_str).unwrap_or("");
            let value =
                base64::engine::general_purpose::STANDARD.encode(format!("{user}:{password}"));
            headers.insert(
                "authorization",
                HeaderValue::from_bytes(format!("Basic {value}").as_bytes())
                    .map_err(|error| error.to_string())?,
            );
        }
        "apikey" if auth.get("addTo").and_then(Value::as_str) != Some("query") => {
            let key = auth.get("key").and_then(Value::as_str).unwrap_or("");
            let value = auth.get("value").and_then(Value::as_str).unwrap_or("");
            if !key.is_empty() {
                headers.insert(
                    HeaderName::from_bytes(key.as_bytes()).map_err(|error| error.to_string())?,
                    HeaderValue::from_bytes(value.as_bytes()).map_err(|error| error.to_string())?,
                );
            }
        }
        _ => {}
    }
    Ok(())
}

fn find_sse_boundary(bytes: &[u8]) -> Option<(usize, usize)> {
    bytes
        .windows(2)
        .position(|window| window == b"\n\n")
        .map(|index| (index, 2))
        .or_else(|| {
            bytes
                .windows(4)
                .position(|window| window == b"\r\n\r\n")
                .map(|index| (index, 4))
        })
}

fn sse_request(
    client: &reqwest::Client,
    url: &reqwest::Url,
    headers: &reqwest::header::HeaderMap,
    last_event_id: Option<&str>,
) -> reqwest::RequestBuilder {
    let mut request = client.get(url.clone());
    for (name, value) in headers {
        request = request.header(name.clone(), value.clone());
    }
    if !headers.contains_key("accept") {
        request = request.header(reqwest::header::ACCEPT, "text/event-stream");
    }
    request = request.header(reqwest::header::CACHE_CONTROL, "no-cache");
    if let Some(last_event_id) = last_event_id {
        request = request.header("last-event-id", last_event_id);
    }
    request
}

fn parse_sse_event(block: &[u8]) -> Option<(String, String, Option<String>)> {
    let mut event_type = String::new();
    let mut event_id = None;
    let mut data_lines = Vec::new();
    let binding = String::from_utf8_lossy(block);
    for line in binding.lines() {
        let line = line.trim_end_matches('\r');
        if line.starts_with(':') {
            continue;
        }
        let (field, raw_value) = line.split_once(':').unwrap_or((line, ""));
        let value = raw_value.strip_prefix(' ').unwrap_or(raw_value);
        match field {
            "event" => event_type = value.to_owned(),
            "data" => data_lines.push(value),
            "id" if !value.contains('\0') => event_id = Some(value.to_owned()),
            _ => {}
        }
    }
    if data_lines.is_empty() {
        return None;
    }
    if event_type.is_empty() {
        event_type = "message".into();
    }
    Some((event_type, data_lines.join("\n"), event_id))
}

async fn emit_log(
    events: &mpsc::Sender<(String, String)>,
    index: &mut usize,
    direction: &str,
    kind: &str,
    payload: &str,
    is_error: bool,
) {
    *index += 1;
    let id = format!("log_{}_{}", chrono::Utc::now().timestamp_micros(), *index);
    let log = json!({
        "id": id,
        "index": *index,
        "direction": direction,
        "type": kind,
        "timestamp": chrono::Utc::now().to_rfc3339(),
        "payload": payload,
        "size": payload.len(),
        "isError": is_error,
    });
    let _ = events.send(("log".into(), log.to_string())).await;
}

#[allow(clippy::too_many_arguments)]
async fn emit_status(
    events: &mpsc::Sender<(String, String)>,
    snapshot: &Arc<Mutex<Value>>,
    stream_id: &str,
    url: &str,
    protocol: &str,
    state: &str,
    total_sent: u64,
    total_received: u64,
    reconnect_count: u64,
    subprotocol: &str,
    close: Option<(u16, String)>,
) {
    let mut status = json!({
        "streamId": stream_id,
        "protocol": protocol,
        "state": state,
        "url": url,
        "reconnectCount": reconnect_count,
        "totalSent": total_sent,
        "totalReceived": total_received,
        "evictedCount": 0
    });
    if !subprotocol.is_empty() {
        status["subprotocol"] = json!(subprotocol);
    }
    if let Some((code, reason)) = close {
        if code > 0 {
            status["closeCode"] = json!(code);
        }
        if !reason.is_empty() {
            status["closeReason"] = json!(reason);
        }
    }
    *snapshot
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner) = status.clone();
    let _ = events.send(("status".into(), status.to_string())).await;
}

fn api_error(status: StatusCode, message: &str) -> Response {
    (status, Json(json!({ "error": message }))).into_response()
}

#[cfg(test)]
mod parity_tests {
    use super::*;

    #[test]
    fn graphql_detection_and_messages_cover_subscription_inputs() {
        assert!(is_graphql_ws(&json!({"protocol":"graphql-ws"})));
        assert!(is_graphql_ws(
            &json!({"body":{"graphql":{"query":"  subscription { updates }"}}})
        ));
        assert!(is_graphql_ws(
            &json!({"stream":{"subprotocols":["graphql-transport-ws"]}})
        ));
        assert!(!is_graphql_ws(
            &json!({"protocol":"websocket","body":{"graphql":{"query":"query { status }"}}})
        ));

        assert_eq!(
            graphql_connection_init(&json!({})),
            r#"{"type":"connection_init"}"#
        );
        let init: Value =
            serde_json::from_str(&graphql_connection_init(&json!({"Authorization":"token"})))
                .unwrap();
        assert_eq!(init["type"], "connection_init");
        assert_eq!(init["payload"]["Authorization"], "token");
        let subscribe: Value = serde_json::from_str(&graphql_subscribe_message(
            "7",
            "subscription { events }",
            &json!({"room":"a"}),
            "Events",
        ))
        .unwrap();
        assert_eq!(subscribe["id"], "7");
        assert_eq!(subscribe["type"], "subscribe");
        assert_eq!(subscribe["payload"]["variables"]["room"], "a");
        assert_eq!(subscribe["payload"]["operationName"], "Events");
        assert_eq!(subscribe["payload"]["query"], "subscription { events }");

        let (message, payload, kind) = graphql_outgoing_message(r#"{"type":"ping"}"#);
        assert!(matches!(message, Message::Text(_)));
        assert_eq!(
            serde_json::from_str::<Value>(&payload).unwrap()["type"],
            "ping"
        );
        assert_eq!(kind, "text");
        let (_, payload, _) = graphql_outgoing_message("subscription { events }");
        let parsed: Value = serde_json::from_str(&payload).unwrap();
        assert_eq!(parsed["type"], "subscribe");
        assert_eq!(parsed["payload"]["query"], "subscription { events }");
    }

    #[test]
    fn sse_parser_handles_multiline_crlf_comments_and_invalid_ids() {
        assert_eq!(
            find_sse_boundary(b"event: update\r\ndata: a\r\n\r\nrest"),
            Some((22, 4))
        );
        assert_eq!(
            parse_sse_event(
                b": heartbeat\r\nevent: update\r\nid: evt-1\r\ndata: first\r\ndata: second\r\n"
            ),
            Some((
                "update".into(),
                "first\nsecond".into(),
                Some("evt-1".into())
            ))
        );
        assert_eq!(
            parse_sse_event(b"data: hello\n\n"),
            Some(("message".into(), "hello".into(), None))
        );
        assert_eq!(
            parse_sse_event(b"id: invalid\0id\ndata: safe\n"),
            Some(("message".into(), "safe".into(), None))
        );
        assert_eq!(parse_sse_event(b": keepalive\nretry: 1000\n"), None);
    }

    #[test]
    fn sse_reconnect_request_restores_last_event_id_and_stream_headers() {
        let client = reqwest::Client::new();
        let url = reqwest::Url::parse("https://events.example.test/stream").unwrap();
        let mut headers = reqwest::header::HeaderMap::new();
        headers.insert("x-client", HeaderValue::from_static("pebblepost"));
        let request = sse_request(&client, &url, &headers, Some("evt-19"))
            .build()
            .unwrap();
        assert_eq!(request.method(), reqwest::Method::GET);
        assert_eq!(
            request.headers()[reqwest::header::ACCEPT],
            "text/event-stream"
        );
        assert_eq!(
            request.headers()[reqwest::header::CACHE_CONTROL],
            "no-cache"
        );
        assert_eq!(request.headers()["last-event-id"], "evt-19");
        assert_eq!(request.headers()["x-client"], "pebblepost");

        let without_cursor = sse_request(&client, &url, &reqwest::header::HeaderMap::new(), None)
            .build()
            .unwrap();
        assert!(!without_cursor.headers().contains_key("last-event-id"));
    }

    #[test]
    fn close_codes_have_stable_descriptions_and_unknown_codes_are_preserved() {
        assert_eq!(close_code_description(1000), "Normal Closure (1000)");
        assert_eq!(close_code_description(1006), "Abnormal Closure (1006)");
        assert_eq!(close_code_description(4321), "Close Code 4321");
        assert_eq!(close_code_description(0), "");
    }
}
