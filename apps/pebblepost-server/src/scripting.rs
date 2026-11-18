use std::io::Read;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};
use std::time::{Duration, Instant};

use base64::Engine as _;
use base64::engine::general_purpose::{STANDARD, URL_SAFE, URL_SAFE_NO_PAD};
use hmac::{Hmac, Mac};
use md5::Md5;
use rquickjs::prelude::Func;
use rquickjs::{Context, Runtime};
use serde_json::{Value, json};
use sha2::{Digest, Sha256, Sha512};

const DEFAULT_TIMEOUT: Duration = Duration::from_secs(5);
const MAX_LOGS: usize = 500;
const MAX_LOG_LENGTH: usize = 4096;
const ASSERTIONS: &str = include_str!("assertion_library.js");

#[derive(Clone, Copy, PartialEq, Eq)]
pub enum ScriptPhase {
    PreRequest,
    PostResponse,
}

pub struct ScriptInput<'a> {
    pub phase: ScriptPhase,
    pub source: &'a str,
    pub script: &'a str,
    pub request: &'a Value,
    pub response: Option<&'a Value>,
    pub variables: &'a std::collections::HashMap<String, String>,
    pub data_row: Option<&'a Value>,
    pub iteration: usize,
    pub iteration_count: usize,
    pub timeout: Duration,
    pub secrets: &'a [String],
}

pub fn execute(input: ScriptInput<'_>) -> Result<Value, String> {
    if input.script.trim().is_empty() {
        return Ok(json!({
            "request": input.request,
            "tests": [], "logs": [], "consoleLogs": [], "extractedEnvVars": {}, "localVars": {},
            "nextRequest": Value::Null, "stopAll": false, "error": Value::Null,
        }));
    }

    validate_syntax(input.source, input.script)?;

    let runtime = Runtime::new().map_err(|error| format!("could not create JavaScript runtime: {error}"))?;
    runtime.set_memory_limit(64 * 1024 * 1024);
    runtime.set_max_stack_size(1024 * 1024);
    let timeout = if input.timeout.is_zero() { DEFAULT_TIMEOUT } else { input.timeout };
    let deadline = Arc::new(Instant::now() + timeout);
    let interrupt_deadline = Arc::clone(&deadline);
    runtime.set_interrupt_handler(Some(Box::new(move || Instant::now() >= *interrupt_deadline)));
    let context = Context::full(&runtime).map_err(|error| format!("could not create JavaScript context: {error}"))?;

    let mut state = json!({
        "phase": if input.phase == ScriptPhase::PreRequest { "pre" } else { "post" },
        "source": input.source,
        "request": input.request,
        "response": input.response.cloned().unwrap_or(Value::Null),
        "envVars": input.variables,
        "localVars": {},
        "dataRow": input.data_row.cloned().unwrap_or(Value::Null),
        "iteration": input.iteration,
        "iterationCount": input.iteration_count.max(1),
        "tests": [], "logs": [], "consoleLogs": [],
        "nextRequest": Value::Null, "nextRequestSet": false, "stopAll": false, "error": Value::Null,
    });
    if !state["request"].is_object() { state["request"] = json!({}); }
    if !state["envVars"].is_object() { state["envVars"] = json!({}); }
    let state_json = serde_json::to_string(&state).map_err(|error| error.to_string())?;
    let script = input.script.to_owned();
    let send_request_calls = Arc::new(AtomicUsize::new(0));
    let secrets = input.secrets.to_vec();
    let evaluation = context.with(|ctx| -> rquickjs::Result<String> {
        let globals = ctx.globals();
        globals.set("__pp_state_json", state_json)?;
        globals.set("__pp_script_source", script)?;
        globals.set("__pp_max_logs", MAX_LOGS)?;
        globals.set("__pp_max_log_length", MAX_LOG_LENGTH)?;
        register_natives(&globals, send_request_calls, timeout, secrets)?;
        ctx.eval::<(), _>("globalThis.__pp_state = JSON.parse(__pp_state_json);")?;
        ctx.eval::<(), _>(ASSERTIONS)?;
        ctx.eval::<(), _>(BOOTSTRAP)?;
        ctx.eval::<(), _>("try { (0, eval)(__pp_script_source); } catch (e) { __pp_record_error(e); }")?;
        ctx.eval::<(), _>("__pp_collect_state();")?;
        ctx.eval::<String, _>("JSON.stringify(__pp_state)")
    });

    match evaluation {
        Ok(_) if Instant::now() >= *deadline => Err(format!("[{}] execution timed out after {} ms", input.source, timeout.as_millis())),
        Ok(serialized) => {
            state = serde_json::from_str(&serialized).map_err(|error| format!("could not collect script results: {error}"))?;
            Ok(state)
        }
        Err(_error) if Instant::now() >= *deadline => Err(format!("[{}] execution timed out after {} ms", input.source, timeout.as_millis())),
        Err(error) => Err(format!("[{}] JavaScript execution failed: {error}", input.source)),
    }
}

