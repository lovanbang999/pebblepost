//! Shared JSON contracts that replace the former Go `internal/types` package.
//!
//! Missing input fields deserialize to their Go zero values. Optional fields
//! keep the repository's existing `omitempty` wire format.

use std::collections::BTreeMap;

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};

fn empty_string(value: &String) -> bool { value.is_empty() }
fn false_value(value: &bool) -> bool { !*value }
fn zero_i64(value: &i64) -> bool { *value == 0 }
fn empty_vec<T>(value: &Vec<T>) -> bool { value.is_empty() }
fn empty_map<K, V>(value: &BTreeMap<K, V>) -> bool { value.is_empty() }
fn go_zero_time() -> DateTime<Utc> {
    DateTime::<Utc>::from_timestamp(-62_135_596_800, 0).expect("Go's zero time is representable in Chrono")
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct KeyValue {
    pub key: String,
    pub value: String,
    pub enabled: bool,
    #[serde(skip_serializing_if = "empty_string")]
    pub r#type: String,
    #[serde(skip_serializing_if = "false_value")]
    pub secret: bool,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct AuthDefinition {
    pub r#type: String,
    #[serde(skip_serializing_if = "empty_string")] pub token: String,
    #[serde(skip_serializing_if = "empty_string")] pub username: String,
    #[serde(skip_serializing_if = "empty_string")] pub password: String,
    #[serde(skip_serializing_if = "empty_string")] pub key: String,
    #[serde(skip_serializing_if = "empty_string")] pub value: String,
    #[serde(skip_serializing_if = "empty_string")] pub add_to: String,
    #[serde(skip_serializing_if = "empty_string")] pub realm: String,
    #[serde(skip_serializing_if = "empty_string")] pub grant_type: String,
    #[serde(skip_serializing_if = "empty_string")] pub auth_url: String,
    #[serde(skip_serializing_if = "empty_string")] pub token_url: String,
    #[serde(skip_serializing_if = "empty_string")] pub client_id: String,
    #[serde(skip_serializing_if = "empty_string")] pub client_secret: String,
    #[serde(skip_serializing_if = "empty_string")] pub scope: String,
    #[serde(skip_serializing_if = "empty_string")] pub redirect_url: String,
    #[serde(skip_serializing_if = "empty_string")] pub code_verifier: String,
    #[serde(skip_serializing_if = "empty_string")] pub refresh_token: String,
    #[serde(skip_serializing_if = "zero_i64")] pub token_expires_at: i64,
    #[serde(skip_serializing_if = "empty_string")] pub access_key: String,
    #[serde(skip_serializing_if = "empty_string")] pub secret_key: String,
    #[serde(skip_serializing_if = "empty_string")] pub region: String,
    #[serde(skip_serializing_if = "empty_string")] pub service: String,
    #[serde(skip_serializing_if = "empty_string")] pub session_token: String,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct BodyDefinition {
    pub r#type: String,
    #[serde(skip_serializing_if = "empty_string")] pub raw: String,
    #[serde(skip_serializing_if = "empty_string")] pub file_path: String,
    #[serde(skip_serializing_if = "empty_vec")] pub form_data: Vec<KeyValue>,
    #[serde(skip_serializing_if = "empty_vec")] pub url_encoded: Vec<KeyValue>,
    #[serde(skip_serializing_if = "Option::is_none")] pub graphql: Option<GraphQL>,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct GraphQL {
    pub query: String,
    #[serde(skip_serializing_if = "empty_string")] pub variables: String,
    #[serde(skip_serializing_if = "empty_string")] pub operation_name: String,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct ScriptDefinition {
    #[serde(skip_serializing_if = "empty_string")] pub pre_request: String,
    #[serde(skip_serializing_if = "empty_string")] pub post_response: String,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct ExtractorDefinition {
    pub id: String,
    pub name: String,
    #[serde(skip_serializing_if = "empty_string")] pub r#type: String,
    pub path: String,
    #[serde(skip_serializing_if = "empty_string")] pub scope: String,
    pub enabled: bool,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct SettingDefinition {
    pub follow_redirects: bool,
    #[serde(rename = "verifySSL")]
    pub verify_ssl: bool,
    pub timeout_ms: i64,
    #[serde(skip_serializing_if = "zero_i64")] pub script_timeout_ms: i64,
    #[serde(skip_serializing_if = "zero_i64")] pub connect_timeout_ms: i64,
    #[serde(skip_serializing_if = "zero_i64")] pub max_redirects: i64,
    #[serde(skip_serializing_if = "Option::is_none")] pub enable_cookies: Option<bool>,
    #[serde(skip_serializing_if = "empty_string")] pub user_agent: String,
    #[serde(skip_serializing_if = "empty_string")] pub proxy_url: String,
    #[serde(skip_serializing_if = "empty_string")] pub client_cert_path: String,
    #[serde(skip_serializing_if = "empty_string")] pub client_key_path: String,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct CookieItem {
    pub name: String,
    pub value: String,
    pub domain: String,
    pub path: String,
    #[serde(default = "go_zero_time")] pub expires: DateTime<Utc>,
    pub max_age: i64,
    pub secure: bool,
    pub http_only: bool,
    #[serde(skip_serializing_if = "empty_string")] pub same_site: String,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct GrpcDefinition {
    pub address: String,
    pub proto_source: String,
    #[serde(skip_serializing_if = "empty_vec")] pub proto_files: Vec<String>,
    #[serde(skip_serializing_if = "empty_vec")] pub import_paths: Vec<String>,
    pub service: String,
    pub method: String,
    #[serde(skip_serializing_if = "empty_vec")] pub metadata: Vec<KeyValue>,
    #[serde(skip_serializing_if = "empty_string")] pub message: String,
    #[serde(skip_serializing_if = "empty_vec")] pub messages: Vec<String>,
    #[serde(rename = "useTls")] pub use_tls: bool,
    #[serde(skip_serializing_if = "false_value")] pub insecure_skip_verify: bool,
    #[serde(skip_serializing_if = "empty_string")] pub root_ca_path: String,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct GrpcStreamMessage {
    pub index: i64,
    pub direction: String,
    #[serde(default = "go_zero_time")] pub timestamp: DateTime<Utc>,
    pub payload: String,
    #[serde(skip_serializing_if = "false_value")] pub is_error: bool,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct GrpcMethodInfo {
    pub name: String,
    pub full_method: String,
    pub client_streaming: bool,
    pub server_streaming: bool,
    pub input_type: String,
    pub output_type: String,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct GrpcServiceInfo {
    pub name: String,
    pub methods: Vec<GrpcMethodInfo>,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct WebSocketMessage {
    pub id: String,
    #[serde(skip_serializing_if = "empty_string")] pub name: String,
    pub payload: String,
    #[serde(skip_serializing_if = "empty_string")] pub r#type: String,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct StreamDefinition {
    #[serde(skip_serializing_if = "empty_vec")] pub subprotocols: Vec<String>,
    #[serde(skip_serializing_if = "false_value")] pub auto_reconnect: bool,
    #[serde(skip_serializing_if = "zero_i64")] pub max_reconnect_attempts: i64,
    #[serde(skip_serializing_if = "zero_i64")] pub reconnect_interval_ms: i64,
    #[serde(skip_serializing_if = "zero_i64")] pub ping_interval_ms: i64,
    #[serde(skip_serializing_if = "zero_i64")] pub max_log_entries: i64,
    #[serde(skip_serializing_if = "zero_i64")] pub max_log_bytes: i64,
    #[serde(skip_serializing_if = "empty_vec")] pub outgoing_messages: Vec<WebSocketMessage>,
    #[serde(skip_serializing_if = "zero_i64")] pub timeout_ms: i64,
    #[serde(skip_serializing_if = "zero_i64")] pub max_wait_messages: i64,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct StreamLogEntry {
    pub id: String,
    pub index: i64,
    pub direction: String,
    pub r#type: String,
    #[serde(default = "go_zero_time")] pub timestamp: DateTime<Utc>,
    pub payload: String,
    pub size: i64,
    #[serde(skip_serializing_if = "zero_i64")] pub close_code: i64,
    #[serde(skip_serializing_if = "empty_string")] pub close_reason: String,
    #[serde(skip_serializing_if = "false_value")] pub is_error: bool,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct StreamSessionStatus {
    pub stream_id: String,
    pub protocol: String,
    pub state: String,
    pub url: String,
    #[serde(skip_serializing_if = "empty_string")] pub subprotocol: String,
    pub reconnect_count: i64,
    pub total_sent: i64,
    pub total_received: i64,
    pub evicted_count: i64,
    #[serde(skip_serializing_if = "zero_i64")] pub close_code: i64,
    #[serde(skip_serializing_if = "empty_string")] pub close_reason: String,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct ExampleResponse {
    pub id: String,
    pub name: String,
    pub status_code: i64,
    #[serde(skip_serializing_if = "empty_string")] pub status_text: String,
    #[serde(skip_serializing_if = "empty_vec")] pub headers: Vec<KeyValue>,
    #[serde(skip_serializing_if = "empty_string")] pub body: String,
    #[serde(skip_serializing_if = "empty_string")] pub content_type: String,
    #[serde(skip_serializing_if = "zero_i64")] pub duration_ms: i64,
    #[serde(skip_serializing_if = "zero_i64")] pub size: i64,
    #[serde(default = "go_zero_time")] pub saved_at: DateTime<Utc>,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct RequestDefinition {
    #[serde(rename = "$schema", skip_serializing_if = "empty_string")] pub schema: String,
    pub schema_version: i64,
    #[serde(skip_serializing_if = "empty_string")] pub version: String,
    #[serde(skip_serializing_if = "empty_string")] pub id: String,
    pub name: String,
    #[serde(skip_serializing_if = "empty_string")] pub description: String,
    #[serde(skip_serializing_if = "zero_i64")] pub order: i64,
    #[serde(skip_serializing_if = "empty_vec")] pub tags: Vec<String>,
    #[serde(skip_serializing_if = "empty_string")] pub protocol: String,
    pub method: String,
    pub url: String,
    #[serde(skip_serializing_if = "empty_vec")] pub headers: Vec<KeyValue>,
    #[serde(skip_serializing_if = "empty_vec")] pub params: Vec<KeyValue>,
    pub auth: AuthDefinition,
    pub body: BodyDefinition,
    #[serde(skip_serializing_if = "Option::is_none")] pub grpc: Option<GrpcDefinition>,
    #[serde(skip_serializing_if = "Option::is_none")] pub stream: Option<StreamDefinition>,
    pub scripts: ScriptDefinition,
    pub settings: SettingDefinition,
    #[serde(skip_serializing_if = "empty_vec")] pub examples: Vec<ExampleResponse>,
    #[serde(skip_serializing_if = "empty_vec")] pub extractors: Vec<ExtractorDefinition>,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct EnvironmentDefinition {
    pub schema_version: i64,
    pub name: String,
    pub variables: Vec<KeyValue>,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct WorkspaceDefinition {
    pub schema_version: i64,
    #[serde(skip_serializing_if = "empty_string")] pub version: String,
    pub name: String,
    #[serde(skip_serializing_if = "empty_string")] pub active_environment: String,
    #[serde(skip_serializing_if = "false_value")] pub trusted: bool,
    pub secret_backend: SecretBackendConfig,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct SecretBackendConfig {
    #[serde(skip_serializing_if = "empty_string")] pub default: String,
    #[serde(skip_serializing_if = "empty_map")] pub per_env: BTreeMap<String, String>,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct OpenAPISyncConfig {
    pub spec_location: String,
    #[serde(skip_serializing_if = "empty_string")] pub last_synced_at: String,
    #[serde(skip_serializing_if = "empty_string")] pub spec_hash: String,
    #[serde(skip_serializing_if = "false_value")] pub auto_sync: bool,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct FolderDefinition {
    pub schema_version: i64,
    #[serde(skip_serializing_if = "empty_string")] pub name: String,
    #[serde(skip_serializing_if = "empty_string")] pub description: String,
    #[serde(skip_serializing_if = "zero_i64")] pub order: i64,
    #[serde(skip_serializing_if = "empty_vec")] pub item_order: Vec<String>,
    #[serde(skip_serializing_if = "empty_vec")] pub headers: Vec<KeyValue>,
    pub auth: AuthDefinition,
    #[serde(skip_serializing_if = "empty_vec")] pub variables: Vec<KeyValue>,
    pub scripts: ScriptDefinition,
    #[serde(skip_serializing_if = "Option::is_none")] pub open_api_sync: Option<OpenAPISyncConfig>,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct InheritedItemInfo {
    pub source_folder: String,
    pub source_path: String,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct ResolvedRequestResult {
    pub request: RequestDefinition,
    pub inherited_headers: BTreeMap<String, InheritedItemInfo>,
    #[serde(skip_serializing_if = "empty_map")] pub overridden_headers: BTreeMap<String, InheritedItemInfo>,
    #[serde(skip_serializing_if = "Option::is_none")] pub inherited_auth: Option<InheritedItemInfo>,
    #[serde(skip_serializing_if = "Option::is_none")] pub parent_auth: Option<AuthDefinition>,
    #[serde(skip_serializing_if = "Option::is_none")] pub parent_auth_source: Option<InheritedItemInfo>,
    pub inherited_vars: BTreeMap<String, InheritedItemInfo>,
    #[serde(skip_serializing_if = "empty_vec")] pub folder_pre_scripts: Vec<String>,
    #[serde(skip_serializing_if = "empty_vec")] pub folder_post_scripts: Vec<String>,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct RedirectHop {
    pub status_code: i64,
    pub method: String,
    pub url: String,
    #[serde(skip_serializing_if = "empty_map")] pub headers: BTreeMap<String, String>,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct SentRequestSummary {
    pub method: String,
    pub url: String,
    pub headers: BTreeMap<String, Vec<String>>,
    #[serde(skip_serializing_if = "empty_string")] pub body: String,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct TimingMetrics {
    pub dns_lookup_ms: f64,
    pub tcp_connect_ms: f64,
    pub tls_handshake_ms: f64,
    #[serde(rename = "ttfbMs")] pub ttfb_ms: f64,
    pub download_ms: f64,
    pub total_duration_ms: f64,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct TestAssertionResult {
    pub name: String,
    pub passed: bool,
    #[serde(skip_serializing_if = "empty_string")] pub message: String,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct ConsoleLogEntry {
    #[serde(default = "go_zero_time")] pub timestamp: DateTime<Utc>,
    pub level: String,
    pub source: String,
    pub message: String,
    #[serde(skip_serializing_if = "zero_i64")] pub line: i64,
    #[serde(skip_serializing_if = "zero_i64")] pub column: i64,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct ExecutionResult {
    pub status_code: i64,
    pub status_text: String,
    pub headers: BTreeMap<String, Vec<String>>,
    pub body: String,
    #[serde(skip_serializing_if = "false_value")] pub body_truncated: bool,
    #[serde(skip_serializing_if = "zero_i64")] pub body_size_bytes: i64,
    #[serde(skip_serializing_if = "empty_string")] pub temp_body_file: String,
    pub size: i64,
    pub timing: TimingMetrics,
    #[serde(skip_serializing_if = "empty_vec")] pub redirect_chain: Vec<RedirectHop>,
    #[serde(skip_serializing_if = "Option::is_none")] pub sent_request: Option<SentRequestSummary>,
    #[serde(skip_serializing_if = "Option::is_none")] pub grpc_status: Option<i64>,
    #[serde(skip_serializing_if = "empty_string")] pub grpc_status_text: String,
    #[serde(skip_serializing_if = "empty_map")] pub grpc_metadata: BTreeMap<String, Vec<String>>,
    #[serde(skip_serializing_if = "empty_map")] pub grpc_trailers: BTreeMap<String, Vec<String>>,
    #[serde(skip_serializing_if = "empty_vec")] pub grpc_messages: Vec<GrpcStreamMessage>,
    #[serde(skip_serializing_if = "empty_vec")] pub stream_logs: Vec<StreamLogEntry>,
    #[serde(skip_serializing_if = "zero_i64")] pub stream_close_code: i64,
    #[serde(skip_serializing_if = "empty_string")] pub stream_close_reason: String,
    #[serde(skip_serializing_if = "zero_i64")] pub stream_evicted: i64,
    pub tests: Vec<TestAssertionResult>,
    pub logs: Vec<String>,
    #[serde(skip_serializing_if = "empty_vec")] pub console_logs: Vec<ConsoleLogEntry>,
    #[serde(skip_serializing_if = "empty_map")] pub extracted_env_vars: BTreeMap<String, String>,
    #[serde(default = "go_zero_time")] pub executed_at: DateTime<Utc>,
    #[serde(skip_serializing_if = "empty_string")] pub error: String,
}

#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, rename_all = "camelCase")]
pub struct TreeNode {
    pub id: String,
    pub name: String,
    #[serde(skip_serializing_if = "empty_string")] pub display_name: String,
    pub path: String,
    pub rel_path: String,
    pub is_dir: bool,
    #[serde(skip_serializing_if = "zero_i64")] pub order: i64,
    #[serde(skip_serializing_if = "empty_string")] pub method: String,
    #[serde(skip_serializing_if = "empty_string")] pub git_status: String,
    #[serde(skip_serializing_if = "empty_vec")] pub children: Vec<TreeNode>,
}
