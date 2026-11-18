use std::collections::{BTreeMap, HashMap};
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};

use axum::Json;
use axum::extract::State;
use axum::http::StatusCode;
use axum::response::sse::{Event, KeepAlive};
use axum::response::{IntoResponse, Response, Sse};
use futures_util::StreamExt;
use pebblepost_core::{
    build_variable_map, interpolate_value, list_environments, resolve_request, safe_workspace_path,
};
use prost::Message;
use prost_reflect::{
    DescriptorPool, DynamicMessage, EnumDescriptor, Kind, MessageDescriptor, MethodDescriptor,
    ServiceDescriptor,
};
use prost_types::{FileDescriptorProto, FileDescriptorSet};
use rustls::client::danger::{HandshakeSignatureValid, ServerCertVerified, ServerCertVerifier};
use rustls::pki_types::{CertificateDer, ServerName, UnixTime};
use rustls::{DigitallySignedStruct, SignatureScheme};
use serde::Deserialize;
use serde_json::{Value, json};
use tokio::sync::{mpsc, oneshot};
use tokio_stream::wrappers::UnboundedReceiverStream;
use tonic::codec::{Codec, DecodeBuf, Decoder, EncodeBuf, Encoder};
use tonic::metadata::{Ascii, Binary, KeyAndValueRef, MetadataKey, MetadataMap, MetadataValue};
use tonic::transport::{Certificate, Channel, ClientTlsConfig, Endpoint};
use tonic::{Code, Request, Status};
use tonic_reflection::pb;

#[derive(Default)]
pub struct GrpcManager {
    active: Mutex<HashMap<String, (u64, oneshot::Sender<()>)>>,
    next_id: AtomicU64,
}

impl GrpcManager {
    fn insert(&self, stream_id: String, cancel: oneshot::Sender<()>) -> u64 {
        let generation = self.next_id.fetch_add(1, Ordering::Relaxed);
        if let Ok(mut active) = self.active.lock()
            && let Some((_, old)) = active.insert(stream_id, (generation, cancel))
        {
            let _ = old.send(());
        }
        generation
    }

    fn remove(&self, stream_id: &str, generation: u64) {
        if let Ok(mut active) = self.active.lock()
            && active
                .get(stream_id)
                .is_some_and(|(current, _)| *current == generation)
        {
            active.remove(stream_id);
        }
    }