/// Compile a script without running it, using the same JavaScript engine as the sandbox.
pub fn validate_syntax(source: &str, script: &str) -> Result<(), String> {
    if script.trim().is_empty() {
        return Ok(());
    }
    let runtime = Runtime::new().map_err(|error| format!("could not create JavaScript runtime: {error}"))?;
    runtime.set_memory_limit(16 * 1024 * 1024);
    runtime.set_max_stack_size(256 * 1024);
    let context = Context::full(&runtime).map_err(|error| format!("could not create JavaScript context: {error}"))?;
    let script_literal = serde_json::to_string(script).map_err(|error| error.to_string())?;
    context.with(|ctx| {
        ctx.eval::<bool, _>(format!("(new Function({script_literal}), true)"))
            .map(|_| ())
            .map_err(|error| format!("[{source}] Syntax error: {error}"))
    })
}

fn register_natives<'js>(
    globals: &rquickjs::Object<'js>,
    send_request_calls: Arc<AtomicUsize>,
    timeout: Duration,
    secrets: Vec<String>,
) -> rquickjs::Result<()> {
    globals.set("__pp_uuid", Func::from(native_uuid))?;
    globals.set("__pp_md5", Func::from(native_md5))?;
    globals.set("__pp_sha256", Func::from(native_sha256))?;
    globals.set("__pp_sha512", Func::from(native_sha512))?;
    globals.set("__pp_native_hmac", Func::from(native_hmac))?;
    globals.set("__pp_base64_encode", Func::from(native_base64_encode))?;
    globals.set("__pp_base64_decode", Func::from(native_base64_decode))?;
    globals.set("__pp_jwt_decode", Func::from(native_jwt_decode))?;
    globals.set("__pp_random_int", Func::from(native_random_int))?;
    globals.set("__pp_random_string", Func::from(native_random_string))?;
    globals.set("__pp_send_request", Func::from(move |config: String| -> String {
        if send_request_calls.fetch_add(1, Ordering::Relaxed) >= 5 {
            return json!({ "error": "pb.sendRequest limit exceeded (maximum 5 requests allowed per script execution)" }).to_string();
        }
        match serde_json::from_str::<Value>(&config) {
            Ok(config) => send_auxiliary_request(&config, timeout, &secrets),
            Err(error) => json!({ "error": format!("invalid pb.sendRequest configuration: {error}") }).to_string(),
        }
    }))?;
    Ok(())
}

fn native_uuid() -> String {
    let mut bytes = [0_u8; 16];
    if getrandom::fill(&mut bytes).is_err() { return String::new(); }
    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    format!("{}-{}-{}-{}-{}", hex(&bytes[0..4]), hex(&bytes[4..6]), hex(&bytes[6..8]), hex(&bytes[8..10]), hex(&bytes[10..]))
}

fn native_md5(input: String) -> String { hex(&Md5::digest(input.as_bytes())) }
fn native_sha256(input: String) -> String { hex(&Sha256::digest(input.as_bytes())) }
fn native_sha512(input: String) -> String { hex(&Sha512::digest(input.as_bytes())) }

fn native_hmac(algorithm: String, secret: String, message: String) -> String {
    macro_rules! hmac_hex {
        ($hash:ty) => {{
            let Ok(mut mac) = Hmac::<$hash>::new_from_slice(secret.as_bytes()) else {
                return json!({ "error": "invalid HMAC key" }).to_string();
            };
            mac.update(message.as_bytes());
            json!({ "value": hex(&mac.finalize().into_bytes()) }).to_string()
        }};
    }
    match algorithm.to_ascii_lowercase().as_str() {
        "sha256" => hmac_hex!(Sha256),
        "sha512" => hmac_hex!(Sha512),
        "md5" => hmac_hex!(Md5),
        _ => json!({ "error": format!("unsupported HMAC algorithm: {algorithm}") }).to_string(),
    }
}

fn native_base64_encode(input: String) -> String { STANDARD.encode(input.as_bytes()) }

fn native_base64_decode(input: String) -> String {
    let decoded = STANDARD.decode(input.as_bytes())
        .or_else(|_| URL_SAFE_NO_PAD.decode(input.as_bytes()))
        .or_else(|_| URL_SAFE.decode(input.as_bytes()));
    match decoded {
        Ok(bytes) => json!({ "value": String::from_utf8_lossy(&bytes) }).to_string(),
        Err(error) => json!({ "error": format!("Base64 decode failed: {error}") }).to_string(),
    }
}

fn native_jwt_decode(token: String) -> String {
    let parts = token.split('.').collect::<Vec<_>>();
    if parts.len() < 2 { return json!({ "error": "invalid JWT: expected at least 2 dot-separated segments" }).to_string(); }
    let decode = |segment: &str| -> Result<Value, String> {
        let bytes = URL_SAFE_NO_PAD.decode(segment).or_else(|_| URL_SAFE.decode(segment)).or_else(|_| STANDARD.decode(segment))
            .map_err(|error| format!("failed to base64-decode JWT segment: {error}"))?;
        Ok(serde_json::from_slice(&bytes).unwrap_or_else(|_| json!(String::from_utf8_lossy(&bytes).into_owned())))
    };
    match (decode(parts[0]), decode(parts[1])) {
        (Ok(header), Ok(payload)) => json!({ "header": header, "payload": payload }).to_string(),
        (Err(error), _) | (_, Err(error)) => json!({ "error": error }).to_string(),
    }
}

fn native_random_int(min: i64, max: i64) -> i64 {
    if min >= max { return min; }
    let width = (max as i128 - min as i128 + 1) as u128;
    let mut bytes = [0_u8; 8];
    if getrandom::fill(&mut bytes).is_err() { return min; }
    (min as i128 + (u64::from_le_bytes(bytes) as u128 % width) as i128) as i64
}

fn native_random_string(length: usize, charset: String) -> String {
    let length = length.min(4096);
    if length == 0 { return String::new(); }
    let charset = if charset.is_empty() { "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789" } else { charset.as_str() };
    let chars = charset.chars().collect::<Vec<_>>();
    if chars.is_empty() { return String::new(); }
    let mut bytes = vec![0_u8; length];
    if getrandom::fill(&mut bytes).is_err() { return String::new(); }
    bytes.into_iter().map(|byte| chars[byte as usize % chars.len()]).collect()
}

fn send_auxiliary_request(config: &Value, max_timeout: Duration, secrets: &[String]) -> String {
    use reqwest::blocking::Client;
    use reqwest::redirect::Policy;

    let raw_url = config.get("url").and_then(Value::as_str).unwrap_or("").trim();
    let url = match reqwest::Url::parse(raw_url) {
        Ok(url) if matches!(url.scheme(), "http" | "https") && url.host_str().is_some() => url,
        _ => return json!({ "error": "pb.sendRequest requires a valid URL with http or https scheme" }).to_string(),
    };
    if blocked_metadata_host(url.host_str().unwrap_or("")) {
        return json!({ "error": format!("security: requests to cloud metadata service {:?} are blocked", url.host_str().unwrap_or("")) }).to_string();
    }
    let requested_timeout = config.get("timeoutMs").and_then(Value::as_u64).filter(|value| *value > 0)
        .map(Duration::from_millis).unwrap_or(max_timeout);
    let timeout = if max_timeout.is_zero() { DEFAULT_TIMEOUT } else { max_timeout.min(requested_timeout) };
    let redirect = Policy::custom(|attempt| {
        if attempt.previous().len() >= 3 { return attempt.stop(); }
        if attempt.previous().last().is_some_and(|previous| previous.scheme() == "https" && attempt.url().scheme() == "http") {
            return attempt.stop();
        }
        if blocked_metadata_host(attempt.url().host_str().unwrap_or("")) { return attempt.stop(); }
        attempt.follow()
    });
    let client = match Client::builder().timeout(timeout).redirect(redirect)
        .danger_accept_invalid_certs(config.get("insecureSkipVerify").and_then(Value::as_bool).unwrap_or(false)).build() {
        Ok(client) => client,
        Err(error) => return masked_error(format!("could not create auxiliary client: {error}"), secrets),
    };
    let method_text = config.get("method").and_then(Value::as_str).unwrap_or("GET").to_ascii_uppercase();
    let method = match reqwest::Method::from_bytes(method_text.as_bytes()) {
        Ok(method) => method,
        Err(error) => return masked_error(format!("invalid auxiliary request method: {error}"), secrets),
    };
    let mut request = client.request(method, url);
    if let Some(headers) = config.get("headers").and_then(Value::as_object) {
        for (name, value) in headers {
            if let Some(value) = value.as_str() {
                request = request.header(name.as_str(), value);
            } else {
                request = request.header(name.as_str(), value.to_string());
            }
        }
    }
    if let Some(body) = config.get("body").filter(|value| !value.is_null()) {
        let body = body.as_str().map(str::to_owned).unwrap_or_else(|| body.to_string());
        request = request.body(body);
    }
    let started = Instant::now();
    let response = match request.send() {
        Ok(response) => response,
        Err(error) => return masked_error(format!("auxiliary request failed: {error}"), secrets),
    };
    let status = response.status();
    let mut headers = serde_json::Map::new();
    for (name, value) in response.headers() {
        headers.entry(name.to_string()).or_insert_with(|| json!(value.to_str().unwrap_or("")));
    }
    let mut body = Vec::new();
    let read_result = response.take(10 * 1024 * 1024).read_to_end(&mut body);
    if let Err(error) = read_result { return masked_error(format!("failed to read auxiliary response: {error}"), secrets); }
    json!({
        "status": status.as_u16(), "statusCode": status.as_u16(), "statusText": status.canonical_reason().unwrap_or(""),
        "duration": started.elapsed().as_millis(), "time": started.elapsed().as_millis(),
        "body": String::from_utf8_lossy(&body), "headers": headers,
    }).to_string()
}