    fn cancel(&self, stream_id: &str) -> bool {
        self.active
            .lock()
            .ok()
            .and_then(|mut active| active.remove(stream_id))
            .is_some_and(|(_, sender)| sender.send(()).is_ok())
    }
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct ReflectPayload {
    pub(crate) address: String,
    #[serde(default)]
    pub(crate) use_tls: bool,
    #[serde(default)]
    pub(crate) insecure_skip_verify: bool,
    #[serde(default)]
    pub(crate) root_ca_path: String,
    #[serde(default)]
    pub(crate) workspace_path: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct ProtoPayload {
    pub(crate) proto_files: Vec<String>,
    #[serde(default)]
    pub(crate) import_paths: Vec<String>,
    #[serde(default)]
    pub(crate) workspace_path: String,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct SamplePayload {
    pub(crate) workspace_path: Option<String>,
    pub(crate) grpc: Option<Value>,
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
pub(crate) struct StreamPayload {
    pub(crate) stream_id: Option<String>,
    pub(crate) workspace_path: Option<String>,
    pub(crate) environment_name: Option<String>,
    pub(crate) path: Option<String>,
    pub(crate) request: Option<Value>,
    pub(crate) trusted: Option<bool>,
    #[serde(default)]
    pub(crate) overrides: HashMap<String, String>,
}

#[derive(Clone)]
struct DynamicCodec {
    output: MessageDescriptor,
}

struct DynamicEncoder;

struct DynamicDecoder {
    descriptor: MessageDescriptor,
}

impl Codec for DynamicCodec {
    type Encode = DynamicMessage;
    type Decode = DynamicMessage;
    type Encoder = DynamicEncoder;
    type Decoder = DynamicDecoder;

    fn encoder(&mut self) -> Self::Encoder {
        DynamicEncoder
    }
    fn decoder(&mut self) -> Self::Decoder {
        DynamicDecoder {
            descriptor: self.output.clone(),
        }
    }
}

impl Encoder for DynamicEncoder {
    type Item = DynamicMessage;
    type Error = Status;

    fn encode(&mut self, item: Self::Item, dst: &mut EncodeBuf<'_>) -> Result<(), Self::Error> {
        item.encode(dst)
            .map_err(|error| Status::internal(error.to_string()))
    }
}

impl Decoder for DynamicDecoder {
    type Item = DynamicMessage;
    type Error = Status;

    fn decode(&mut self, src: &mut DecodeBuf<'_>) -> Result<Option<Self::Item>, Self::Error> {
        DynamicMessage::decode(self.descriptor.clone(), src)
            .map(Some)
            .map_err(|error| Status::internal(error.to_string()))
    }
}

#[derive(Debug)]
struct SkipServerVerification {
    algorithms: rustls::crypto::WebPkiSupportedAlgorithms,
}

impl ServerCertVerifier for SkipServerVerification {
    fn verify_server_cert(
        &self,
        _end_entity: &CertificateDer<'_>,
        _intermediates: &[CertificateDer<'_>],
        _server_name: &ServerName<'_>,
        _ocsp_response: &[u8],
        _now: UnixTime,
    ) -> Result<ServerCertVerified, rustls::Error> {
        Ok(ServerCertVerified::assertion())
    }

    fn verify_tls12_signature(
        &self,
        message: &[u8],
        cert: &CertificateDer<'_>,
        dss: &DigitallySignedStruct,
    ) -> Result<HandshakeSignatureValid, rustls::Error> {
        rustls::crypto::verify_tls12_signature(message, cert, dss, &self.algorithms)
    }

    fn verify_tls13_signature(
        &self,
        message: &[u8],
        cert: &CertificateDer<'_>,
        dss: &DigitallySignedStruct,
    ) -> Result<HandshakeSignatureValid, rustls::Error> {
        rustls::crypto::verify_tls13_signature(message, cert, dss, &self.algorithms)
    }

    fn supported_verify_schemes(&self) -> Vec<SignatureScheme> {
        self.algorithms.supported_schemes()
    }
}

pub async fn reflect_services(Json(payload): Json<ReflectPayload>) -> Response {
    if payload.address.trim().is_empty() {
        return api_error(StatusCode::BAD_REQUEST, "address is required");
    }
    let workspace = nonempty_path(&payload.workspace_path);
    let channel = match connect(
        &payload.address,
        payload.use_tls,
        payload.insecure_skip_verify,
        workspace,
        &payload.root_ca_path,
    )
    .await
    {
        Ok(channel) => channel,
        Err(error) => return api_error(StatusCode::BAD_GATEWAY, &error),
    };
    match reflect_pool(channel).await {
        Ok(pool) => Json(services_json(&pool)).into_response(),
        Err(error) => api_error(StatusCode::BAD_GATEWAY, &error),
    }
}

pub async fn proto_services(Json(payload): Json<ProtoPayload>) -> Response {
    match load_proto_pool(
        &payload.workspace_path,
        &payload.proto_files,
        &payload.import_paths,
    ) {
        Ok(pool) => Json(services_json(&pool)).into_response(),
        Err(error) => api_error(StatusCode::BAD_REQUEST, &error),
    }
}

pub async fn sample_message(Json(payload): Json<SamplePayload>) -> Response {
    let Some(grpc) = payload.grpc else {
        return api_error(StatusCode::BAD_REQUEST, "grpc definition is required");
    };
    match resolve_method(&grpc, payload.workspace_path.as_deref()).await {
        Ok(method) => match serde_json::to_string_pretty(&sample_message_value(
            &method.input(),
            &mut Vec::new(),
        )) {
            Ok(sample) => Json(json!({ "sample": sample })).into_response(),
            Err(error) => api_error(StatusCode::INTERNAL_SERVER_ERROR, &error.to_string()),
        },
        Err(error) => api_error(StatusCode::BAD_REQUEST, &error),
    }
}

pub async fn cancel_stream(
    State(state): State<Arc<super::AppState>>,
    Json(payload): Json<Value>,
) -> Json<Value> {
    let stream_id = payload
        .get("streamId")
        .and_then(Value::as_str)
        .unwrap_or_default();
    Json(json!({ "cancelled": state.grpc_manager.cancel(stream_id) }))
}

pub async fn stream(
    State(state): State<Arc<super::AppState>>,
    Json(payload): Json<StreamPayload>,
) -> Response {
    let Some(mut request) = payload.request else {
        return api_error(StatusCode::BAD_REQUEST, "request is required");
    };
    let Some(grpc) = request.get("grpc").cloned() else {
        return api_error(
            StatusCode::BAD_REQUEST,
            "Request definition with grpc is required",
        );
    };
    let stream_id = payload
        .stream_id
        .filter(|id| !id.is_empty())
        .unwrap_or_else(|| format!("grpc-{}", chrono::Utc::now().timestamp_micros()));

    if let (Some(workspace), Some(path)) =
        (payload.workspace_path.as_deref(), payload.path.as_deref())
        && !path.is_empty()
        && let Ok(resolved) = resolve_request(
            Path::new(workspace),
            Path::new(path),
            payload.environment_name.as_deref(),
        )
        && let Some(resolved_request) = resolved.get("request")
    {
        request = resolved_request.clone();
    }
    let variables = payload
        .workspace_path
        .as_deref()
        .and_then(|workspace| list_environments(Path::new(workspace)).ok())
        .and_then(|environments| {
            payload
                .environment_name
                .as_deref()
                .and_then(|name| {
                    environments
                        .iter()
                        .find(|environment| environment.name.eq_ignore_ascii_case(name))
                })
                .map(|environment| build_variable_map(Some(environment), &payload.overrides))
        })
        .unwrap_or_else(|| build_variable_map(None, &payload.overrides));
    let _trusted = payload.trusted.unwrap_or(true);
    if let Err(error) = interpolate_value(&mut request, &variables) {
        return api_error(StatusCode::BAD_REQUEST, &error.to_string());
    }
    let grpc = request.get("grpc").cloned().unwrap_or(grpc);
    let timeout_ms = request
        .pointer("/settings/timeoutMs")
        .and_then(Value::as_u64)
        .filter(|ms| *ms > 0)
        .unwrap_or(30_000);
    let request_for_extractors = request.clone();

    let (cancel_tx, cancel_rx) = oneshot::channel();
    let generation = state.grpc_manager.insert(stream_id.clone(), cancel_tx);
    let manager = state.grpc_manager.clone();
    let (events_tx, events_rx) = mpsc::unbounded_channel::<(String, Value)>();
    let execution_tx = events_tx.clone();
    let workspace = payload.workspace_path.clone();
    let task_stream_id = stream_id.clone();
    tokio::spawn(async move {
        let mut result = tokio::select! {
            _ = cancel_rx => cancelled_result(),
            _ = events_tx.closed() => cancelled_result(),
            result = execute(&grpc, workspace.as_deref(), timeout_ms, move |message| {
                let _ = execution_tx.send(("message".into(), message));
            }) => result,
        };
        super::apply_extractors(&request_for_extractors, &mut result);
        let _ = events_tx.send(("done".into(), result));
        manager.remove(&task_stream_id, generation);
    });

    let stream = UnboundedReceiverStream::new(events_rx).map(|(event, data)| {
        Ok::<Event, std::convert::Infallible>(Event::default().event(event).data(data.to_string()))
    });
    Sse::new(stream)
        .keep_alive(KeepAlive::default())
        .into_response()
}

fn nonempty_path(path: &str) -> Option<&Path> {
    (!path.trim().is_empty()).then(|| Path::new(path))
}

fn api_error(status: StatusCode, message: &str) -> Response {
    (status, Json(json!({ "error": message }))).into_response()
}

fn workspace_root(path: Option<&Path>) -> Result<PathBuf, String> {
    let path = path.ok_or_else(|| "workspacePath is required".to_owned())?;
    path.canonicalize()
        .map_err(|error| format!("invalid workspace path: {error}"))
}

fn load_proto_pool(
    workspace: &str,
    proto_files: &[String],
    import_paths: &[String],
) -> Result<DescriptorPool, String> {
    if proto_files.is_empty() {
        return Err("protoFiles are required".into());
    }
    let root = workspace_root(nonempty_path(workspace))?;
    let mut includes = vec![root.clone()];
    let mut files = Vec::new();
    for import in import_paths {
        let path =
            safe_workspace_path(&root, Path::new(import)).map_err(|error| error.to_string())?;
        if !path.is_dir() {
            return Err(format!(
                "import path is not a directory: {}",
                path.display()
            ));
        }
        includes.push(path);
    }
    for file in proto_files {
        let path =
            safe_workspace_path(&root, Path::new(file)).map_err(|error| error.to_string())?;
        if path.extension().and_then(|extension| extension.to_str()) != Some("proto") {
            return Err(format!("expected a .proto file: {}", path.display()));
        }
        if let Some(parent) = path.parent() {
            includes.push(parent.to_path_buf());
        }
        files.push(path);
    }
    includes.sort();
    includes.dedup();
    let descriptors = protox::compile(files, includes)
        .map_err(|error| format!("failed to parse proto files: {error}"))?;
    DescriptorPool::from_file_descriptor_set(descriptors)
        .map_err(|error| format!("invalid protobuf descriptors: {error}"))
}

fn services_json(pool: &DescriptorPool) -> Value {
    let mut services = pool
        .services()
        .map(|service| {
            let mut methods: Vec<Value> = service
                .methods()
                .map(|method| method_json(&service, &method))
                .collect();
            methods.sort_by(|left, right| left["name"].as_str().cmp(&right["name"].as_str()));
            json!({ "name": service.full_name(), "methods": methods })
        })
        .collect::<Vec<_>>();
    services.sort_by(|left, right| left["name"].as_str().cmp(&right["name"].as_str()));
    Value::Array(services)
}

fn method_json(service: &ServiceDescriptor, method: &MethodDescriptor) -> Value {
    json!({
        "name": method.name(),
        "fullMethod": format!("/{}/{}", service.full_name(), method.name()),
        "clientStreaming": method.is_client_streaming(),
        "serverStreaming": method.is_server_streaming(),
        "inputType": method.input().full_name(),
        "outputType": method.output().full_name(),
    })
}

fn find_method(pool: DescriptorPool, grpc: &Value) -> Result<MethodDescriptor, String> {
    let service_name = grpc
        .get("service")
        .and_then(Value::as_str)
        .unwrap_or_default();
    let method_name = grpc
        .get("method")
        .and_then(Value::as_str)
        .unwrap_or_default();
    if service_name.is_empty() || method_name.is_empty() {
        return Err("gRPC service and method are required".into());
    }
    let service = pool
        .services()
        .find(|service| service.full_name() == service_name || service.name() == service_name)
        .ok_or_else(|| format!("service {service_name:?} not found"))?;
    service
        .methods()
        .find(|method| {
            method.name() == method_name
                || format!("/{}/{}", service.full_name(), method.name()) == method_name
        })
        .ok_or_else(|| format!("method {method_name:?} not found in service {service_name:?}"))
}

async fn resolve_method(grpc: &Value, workspace: Option<&str>) -> Result<MethodDescriptor, String> {
    let pool = if grpc.get("protoSource").and_then(Value::as_str) == Some("file") {
        let files = grpc
            .get("protoFiles")
            .and_then(Value::as_array)
            .map(|items| {
                items
                    .iter()
                    .filter_map(Value::as_str)
                    .map(str::to_owned)
                    .collect::<Vec<_>>()
            })
            .unwrap_or_default();
        let imports = grpc
            .get("importPaths")
            .and_then(Value::as_array)
            .map(|items| {
                items
                    .iter()
                    .filter_map(Value::as_str)
                    .map(str::to_owned)
                    .collect::<Vec<_>>()
            })
            .unwrap_or_default();
        load_proto_pool(workspace.unwrap_or_default(), &files, &imports)?
    } else {
        let address = grpc
            .get("address")
            .and_then(Value::as_str)
            .unwrap_or_default();
        if address.trim().is_empty() {
            return Err("gRPC address is required for reflection".into());
        }
        let tls = grpc.get("useTls").and_then(Value::as_bool).unwrap_or(false);
        let skip_verify = grpc
            .get("insecureSkipVerify")
            .and_then(Value::as_bool)
            .unwrap_or(false);
        let root_ca = grpc
            .get("rootCaPath")
            .and_then(Value::as_str)
            .unwrap_or_default();
        let channel = connect(address, tls, skip_verify, workspace.map(Path::new), root_ca).await?;
        reflect_pool(channel).await?
    };
    find_method(pool, grpc)
}

async fn connect(
    address: &str,
    use_tls: bool,
    skip_verify: bool,
    workspace: Option<&Path>,
    root_ca: &str,
) -> Result<Channel, String> {
    let authority = address.trim();
    let uri = if authority.starts_with("http://") || authority.starts_with("https://") {
        authority.to_owned()
    } else {
        format!("{}://{}", if use_tls { "https" } else { "http" }, authority)
    };
    let mut endpoint = Endpoint::from_shared(uri.clone())
        .map_err(|error| format!("invalid gRPC address: {error}"))?
        .connect_timeout(Duration::from_secs(10));
    if use_tls {
        let tls = if skip_verify {
            ClientTlsConfig::new()
        } else if root_ca.trim().is_empty() {
            ClientTlsConfig::new().with_webpki_roots()
        } else {
            let root = workspace_root(workspace)?;
            let ca_path = safe_workspace_path(&root, Path::new(root_ca))
                .map_err(|error| error.to_string())?;
            let ca = std::fs::read(&ca_path)
                .map_err(|error| format!("failed to read root CA certificate: {error}"))?;
            ClientTlsConfig::new()
                .with_webpki_roots()
                .ca_certificate(Certificate::from_pem(ca))
        };
        endpoint = if skip_verify {
            let algorithms =
                rustls::crypto::ring::default_provider().signature_verification_algorithms;
            endpoint.tls_config_with_verifier(tls, Arc::new(SkipServerVerification { algorithms }))
        } else {
            endpoint.tls_config(tls)
        }
        .map_err(|error| format!("invalid gRPC TLS configuration: {error}"))?;
    }
    endpoint
        .connect()
        .await
        .map_err(|error| format!("failed to connect to gRPC address {uri:?}: {error}"))
}

async fn reflect_pool(channel: Channel) -> Result<DescriptorPool, String> {
    match reflect_pool_v1(channel.clone()).await {
        Ok(pool) => Ok(pool),
        Err(v1_error) => reflect_pool_v1alpha(channel)
            .await
            .map_err(|v1_error_alpha| {
                format!("gRPC reflection failed (v1: {v1_error}; v1alpha: {v1_error_alpha})")
            }),
    }
}

async fn reflect_pool_v1(channel: Channel) -> Result<DescriptorPool, String> {
    use pb::v1::server_reflection_request::MessageRequest;
    let (request_tx, request_rx) = mpsc::channel(8);
    let mut client = pb::v1::server_reflection_client::ServerReflectionClient::new(channel);
    let mut responses = client
        .server_reflection_info(tokio_stream::wrappers::ReceiverStream::new(request_rx))
        .await
        .map_err(|error| error.to_string())?
        .into_inner();
    request_tx
        .send(pb::v1::ServerReflectionRequest {
            host: String::new(),
            message_request: Some(MessageRequest::ListServices(String::new())),
        })
        .await
        .map_err(|error| error.to_string())?;
    let service_response = responses
        .message()
        .await
        .map_err(|error| error.to_string())?
        .ok_or_else(|| "reflection returned no service list".to_owned())?;
    let services = match service_response.message_response {
        Some(pb::v1::server_reflection_response::MessageResponse::ListServicesResponse(list)) => {
            list.service
        }
        Some(pb::v1::server_reflection_response::MessageResponse::ErrorResponse(error)) => {
            return Err(format!(
                "reflection error {}: {}",
                error.error_code, error.error_message
            ));
        }
        _ => return Err("unexpected response while listing gRPC services".into()),
    };
    let mut descriptors = BTreeMap::<String, FileDescriptorProto>::new();
    for service in services {
        if service.name == "grpc.reflection.v1.ServerReflection"
            || service.name == "grpc.reflection.v1alpha.ServerReflection"
        {
            continue;
        }
        request_tx
            .send(pb::v1::ServerReflectionRequest {
                host: String::new(),
                message_request: Some(MessageRequest::FileContainingSymbol(service.name)),
            })
            .await
            .map_err(|error| error.to_string())?;
        let response = responses
            .message()
            .await
            .map_err(|error| error.to_string())?
            .ok_or_else(|| "reflection returned no descriptor response".to_owned())?;
        collect_v1_descriptors(response, &mut descriptors)?;
    }
    build_reflected_pool(descriptors)
}

fn collect_v1_descriptors(
    response: pb::v1::ServerReflectionResponse,
    descriptors: &mut BTreeMap<String, FileDescriptorProto>,
) -> Result<(), String> {
    match response.message_response {
        Some(pb::v1::server_reflection_response::MessageResponse::FileDescriptorResponse(
            result,
        )) => {
            for bytes in result.file_descriptor_proto {
                let file = FileDescriptorProto::decode(bytes.as_slice())
                    .map_err(|error| error.to_string())?;
                let name = file
                    .name
                    .clone()
                    .unwrap_or_else(|| format!("anonymous-{}", descriptors.len()));
                descriptors.entry(name).or_insert(file);
            }
            Ok(())
        }
        Some(pb::v1::server_reflection_response::MessageResponse::ErrorResponse(error)) => {
            Err(format!(
                "reflection error {}: {}",
                error.error_code, error.error_message
            ))
        }
        _ => Err("unexpected descriptor response from gRPC reflection".into()),
    }
}

async fn reflect_pool_v1alpha(channel: Channel) -> Result<DescriptorPool, String> {
    use pb::v1alpha::server_reflection_request::MessageRequest;
    let (request_tx, request_rx) = mpsc::channel(8);
    let mut client = pb::v1alpha::server_reflection_client::ServerReflectionClient::new(channel);
    let mut responses = client
        .server_reflection_info(tokio_stream::wrappers::ReceiverStream::new(request_rx))
        .await
        .map_err(|error| error.to_string())?
        .into_inner();
    request_tx
        .send(pb::v1alpha::ServerReflectionRequest {
            host: String::new(),
            message_request: Some(MessageRequest::ListServices(String::new())),
        })
        .await
        .map_err(|error| error.to_string())?;
    let service_response = responses
        .message()
        .await
        .map_err(|error| error.to_string())?
        .ok_or_else(|| "reflection returned no service list".to_owned())?;
    let services = match service_response.message_response {
        Some(pb::v1alpha::server_reflection_response::MessageResponse::ListServicesResponse(
            list,
        )) => list.service,
        Some(pb::v1alpha::server_reflection_response::MessageResponse::ErrorResponse(error)) => {
            return Err(format!(
                "reflection error {}: {}",
                error.error_code, error.error_message
            ));
        }
        _ => return Err("unexpected response while listing gRPC services".into()),
    };
    let mut descriptors = BTreeMap::<String, FileDescriptorProto>::new();
    for service in services {
        if service.name == "grpc.reflection.v1.ServerReflection"
            || service.name == "grpc.reflection.v1alpha.ServerReflection"
        {
            continue;
        }
        request_tx
            .send(pb::v1alpha::ServerReflectionRequest {
                host: String::new(),
                message_request: Some(MessageRequest::FileContainingSymbol(service.name)),
            })
            .await
            .map_err(|error| error.to_string())?;
        let response = responses
            .message()
            .await
            .map_err(|error| error.to_string())?
            .ok_or_else(|| "reflection returned no descriptor response".to_owned())?;
        match response.message_response {
            Some(
                pb::v1alpha::server_reflection_response::MessageResponse::FileDescriptorResponse(
                    result,
                ),
            ) => {
                for bytes in result.file_descriptor_proto {
                    let file = FileDescriptorProto::decode(bytes.as_slice())
                        .map_err(|error| error.to_string())?;
                    let name = file
                        .name
                        .clone()
                        .unwrap_or_else(|| format!("anonymous-{}", descriptors.len()));
                    descriptors.entry(name).or_insert(file);
                }
            }
            Some(pb::v1alpha::server_reflection_response::MessageResponse::ErrorResponse(
                error,
            )) => {
                return Err(format!(
                    "reflection error {}: {}",
                    error.error_code, error.error_message
                ));
            }
            _ => return Err("unexpected descriptor response from gRPC reflection".into()),
        }
    }
    build_reflected_pool(descriptors)
}

fn build_reflected_pool(
    descriptors: BTreeMap<String, FileDescriptorProto>,
) -> Result<DescriptorPool, String> {
    if descriptors.is_empty() {
        return Err("reflection returned no service descriptors".into());
    }
    fn append_with_dependencies(
        name: &str,
        descriptors: &BTreeMap<String, FileDescriptorProto>,
        visited: &mut std::collections::BTreeSet<String>,
        ordered: &mut Vec<FileDescriptorProto>,
    ) {
        if !visited.insert(name.to_owned()) {
            return;
        }
        if let Some(file) = descriptors.get(name) {
            for dependency in &file.dependency {
                append_with_dependencies(dependency, descriptors, visited, ordered);
            }
            ordered.push(file.clone());
        }
    }
    let mut visited = std::collections::BTreeSet::new();
    let mut ordered = Vec::with_capacity(descriptors.len());
    for name in descriptors.keys() {
        append_with_dependencies(name, &descriptors, &mut visited, &mut ordered);
    }
    DescriptorPool::from_file_descriptor_set(FileDescriptorSet { file: ordered })
        .map_err(|error| format!("could not build reflected descriptors: {error}"))
}

fn sample_message_value(descriptor: &MessageDescriptor, visited: &mut Vec<String>) -> Value {
    let name = descriptor.full_name().to_owned();
    if visited.contains(&name) {
        return json!({});
    }
    visited.push(name.clone());
    let mut result = serde_json::Map::new();
    for field in descriptor.fields() {
        let value = sample_field(&field.kind(), field.name(), visited);
        if field.is_map() {
            result.insert(field.json_name().to_owned(), json!({ "key": "value" }));
        } else if field.is_list() {
            result.insert(field.json_name().to_owned(), json!([value]));
        } else {
            result.insert(field.json_name().to_owned(), value);
        }
    }
    visited.pop();
    Value::Object(result)
}

fn sample_field(kind: &Kind, field_name: &str, visited: &mut Vec<String>) -> Value {
    match kind {
        Kind::Bool => json!(true),
        Kind::Int32
        | Kind::Sint32
        | Kind::Sfixed32
        | Kind::Uint32
        | Kind::Fixed32
        | Kind::Int64
        | Kind::Sint64
        | Kind::Sfixed64
        | Kind::Uint64
        | Kind::Fixed64
        | Kind::Float
        | Kind::Double => json!(0),
        Kind::String => json!(format!("sample_{field_name}")),
        Kind::Bytes => json!("c2FtcGxl"),
        Kind::Enum(descriptor) => sample_enum(descriptor),
        Kind::Message(descriptor) => match descriptor.full_name() {
            "google.protobuf.Timestamp" => json!("2026-10-03T12:00:00Z"),
            "google.protobuf.Duration" => json!("1s"),
            "google.protobuf.StringValue" => json!("sample"),
            "google.protobuf.Int32Value" | "google.protobuf.Int64Value" => json!(0),
            "google.protobuf.BoolValue" => json!(true),
            "google.protobuf.Struct" => json!({ "key": "value" }),
            _ => sample_message_value(descriptor, visited),
        },
    }
}

fn sample_enum(descriptor: &EnumDescriptor) -> Value {
    descriptor
        .values()
        .next()
        .map(|value| json!(value.name()))
        .unwrap_or(json!(0))
}

pub async fn execute(
    grpc_definition: &Value,
    workspace: Option<&str>,
    timeout_ms: u64,
    mut on_message: impl FnMut(Value),
) -> Value {
    let started = Instant::now();
    let method = match resolve_method(grpc_definition, workspace).await {
        Ok(method) => method,
        Err(error) => {
            return failure_result(
                500,
                "Method Resolution Failed",
                &error,
                "UNKNOWN",
                None,
                started,
            );
        }
    };
    let address = grpc_definition
        .get("address")
        .and_then(Value::as_str)
        .unwrap_or_default();
    if address.is_empty() {
        return failure_result(
            400,
            "Request Error",
            "gRPC address is required",
            "INVALID_ARGUMENT",
            Some(3),
            started,
        );
    }
    let use_tls = grpc_definition
        .get("useTls")
        .and_then(Value::as_bool)
        .unwrap_or(false);
    let skip_verify = grpc_definition
        .get("insecureSkipVerify")
        .and_then(Value::as_bool)
        .unwrap_or(false);
    let root_ca = grpc_definition
        .get("rootCaPath")
        .and_then(Value::as_str)
        .unwrap_or_default();
    let channel = match connect(
        address,
        use_tls,
        skip_verify,
        workspace.map(Path::new),
        root_ca,
    )
    .await
    {
        Ok(channel) => channel,
        Err(error) => {
            return failure_result(
                500,
                "Connection Failed",
                &error,
                "UNAVAILABLE",
                Some(14),
                started,
            );
        }
    };

    let request_messages = grpc_definition
        .get("messages")
        .and_then(Value::as_array)
        .map(|items| {
            items
                .iter()
                .filter_map(Value::as_str)
                .map(str::to_owned)
                .collect::<Vec<_>>()
        })
        .filter(|items| !items.is_empty())
        .unwrap_or_else(|| {
            grpc_definition
                .get("message")
                .and_then(Value::as_str)
                .filter(|message| !message.trim().is_empty())
                .map(|message| vec![message.to_owned()])
                .unwrap_or_default()
        });
    let is_client_stream = method.is_client_streaming();
    let is_server_stream = method.is_server_streaming();
    let input_descriptor = method.input();
    let output_descriptor = method.output();
    let mut encoded_messages = Vec::new();
    if is_client_stream {
        for raw in &request_messages {
            match parse_dynamic_message(&input_descriptor, raw) {
                Ok(message) => encoded_messages.push(message),
                Err(error) => {
                    return failure_result(
                        400,
                        "3 INVALID_ARGUMENT",
                        &error,
                        "INVALID_ARGUMENT",
                        Some(3),
                        started,
                    );
                }
            }
        }
    } else if is_server_stream {
        let raw = grpc_definition
            .get("message")
            .and_then(Value::as_str)
            .filter(|value| !value.trim().is_empty())
            .unwrap_or("{}");
        match parse_dynamic_message(&input_descriptor, raw) {
            Ok(message) => encoded_messages.push(message),
            Err(error) => {
                return failure_result(
                    400,
                    "3 INVALID_ARGUMENT",
                    &error,
                    "INVALID_ARGUMENT",
                    Some(3),
                    started,
                );
            }
        }
    } else {
        let raw = grpc_definition
            .get("message")
            .and_then(Value::as_str)
            .unwrap_or("{}");
        match parse_dynamic_message(&input_descriptor, raw) {
            Ok(message) => encoded_messages.push(message),
            Err(error) => {
                return failure_result(
                    400,
                    "3 INVALID_ARGUMENT",
                    &error,
                    "INVALID_ARGUMENT",
                    Some(3),
                    started,
                );
            }
        }
    }

    let mut metadata = MetadataMap::new();
    if let Some(items) = grpc_definition.get("metadata").and_then(Value::as_array) {
        for item in items.iter().filter(|item| {
            item.get("enabled")
                .and_then(Value::as_bool)
                .unwrap_or(false)
        }) {
            let key = item
                .get("key")
                .and_then(Value::as_str)
                .unwrap_or_default()
                .trim()
                .to_ascii_lowercase();
            let value = item
                .get("value")
                .and_then(Value::as_str)
                .unwrap_or_default();
            if key.is_empty() {
                continue;
            }
            let result = if key.ends_with("-bin") {
                let key = MetadataKey::<Binary>::from_bytes(key.as_bytes())
                    .map_err(|error| error.to_string());
                let value = MetadataValue::<Binary>::from_bytes(value.as_bytes());
                key.map(|key| metadata.append_bin(key, value))
            } else {
                let key = MetadataKey::<Ascii>::from_bytes(key.as_bytes())
                    .map_err(|error| error.to_string());
                let value =
                    MetadataValue::<Ascii>::try_from(value).map_err(|error| error.to_string());
                key.and_then(|key| value.map(|value| metadata.append(key, value)))
            };
            if let Err(error) = result {
                return failure_result(
                    400,
                    "3 INVALID_ARGUMENT",
                    &format!("invalid gRPC metadata: {error}"),
                    "INVALID_ARGUMENT",
                    Some(3),
                    started,
                );
            }
        }
    }

    let service = method.parent_service().full_name();
    let path =
        match format!("/{service}/{}", method.name()).parse::<axum::http::uri::PathAndQuery>() {
            Ok(path) => path,
            Err(error) => {
                return failure_result(
                    400,
                    "Request Error",
                    &error.to_string(),
                    "INVALID_ARGUMENT",
                    Some(3),
                    started,
                );
            }
        };
    let codec = DynamicCodec {
        output: output_descriptor.clone(),
    };
    let mut client = tonic::client::Grpc::new(channel);
    if let Err(error) = client.ready().await {
        return failure_result(
            503,
            "Connection Failed",
            &error.to_string(),
            "UNAVAILABLE",
            Some(14),
            started,
        );
    }

    let mut body = String::new();
    let mut response_metadata = Value::Object(serde_json::Map::new());
    let mut response_trailers = Value::Object(serde_json::Map::new());
    let mut stream_messages = Vec::new();
    let mut message_index = 0usize;
    let mut call_error: Option<Status> = None;
    let timeout = Duration::from_millis(timeout_ms);
    let operation = async {
        if !is_client_stream && !is_server_stream {
            let raw = grpc_definition
                .get("message")
                .and_then(Value::as_str)
                .filter(|value| !value.trim().is_empty())
                .unwrap_or("{}");
            record_message(
                &mut stream_messages,
                &mut message_index,
                &mut on_message,
                "send",
                raw.to_owned(),
                false,
            );
            let mut request = Request::new(encoded_messages.remove(0));
            *request.metadata_mut() = metadata;
            match client.unary(request, path, codec).await {
                Ok(response) => {
                    response_metadata = metadata_json(response.metadata());
                    match serde_json::to_string_pretty(response.get_ref()) {
                        Ok(serialized) => {
                            body = serialized.clone();
                            record_message(
                                &mut stream_messages,
                                &mut message_index,
                                &mut on_message,
                                "receive",
                                serialized,
                                false,
                            );
                        }
                        Err(error) => call_error = Some(Status::internal(error.to_string())),
                    }
                }
                Err(error) => call_error = Some(error),
            }
        } else if !is_client_stream && is_server_stream {
            let raw = grpc_definition
                .get("message")
                .and_then(Value::as_str)
                .filter(|value| !value.trim().is_empty())
                .unwrap_or("{}");
            record_message(
                &mut stream_messages,
                &mut message_index,
                &mut on_message,
                "send",
                raw.to_owned(),
                false,
            );
            let mut request = Request::new(encoded_messages.remove(0));
            *request.metadata_mut() = metadata;
            match client.server_streaming(request, path, codec).await {
                Ok(response) => {
                    response_metadata = metadata_json(response.metadata());
                    let mut incoming = response.into_inner();
                    let mut messages = Vec::<String>::new();
                    loop {
                        match incoming.message().await {
                            Ok(Some(message)) => match serde_json::to_string(&message) {
                                Ok(text) => {
                                    messages.push(text.clone());
                                    record_message(
                                        &mut stream_messages,
                                        &mut message_index,
                                        &mut on_message,
                                        "receive",
                                        text,
                                        false,
                                    );
                                }
                                Err(error) => {
                                    call_error = Some(Status::internal(error.to_string()));
                                    break;
                                }
                            },
                            Ok(None) => break,
                            Err(error) => {
                                call_error = Some(error);
                                break;
                            }
                        }
                    }
                    if let Ok(Some(trailers)) = incoming.trailers().await {
                        response_trailers = metadata_json(&trailers);
                    }
                    body = format_stream_body(&messages);
                }
                Err(error) => call_error = Some(error),
            }
        } else {
            let messages = encoded_messages;
            for raw in &request_messages {
                record_message(
                    &mut stream_messages,
                    &mut message_index,
                    &mut on_message,
                    "send",
                    raw.clone(),
                    false,
                );
            }
            let mut request = Request::new(tokio_stream::iter(messages));
            *request.metadata_mut() = metadata;
            if is_client_stream && !is_server_stream {
                match client.client_streaming(request, path, codec).await {
                    Ok(response) => {
                        response_metadata = metadata_json(response.metadata());
                        match serde_json::to_string_pretty(response.get_ref()) {
                            Ok(serialized) => {
                                body = serialized.clone();
                                record_message(
                                    &mut stream_messages,
                                    &mut message_index,
                                    &mut on_message,
                                    "receive",
                                    serialized,
                                    false,
                                );
                            }
                            Err(error) => call_error = Some(Status::internal(error.to_string())),
                        }
                    }
                    Err(error) => call_error = Some(error),
                }
            } else {
                match client.streaming(request, path, codec).await {
                    Ok(response) => {
                        response_metadata = metadata_json(response.metadata());
                        let mut incoming = response.into_inner();
                        let mut messages = Vec::<String>::new();
                        loop {
                            match incoming.message().await {
                                Ok(Some(message)) => match serde_json::to_string(&message) {
                                    Ok(text) => {
                                        messages.push(text.clone());
                                        record_message(
                                            &mut stream_messages,
                                            &mut message_index,
                                            &mut on_message,
                                            "receive",
                                            text,
                                            false,
                                        );
                                    }
                                    Err(error) => {
                                        call_error = Some(Status::internal(error.to_string()));
                                        break;
                                    }
                                },
                                Ok(None) => break,
                                Err(error) => {
                                    call_error = Some(error);
                                    break;
                                }
                            }
                        }
                        if let Ok(Some(trailers)) = incoming.trailers().await {
                            response_trailers = metadata_json(&trailers);
                        }
                        body = format_stream_body(&messages);
                    }
                    Err(error) => call_error = Some(error),
                }
            }
        }
    };

    if tokio::time::timeout(timeout, operation).await.is_err() {
        call_error = Some(Status::deadline_exceeded("gRPC request timed out"));
    }
    let (grpc_status, grpc_status_text, status_code, status_text, error) = match call_error {
        Some(error) => {
            let code = grpc_code(error.code());
            let status = map_status(error.code());
            (
                code,
                error.code().to_string(),
                status,
                format!("{code} {}", error.code()),
                Some(error.message().to_owned()),
            )
        }
        None => (0, "OK".into(), 200, "0 OK".into(), None),
    };
    let body_size = body.len();
    json!({
        "statusCode": status_code,
        "statusText": status_text,
        "headers": response_metadata.clone(),
        "body": body,
        "size": body_size,
        "timing": { "dnsLookupMs": 0, "tcpConnectMs": 0, "tlsHandshakeMs": 0, "ttfbMs": 0, "downloadMs": 0, "totalDurationMs": started.elapsed().as_millis() as f64 },
        "tests": [],
        "logs": [],
        "executedAt": chrono::Utc::now().to_rfc3339(),
        "grpcStatus": grpc_status,
        "grpcStatusText": grpc_status_text,
        "grpcMetadata": response_metadata,
        "grpcTrailers": response_trailers,
        "grpcMessages": stream_messages,
        "error": error,
    })
}

fn parse_dynamic_message(
    descriptor: &MessageDescriptor,
    raw: &str,
) -> Result<DynamicMessage, String> {
    let raw = if raw.trim().is_empty() { "{}" } else { raw };
    let mut deserializer = serde_json::Deserializer::from_str(raw);
    let message = DynamicMessage::deserialize(descriptor.clone(), &mut deserializer)
        .map_err(|error| format!("invalid JSON message for {}: {error}", descriptor.name()))?;
    deserializer
        .end()
        .map_err(|error| format!("invalid JSON message: {error}"))?;
    Ok(message)
}

fn format_stream_body(messages: &[String]) -> String {
    format!("[{}]", messages.join(",\n"))
}

fn record_message(
    messages: &mut Vec<Value>,
    index: &mut usize,
    callback: &mut impl FnMut(Value),
    direction: &str,
    payload: String,
    is_error: bool,
) {
    let message = json!({
        "index": *index,
        "direction": direction,
        "timestamp": chrono::Utc::now().to_rfc3339(),
        "payload": payload,
        "isError": is_error,
    });
    *index += 1;
    messages.push(message.clone());
    callback(message);
}

fn metadata_json(metadata: &MetadataMap) -> Value {
    let mut result = serde_json::Map::<String, Value>::new();
    for entry in metadata.iter() {
        let (key, value) = match entry {
            KeyAndValueRef::Ascii(key, value) => {
                (key.as_str().to_owned(), value.as_encoded_bytes())
            }
            KeyAndValueRef::Binary(key, value) => {
                (key.as_str().to_owned(), value.as_encoded_bytes())
            }
        };
        result
            .entry(key)
            .or_insert_with(|| json!([]))
            .as_array_mut()
            .expect("metadata values are arrays")
            .push(json!(String::from_utf8_lossy(value)));
    }
    Value::Object(result)
}

fn grpc_code(code: Code) -> i32 {
    match code {
        Code::Ok => 0,
        Code::Cancelled => 1,
        Code::Unknown => 2,
        Code::InvalidArgument => 3,
        Code::DeadlineExceeded => 4,
        Code::NotFound => 5,
        Code::AlreadyExists => 6,
        Code::PermissionDenied => 7,
        Code::ResourceExhausted => 8,
        Code::FailedPrecondition => 9,
        Code::Aborted => 10,
        Code::OutOfRange => 11,
        Code::Unimplemented => 12,
        Code::Internal => 13,
        Code::Unavailable => 14,
        Code::DataLoss => 15,
        Code::Unauthenticated => 16,
    }
}

fn map_status(code: Code) -> u16 {
    match code {
        Code::Ok => 200,
        Code::Cancelled => 499,
        Code::InvalidArgument => 400,
        Code::DeadlineExceeded => 504,
        Code::NotFound => 404,
        Code::AlreadyExists => 409,
        Code::PermissionDenied => 403,
        Code::Unauthenticated => 401,
        Code::ResourceExhausted => 429,
        Code::Unimplemented => 501,
        Code::Unavailable => 503,
        _ => 500,
    }
}

fn failure_result(
    status_code: u16,
    status_text: &str,
    message: &str,
    grpc_status_text: &str,
    grpc_status: Option<i32>,
    started: Instant,
) -> Value {
    json!({
        "statusCode": status_code,
        "statusText": status_text,
        "headers": {},
        "body": "",
        "size": 0,
        "timing": { "dnsLookupMs": 0, "tcpConnectMs": 0, "tlsHandshakeMs": 0, "ttfbMs": 0, "downloadMs": 0, "totalDurationMs": started.elapsed().as_millis() as f64 },
        "tests": [],
        "logs": [format!("[ERROR] {message}")],
        "executedAt": chrono::Utc::now().to_rfc3339(),
        "grpcStatus": grpc_status,
        "grpcStatusText": grpc_status_text,
        "error": message,
    })
}

fn cancelled_result() -> Value {
    json!({
        "statusCode": 499,
        "statusText": "1 CANCELLED",
        "headers": {},
        "body": "",
        "size": 0,
        "timing": { "totalDurationMs": 0 },
        "tests": [],
        "logs": ["[INFO] gRPC stream cancelled"],
        "executedAt": chrono::Utc::now().to_rfc3339(),
        "grpcStatus": 1,
        "grpcStatusText": "CANCELLED",
        "error": "gRPC stream cancelled",
    })
}