fn blocked_metadata_host(host: &str) -> bool {
    matches!(host.trim_matches(['[', ']']).to_ascii_lowercase().as_str(), "169.254.169.254" | "instance-data" | "metadata.google.internal" | "100.100.100.200")
}

fn masked_error(error: String, secrets: &[String]) -> String {
    json!({ "error": pebblepost_core::security::mask_secrets(&error, secrets) }).to_string()
}

fn hex(bytes: &[u8]) -> String {
    const DIGITS: &[u8; 16] = b"0123456789abcdef";
    let mut output = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        output.push(DIGITS[(byte >> 4) as usize] as char);
        output.push(DIGITS[(byte & 15) as usize] as char);
    }
    output
}

const BOOTSTRAP: &str = r#"
const __pp_state = globalThis.__pp_state;
const __pp_consoleLog = (level, args) => {
  const message = args.map(value => value === undefined ? "undefined" : value === null ? "null" : String(value)).join(" ");
  if (__pp_state.logs.length < globalThis.__pp_max_logs) {
    const clipped = message.length > globalThis.__pp_max_log_length ? message.slice(0, globalThis.__pp_max_log_length) + "…[truncated]" : message;
    __pp_state.logs.push("[" + new Date().toISOString().slice(11, 23) + "] [" + __pp_state.source + "] [" + level.toUpperCase() + "] " + clipped);
    __pp_state.consoleLogs.push({ timestamp: new Date().toISOString(), level, source: __pp_state.source, message: clipped });
  }
};
globalThis.console = {
  log: (...args) => __pp_consoleLog("log", args),
  info: (...args) => __pp_consoleLog("info", args),
  warn: (...args) => __pp_consoleLog("warn", args),
  error: (...args) => __pp_consoleLog("error", args)
};
globalThis.__pp_record_error = error => {
  const message = error && error.stack ? String(error.stack) : String(error);
  __pp_state.error = message;
  __pp_consoleLog("error", [message]);
  if (__pp_state.phase === "post") __pp_state.tests.push({ name: __pp_state.source + " Script Error", passed: false, message });
};
const __pp_request = __pp_state.request || (__pp_state.request = {});
if (!Array.isArray(__pp_request.headers)) __pp_request.headers = [];
if (!__pp_request.body || typeof __pp_request.body !== "object") __pp_request.body = {};
const __pp_headers = {
  add(key, value) { __pp_request.headers.push({ key: String(key), value: String(value), enabled: true }); },
  set(key, value) {
    const header = __pp_request.headers.find(item => String(item.key).toLowerCase() === String(key).toLowerCase());
    if (header) { header.value = String(value); header.enabled = true; }
    else this.add(key, value);
  },
  get(key) { const header = __pp_request.headers.find(item => item.enabled !== false && String(item.key).toLowerCase() === String(key).toLowerCase()); return header ? String(header.value ?? "") : ""; },
  remove(key) { __pp_request.headers = __pp_request.headers.filter(item => String(item.key).toLowerCase() !== String(key).toLowerCase()); }
};
const __pp_body = {};
Object.defineProperties(__pp_body, {
  raw: { enumerable: true, get() { return __pp_request.body.raw || ""; }, set(value) { __pp_request.body.raw = String(value); } },
  setRaw: { enumerable: true, value(value) { __pp_request.body.raw = String(value); } }
});
const __pp_env = __pp_state.envVars || (__pp_state.envVars = {});
const __pp_local = __pp_state.localVars || (__pp_state.localVars = {});
const __pp_environment = {
  get(key) { return Object.prototype.hasOwnProperty.call(__pp_env, key) ? String(__pp_env[key]) : ""; },
  set(key, value) { __pp_env[String(key)] = String(value); },
  has(key) { return Object.prototype.hasOwnProperty.call(__pp_env, key); },
  unset(key) { delete __pp_env[key]; },
  toObject() { return { ...__pp_env }; }
};
const __pp_variables = {
  get(key) { if (Object.prototype.hasOwnProperty.call(__pp_local, key)) return String(__pp_local[key]); return __pp_environment.get(key); },
  set(key, value) { __pp_local[String(key)] = String(value); },
  has(key) { return Object.prototype.hasOwnProperty.call(__pp_local, key) || __pp_environment.has(key); },
  unset(key) { delete __pp_local[key]; },
  toObject() { return { ...__pp_env, ...__pp_local }; }
};
const __pp_action = { nextRequest: null, stopAll: false };
const __pp_runner = {
  setNextRequest(value) { __pp_action.nextRequest = value == null ? "" : String(value).trim(); __pp_action.nextRequestSet = true; },
  stop() { __pp_action.stopAll = true; }
};
const __pp_data = { ...(__pp_state.dataRow && typeof __pp_state.dataRow === "object" ? __pp_state.dataRow : {}) };
__pp_data.get = key => Object.prototype.hasOwnProperty.call(__pp_state.dataRow || {}, key) ? __pp_state.dataRow[key] : undefined;
__pp_data.toObject = () => ({ ...(__pp_state.dataRow || {}) });
const __pp_info = { iteration: __pp_state.iteration || 0, iterationCount: __pp_state.iterationCount || 1 };
const __pp_date = {
  now: () => Date.now(),
  nowISO: () => new Date().toISOString(),
  format(value, pattern) {
    const date = value == null || value === "" || value === "now" ? new Date() : new Date(value);
    if (Number.isNaN(date.getTime())) throw new Error("Date format failed: invalid date value");
    const pad = (number, width = 2) => String(number).padStart(width, "0");
    const parts = { YYYY: String(date.getUTCFullYear()), YY: pad(date.getUTCFullYear() % 100), MM: pad(date.getUTCMonth() + 1), DD: pad(date.getUTCDate()), HH: pad(date.getUTCHours()), mm: pad(date.getUTCMinutes()), ss: pad(date.getUTCSeconds()), SSS: pad(date.getUTCMilliseconds(), 3) };
    return String(pattern || "YYYY-MM-DDTHH:mm:ssZ").replace(/YYYY|SSS|YY|MM|DD|HH|mm|ss/g, token => parts[token]);
  },
  add(value, amount, unit) {
    const date = value == null || value === "" || value === "now" ? new Date() : new Date(value);
    const factors = { ms: 1, millisecond: 1, milliseconds: 1, s: 1000, sec: 1000, second: 1000, seconds: 1000, m: 60000, min: 60000, minute: 60000, minutes: 60000, h: 3600000, hr: 3600000, hour: 3600000, hours: 3600000, d: 86400000, day: 86400000, days: 86400000 };
    const factor = factors[String(unit).trim().toLowerCase()];
    if (!factor) throw new Error("Date add failed: unknown date unit '" + unit + "'");
    date.setTime(date.getTime() + Number(amount) * factor);
    return date.toISOString();
  }
};
const __pp_hmac_call = (algorithm, secret, message) => {
  const result = JSON.parse(__pp_native_hmac(String(algorithm), String(secret), String(message)));
  if (result.error) throw new Error(result.error);
  return result.value;
};
const __pp_crypto = {
  md5: __pp_md5, sha256: __pp_sha256, sha512: __pp_sha512,
  hmac: (algorithm, secret, message) => __pp_hmac_call(algorithm, secret, message),
  uuid: __pp_uuid, uuidv4: __pp_uuid,
  base64Encode: __pp_base64_encode,
  base64Decode(value) { const result = JSON.parse(__pp_base64_decode(String(value))); if (result.error) throw new Error(result.error); return result.value; }
};
const __pp_base64 = {
  encode: __pp_base64_encode,
  decode(value) { const result = JSON.parse(__pp_base64_decode(String(value))); if (result.error) throw new Error(result.error); return result.value; }
};
const __pp_hash = { sha256: __pp_sha256, md5: __pp_md5 };
const __pp_hmac = { sha256: (secret, message) => __pp_hmac_call("sha256", secret, message) };
const __pp_jwt = { decode(token) { const result = JSON.parse(__pp_jwt_decode(String(token))); if (result.error) throw new Error(result.error); return result; } };
const __pp_random = {
  int: (min, max) => __pp_random_int(Number(min), Number(max)),
  string: (length, charset) => __pp_random_string(Number(length), String(charset || ""))
};
const __pp_pb = {
  environment: __pp_environment, variables: __pp_variables,
  crypto: __pp_crypto, expect: globalThis.expect, runner: __pp_runner,
  data: __pp_data, iterationData: __pp_data, info: __pp_info,
  uuid: __pp_uuid, base64: __pp_base64, hash: __pp_hash, hmac: __pp_hmac,
  jwt: __pp_jwt, date: __pp_date, random: __pp_random,
  request: {
    get url() { return __pp_request.url || ""; }, set url(value) { __pp_request.url = String(value); },
    get method() { return __pp_request.method || ""; }, set method(value) { __pp_request.method = String(value); },
    headers: __pp_headers, body: __pp_body
  }
};
if (__pp_state.phase === "post") {
  const response = __pp_state.response || {};
  const responseHeaders = {};
  for (const [name, value] of Object.entries(response.headers || {})) {
    const normalized = Array.isArray(value) ? (value[0] || "") : value;
    responseHeaders[name] = normalized;
    responseHeaders[name.toLowerCase()] = normalized;
  }
  responseHeaders.get = key => { const pair = Object.entries(response.headers || {}).find(([name]) => name.toLowerCase() === String(key).toLowerCase()); return pair ? (Array.isArray(pair[1]) ? pair[1][0] : pair[1]) : ""; };
  responseHeaders.has = key => Object.keys(response.headers || {}).some(name => name.toLowerCase() === String(key).toLowerCase());
  const responseBody = String(response.body || "");
  __pp_pb.response = {
    code: response.statusCode || 0, status: response.statusCode || 0, statusText: response.statusText || "",
    responseTime: response.timing?.totalDurationMs || 0, time: response.timing?.totalDurationMs || 0,
    duration: response.timing?.totalDurationMs || 0, size: response.size || 0, body: responseBody,
    headers: responseHeaders, text: () => responseBody,
    json() {
      if (responseBody) {
        try { return JSON.parse(responseBody); } catch (_) {}
      }
      const streamLogs = response.streamLogs || [];
      for (let index = streamLogs.length - 1; index >= 0; index--) {
        const entry = streamLogs[index];
        if (entry.direction !== "receive") continue;
        try { return JSON.parse(entry.payload); } catch (_) {}
      }
      if (!responseBody) return null;
      try { return JSON.parse(responseBody); }
      catch (error) { throw new Error("Failed to parse response body as JSON: " + error); }
    }
  };
  const received = (response.streamLogs || []).filter(entry => entry.direction === "receive");
  __pp_pb.stream = {
    messages: () => received.map(entry => entry.payload),
    jsonMessages: () => received.map(entry => { try { return JSON.parse(entry.payload); } catch (_) { return entry.payload; } }),
    logs: () => response.streamLogs || [],
    closeCode: () => response.streamCloseCode || 0,
    closeReason: () => response.streamCloseReason || "",
    evicted: () => response.streamEvicted || 0,
    waitFor(predicate) { return typeof predicate === "function" && received.some(entry => { let value; try { value = JSON.parse(entry.payload); } catch (_) { value = entry.payload; } try { return Boolean(predicate(value)); } catch (_) { return false; } }); }
  };
  __pp_pb.test = (name, fn) => {
    try { if (typeof fn !== "function") throw new Error("Test function cannot be null"); fn(); __pp_state.tests.push({ name: String(name), passed: true }); }
    catch (error) { __pp_state.tests.push({ name: String(name), passed: false, message: String(error?.message || error) }); }
  };
}
__pp_pb.sendRequest = config => {
  const normalized = typeof config === "string" ? { url: config } : config;
  if (!normalized || typeof normalized !== "object") throw new Error("pb.sendRequest config must be an object or URL string");
  const result = JSON.parse(__pp_send_request(JSON.stringify(normalized)));
  if (result.error) throw new Error("pb.sendRequest failed: " + result.error);
  result.text = () => String(result.body ?? "");
  result.json = () => { try { return JSON.parse(String(result.body)); } catch (error) { throw new Error("Failed to parse auxiliary response body as JSON: " + error); } };
  return result;
};
globalThis.pb = __pp_pb;
globalThis.pm = __pp_pb;
globalThis.postman = { setNextRequest: __pp_runner.setNextRequest };
globalThis.__pp_collect_state = () => {
  __pp_state.localVars = __pp_local;
  __pp_state.extractedEnvVars = __pp_env;
  __pp_state.nextRequest = __pp_action.nextRequest;
  __pp_state.nextRequestSet = __pp_action.nextRequestSet;
  __pp_state.stopAll = __pp_action.stopAll;
};
globalThis.__pp_record_error = error => {
  const message = error && error.stack ? String(error.stack) : String(error);
  __pp_state.error = message;
  __pp_consoleLog("error", [message]);
  if (__pp_state.phase === "post") __pp_state.tests.push({ name: __pp_state.source + " Script Error", passed: false, message });
};
"#;

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    fn run(phase: ScriptPhase, script: &str, request: &Value, response: Option<&Value>) -> Value {
        let variables = HashMap::from([("BASE_URL".to_owned(), "https://example.test".to_owned())]);
        execute(ScriptInput {
            phase,
            source: "unit-test",
            script,
            request,
            response,
            variables: &variables,
            data_row: None,
            iteration: 0,
            iteration_count: 1,
            timeout: Duration::from_secs(1),
            secrets: &[],
        }).unwrap()
    }

    #[test]
    fn crypto_helpers_match_standard_vectors() {
        assert_eq!(native_md5("hello world".into()), "5eb63bbbe01eeed093cb22bb8f5acdc3");
        assert_eq!(native_sha256("hello world".into()), "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9");
        let hmac: Value = serde_json::from_str(&native_hmac(
            "sha256".into(), "key".into(), "The quick brown fox jumps over the lazy dog".into(),
        )).unwrap();
        assert_eq!(hmac["value"], "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8");
        assert!(serde_json::from_str::<Value>(&native_hmac("unknown".into(), "key".into(), "message".into())).unwrap()["error"].is_string());
        assert_eq!(native_base64_decode("aGVsbG8=".into()), json!({ "value": "hello" }).to_string());
    }

    #[test]
    fn syntax_validation_compiles_without_running_scripts() {
        assert!(validate_syntax("syntax", "globalThis.__ran = true; const n = 1;").is_ok());
        let error = validate_syntax("syntax", "function ( {").unwrap_err();
        assert!(error.contains("[syntax] Syntax error"));
    }

    #[test]
    fn pre_request_context_mutates_request_and_captures_logs() {
        let request = json!({
            "method": "GET", "url": "https://example.test/items",
            "headers": [], "body": { "raw": "before" }
        });
        let output = run(ScriptPhase::PreRequest, r#"
            pm.request.method = "POST";
            pm.request.headers.set("X-Signed", pm.crypto.sha256("payload"));
            pm.request.body.raw = "after";
            pm.environment.set("RUN_ID", "abc123");
            pm.variables.set("LOCAL_ONLY", "yes");
            console.log("method", pm.request.method);
        "#, &request, None);
        assert_eq!(output.pointer("/request/method").and_then(Value::as_str), Some("POST"));
        assert_eq!(output.pointer("/request/body/raw").and_then(Value::as_str), Some("after"));
        assert_eq!(output.pointer("/request/headers/0/key").and_then(Value::as_str), Some("X-Signed"));
        assert_eq!(output.pointer("/extractedEnvVars/RUN_ID").and_then(Value::as_str), Some("abc123"));
        assert_eq!(output.pointer("/localVars/LOCAL_ONLY").and_then(Value::as_str), Some("yes"));
        assert_eq!(output.pointer("/consoleLogs/0/level").and_then(Value::as_str), Some("log"));
    }

    #[test]
    fn post_response_context_runs_assertion_aliases_and_captures_output() {
        let request = json!({ "method": "GET", "url": "https://example.test", "headers": [], "body": {} });
        let response = json!({
            "statusCode": 200, "statusText": "OK", "body": "{\"id\":7}",
            "headers": { "content-type": ["application/json"] },
            "timing": { "totalDurationMs": 12.0 }
        });
        let output = run(ScriptPhase::PostResponse, r#"
            pm.test("Jest and Chai assertions", () => {
                expect(pm.response.status).toBe(200);
                expect(pm.response.json()).toEqual({ id: 7 });
                expect(pm.response.json().missing).toBeUndefined();
                expect(null).toBeNull();
                expect(["one", "two"]).toContain("two");
                expect("hello").toHaveLength(5);
                pm.expect(pm.response).to.have.status(200);
            });
            console.info("response status", pm.response.status);
        "#, &request, Some(&response));
        assert_eq!(output.pointer("/tests/0/passed").and_then(Value::as_bool), Some(true));
        assert_eq!(output.pointer("/consoleLogs/0/level").and_then(Value::as_str), Some("info"));
        assert_eq!(output.pointer("/consoleLogs/0/source").and_then(Value::as_str), Some("unit-test"));
    }

    #[test]
    fn sandbox_blocks_node_globals_and_reports_assertion_failures() {
        let request = json!({ "method": "GET", "url": "https://example.test", "headers": [], "body": {} });
        for script in ["require('fs')", "process.exit(0)", "fetch('https://example.test')"] {
            let result = execute(ScriptInput {
                phase: ScriptPhase::PreRequest, source: "sandbox-test", script, request: &request,
                response: None, variables: &HashMap::new(), data_row: None, iteration: 0,
                iteration_count: 1, timeout: Duration::from_secs(1), secrets: &[],
            }).unwrap();
            assert!(result["error"].as_str().is_some_and(|error| !error.is_empty()), "sandbox accepted {script:?}");
        }
        let response = json!({ "statusCode": 200, "body": "{}", "headers": {} });
        let output = run(ScriptPhase::PostResponse, r#"
            pm.test("bad equality", () => expect(pm.response.status).toBe(404));
            pm.test("bad deep equality", () => expect({a: 1}).toEqual({a: 2}));
            pm.test("bad inclusion", () => expect([1, 2]).toContain(3));
        "#, &request, Some(&response));
        let tests = output["tests"].as_array().unwrap();
        assert_eq!(tests.len(), 3);
        assert!(tests.iter().all(|test| test["passed"] == false));
        assert!(tests[0]["message"].as_str().unwrap().contains("404"));
    }

    #[test]
    fn sandbox_interrupts_infinite_loops_and_caps_console_output() {
        let request = json!({ "method": "GET", "url": "https://example.test", "headers": [], "body": {} });
        let started = std::time::Instant::now();
        let error = execute(ScriptInput {
            phase: ScriptPhase::PreRequest, source: "timeout", script: "while (true) {}", request: &request,
            response: None, variables: &HashMap::new(), data_row: None, iteration: 0,
            iteration_count: 1, timeout: Duration::from_millis(50), secrets: &[],
        }).unwrap_err();
        assert!(error.to_ascii_lowercase().contains("timed out"), "unexpected error: {error}");
        assert!(started.elapsed() < Duration::from_secs(2));

        let logs = (0..560).map(|index| format!("console.log('line {index}');")).collect::<Vec<_>>().join("\n");
        let capped = run(ScriptPhase::PreRequest, &logs, &request, None);
        assert!(capped["consoleLogs"].as_array().unwrap().len() <= MAX_LOGS);
        let long = run(ScriptPhase::PreRequest, "console.log('x'.repeat(10000));", &request, None);
        assert!(long["consoleLogs"][0]["message"].as_str().unwrap().len() <= MAX_LOG_LENGTH + 20);
    }

    #[test]
    fn console_levels_keep_source_and_emit_structured_entries() {
        let request = json!({ "method": "GET", "url": "https://example.test", "headers": [], "body": {} });
        let output = execute(ScriptInput {
            phase: ScriptPhase::PreRequest, source: "CustomScript",
            script: "console.log('log', 123); console.info('info'); console.warn('warn'); console.error('error');",
            request: &request, response: None, variables: &HashMap::new(), data_row: None,
            iteration: 0, iteration_count: 1, timeout: Duration::from_secs(1), secrets: &[],
        }).unwrap();
        let logs = output["consoleLogs"].as_array().unwrap();
        assert_eq!(logs.iter().map(|entry| entry["level"].as_str().unwrap()).collect::<Vec<_>>(), ["log", "info", "warn", "error"]);
        assert!(logs.iter().all(|entry| entry["source"] == "CustomScript" && entry["timestamp"].is_string()));
    }
}
