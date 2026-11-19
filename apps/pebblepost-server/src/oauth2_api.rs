use std::collections::HashMap;
use std::net::{Ipv4Addr, SocketAddr};
use std::path::Path;
use std::process::Command;
use std::sync::{Arc, Mutex, OnceLock};
use std::time::Duration;

use axum::extract::{Query, State};
use axum::http::{HeaderMap, StatusCode};
use axum::response::{Html, IntoResponse, Response};
use axum::routing::get;
use axum::{Json, Router};
use base64::Engine;
use pebblepost_core::{build_variable_map, interpolate_value, list_environments};
use serde_json::{Value, json};
use sha2::{Digest, Sha256};
use tokio::net::TcpListener;
use tokio::sync::oneshot;

struct CallbackState {
    expected_state: String,
    expected_host: String,
    code_sender: Mutex<Option<oneshot::Sender<Result<String, String>>>>,
    shutdown_sender: Mutex<Option<oneshot::Sender<()>>>,
}

static TOKEN_CACHE: OnceLock<Mutex<HashMap<String, Value>>> = OnceLock::new();

pub(super) async fn get_token(auth: &Value) -> Result<Value, String> {
    let key = token_cache_key(auth);
    let cached = token_cache()
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner)
        .get(&key)
        .cloned();
    let now = chrono::Utc::now().timestamp();
    if let Some(token) = cached.as_ref().filter(|token| token_is_valid(token, now)) {
        return Ok(token.clone());
    }

    let client_id = auth.get("clientId").and_then(Value::as_str).unwrap_or("");
    let client_secret = auth
        .get("clientSecret")
        .and_then(Value::as_str)
        .unwrap_or("");
    let token_url = auth.get("tokenUrl").and_then(Value::as_str).unwrap_or("");
    if let Some(refresh_token) = cached
        .as_ref()
        .and_then(|token| token.get("refresh_token"))
        .and_then(Value::as_str)
        .filter(|value| !value.is_empty())
        && let Ok(token) = request_token(
            token_url,
            client_id,
            client_secret,
            "refresh_token",
            Some(refresh_token),
            None,
        )
        .await
    {
        cache_token(&key, token.clone());
        return Ok(token);
    }

    let static_token = auth.get("token").and_then(Value::as_str).unwrap_or("");
    let static_expires_at = auth
        .get("tokenExpiresAt")
        .and_then(Value::as_i64)
        .unwrap_or(0);
    if !static_token.is_empty() && (static_expires_at == 0 || static_expires_at > now + 30) {
        let token = json!({
            "access_token": static_token,
            "refresh_token": auth.get("refreshToken").and_then(Value::as_str).unwrap_or(""),
            "expires_at": static_expires_at
        });
        cache_token(&key, token.clone());
        return Ok(token);
    }

    let grant_type = auth
        .get("grantType")
        .and_then(Value::as_str)
        .unwrap_or("")
        .trim()
        .to_ascii_lowercase();
    if let Some(refresh_token) = auth
        .get("refreshToken")
        .and_then(Value::as_str)
        .filter(|value| !value.is_empty())
        && let Ok(token) = request_token(
            token_url,
            client_id,
            client_secret,
            "refresh_token",
            Some(refresh_token),
            None,
        )
        .await
    {
        cache_token(&key, token.clone());
        return Ok(token);
    }
    if grant_type.is_empty() || grant_type == "client_credentials" {
        let scope = auth.get("scope").and_then(Value::as_str).unwrap_or("");
        let token = request_token(
            token_url,
            client_id,
            client_secret,
            "client_credentials",
            None,
            Some(scope),
        )
        .await?;
        cache_token(&key, token.clone());
        return Ok(token);
    }
    Err("no valid OAuth2 access token available; authorization code flow requires interactive login".into())
}

fn token_cache() -> &'static Mutex<HashMap<String, Value>> {
    TOKEN_CACHE.get_or_init(|| Mutex::new(HashMap::new()))
}

fn token_cache_key(auth: &Value) -> String {
    let grant = auth
        .get("grantType")
        .and_then(Value::as_str)
        .unwrap_or("")
        .trim()
        .to_ascii_lowercase();
    let grant = if grant.is_empty() {
        "client_credentials"
    } else {
        grant.as_str()
    };
    format!(
        "{}|{}|{}|{}",
        auth.get("tokenUrl").and_then(Value::as_str).unwrap_or(""),
        auth.get("clientId").and_then(Value::as_str).unwrap_or(""),
        grant,
        auth.get("scope").and_then(Value::as_str).unwrap_or("")
    )
}

fn token_is_valid(token: &Value, now: i64) -> bool {
    token
        .get("access_token")
        .and_then(Value::as_str)
        .is_some_and(|value| !value.is_empty())
        && token
            .get("expires_at")
            .and_then(Value::as_i64)
            .is_none_or(|expires_at| expires_at == 0 || expires_at > now + 30)
}

fn cache_token(key: &str, token: Value) {
    token_cache()
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner)
        .insert(key.to_owned(), token);
}

fn cache_auth_token(auth: &Value, token: Value) {
    cache_token(&token_cache_key(auth), token);
}

async fn request_token(
    token_url: &str,
    client_id: &str,
    client_secret: &str,
    grant_type: &str,
    refresh_token: Option<&str>,
    scope: Option<&str>,
) -> Result<Value, String> {
    if token_url.trim().is_empty() {
        return Err("OAuth2 token URL is required".into());
    }
    let mut fields = vec![("grant_type", grant_type.to_owned())];
    if let Some(refresh_token) = refresh_token.filter(|value| !value.trim().is_empty()) {
        fields.push(("refresh_token", refresh_token.to_owned()));
    }
    if !client_id.is_empty() {
        fields.push(("client_id", client_id.to_owned()));
    }
    if !client_secret.is_empty() {
        fields.push(("client_secret", client_secret.to_owned()));
    }
    if let Some(scope) = scope.filter(|value| !value.is_empty()) {
        fields.push(("scope", scope.to_owned()));
    }
    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(30))
        .build()
        .map_err(|error| error.to_string())?;
    let response = client
        .post(token_url)
        .form(&fields)
        .header(reqwest::header::ACCEPT, "application/json")
        .send()
        .await
        .map_err(|error| format!("token request failed: {error}"))?;
    let status = response.status();
    let body = response
        .text()
        .await
        .map_err(|error| format!("could not read token response: {error}"))?;
    if !status.is_success() {
        return Err(format!("token request returned status {status}: {body}"));
    }
    let mut token: Value = serde_json::from_str(&body)
        .map_err(|error| format!("failed to parse token response: {error}"))?;
    if grant_type == "refresh_token"
        && token
            .get("refresh_token")
            .and_then(Value::as_str)
            .is_none_or(str::is_empty)
        && let Some(refresh_token) = refresh_token
    {
        token["refresh_token"] = json!(refresh_token);
    }
    if let Some(expires_in) = token
        .get("expires_in")
        .and_then(Value::as_i64)
        .filter(|value| *value > 0)
    {
        token["expires_at"] = json!(chrono::Utc::now().timestamp().saturating_add(expires_in));
    }
    Ok(token)
}

pub(super) async fn authorize(Json(payload): Json<Value>) -> Response {
    match authorize_code_pkce(payload).await {
        Ok(token) => Json(token).into_response(),
        Err(error) => (StatusCode::BAD_GATEWAY, Json(json!({ "error": error }))).into_response(),
    }
}

pub(super) fn resolve_auth(payload: &Value) -> Result<Value, String> {
    let mut auth = payload.get("auth").cloned().unwrap_or(Value::Null);
    let environments = payload
        .get("workspacePath")
        .and_then(Value::as_str)
        .map(|path| list_environments(Path::new(path)))
        .transpose()
        .map_err(|error| error.to_string())?
        .unwrap_or_default();
    let environment_name = payload
        .get("environmentName")
        .and_then(Value::as_str)
        .unwrap_or("");
    let environment = environments
        .iter()
        .find(|environment| !environment_name.is_empty() && environment.name == environment_name);
    let variables = build_variable_map(environment, &HashMap::new());
    interpolate_value(&mut auth, &variables)?;
    Ok(auth)
}

async fn authorize_code_pkce(payload: Value) -> Result<Value, String> {
    let auth = resolve_auth(&payload)?;
    let auth_url = auth
        .get("authUrl")
        .and_then(Value::as_str)
        .filter(|value| !value.trim().is_empty())
        .ok_or_else(|| "OAuth2 authorization URL is required".to_owned())?;
    let token_url = auth
        .get("tokenUrl")
        .and_then(Value::as_str)
        .filter(|value| !value.trim().is_empty())
        .ok_or_else(|| "OAuth2 token URL is required".to_owned())?;
    let client_id = auth
        .get("clientId")
        .and_then(Value::as_str)
        .filter(|value| !value.is_empty())
        .ok_or_else(|| "OAuth2 client ID is required".to_owned())?;
    let mut authorization_url = reqwest::Url::parse(auth_url)
        .map_err(|error| format!("invalid authorization URL: {error}"))?;
    if !matches!(authorization_url.scheme(), "http" | "https") {
        return Err("OAuth2 authorization URL must use HTTP or HTTPS".into());
    }
    let parsed_token_url =
        reqwest::Url::parse(token_url).map_err(|error| format!("invalid token URL: {error}"))?;
    if !matches!(parsed_token_url.scheme(), "http" | "https") {
        return Err("OAuth2 token URL must use HTTP or HTTPS".into());
    }

    let verifier = random_url_token(32)?;
    let state_value = random_url_token(32)?;
    let challenge = base64::engine::general_purpose::URL_SAFE_NO_PAD
        .encode(Sha256::digest(verifier.as_bytes()));
    let listener = TcpListener::bind(SocketAddr::from((Ipv4Addr::LOCALHOST, 0)))
        .await
        .map_err(|error| format!("could not start the OAuth2 callback listener: {error}"))?;
    let address = listener.local_addr().map_err(|error| error.to_string())?;
    let redirect_uri = format!("http://127.0.0.1:{}/callback", address.port());
    let (code_sender, code_receiver) = oneshot::channel();
    let (shutdown_sender, shutdown_receiver) = oneshot::channel();
    let callback_state = Arc::new(CallbackState {
        expected_state: state_value.clone(),
        expected_host: format!("127.0.0.1:{}", address.port()),
        code_sender: Mutex::new(Some(code_sender)),
        shutdown_sender: Mutex::new(Some(shutdown_sender)),
    });
    let callback_router = Router::new()
        .route("/callback", get(callback))
        .with_state(callback_state);
    let server_task = tokio::spawn(async move {
        axum::serve(listener, callback_router)
            .with_graceful_shutdown(async {
                let _ = shutdown_receiver.await;
            })
            .await
    });

    let scope = auth
        .get("scope")
        .and_then(Value::as_str)
        .filter(|scope| !scope.trim().is_empty());
    {
        // Replace values managed by this flow so a configured URL cannot
        // override the state, redirect target, or PKCE challenge we created.
        let managed = [
            "response_type",
            "client_id",
            "redirect_uri",
            "code_challenge",
            "code_challenge_method",
            "state",
        ];
        let existing = authorization_url
            .query_pairs()
            .filter(|(key, _)| {
                !managed.contains(&key.as_ref()) && !(key == "scope" && scope.is_some())
            })
            .map(|(key, value)| (key.into_owned(), value.into_owned()))
            .collect::<Vec<_>>();
        authorization_url.set_query(None);
        let mut query = authorization_url.query_pairs_mut();
        for (key, value) in existing {
            query.append_pair(&key, &value);
        }
        query.append_pair("response_type", "code");
        query.append_pair("client_id", client_id);
        query.append_pair("redirect_uri", &redirect_uri);
        query.append_pair("code_challenge", &challenge);
        query.append_pair("code_challenge_method", "S256");
        query.append_pair("state", &state_value);
        if let Some(scope) = scope {
            query.append_pair("scope", scope);
        }
    }
    let browser_url = authorization_url.to_string();
    if let Err(error) = open_browser(&browser_url) {
        server_task.abort();
        return Err(format!(
            "could not open a browser ({error}); open this URL manually: {browser_url}"
        ));
    }

    let code = match tokio::time::timeout(Duration::from_secs(300), code_receiver).await {
        Ok(Ok(Ok(code))) => code,
        Ok(Ok(Err(error))) => {
            let _ = server_task.await;
            return Err(error);
        }
        Ok(Err(_)) => {
            let _ = server_task.await;
            return Err("OAuth2 callback ended before returning an authorization code".into());
        }
        Err(_) => {
            server_task.abort();
            return Err("OAuth2 authorization timed out after five minutes".into());
        }
    };
    let _ = server_task.await;

    let mut fields = vec![
        ("grant_type", "authorization_code".to_owned()),
        ("code", code),
        ("redirect_uri", redirect_uri),
        ("code_verifier", verifier),
        ("client_id", client_id.to_owned()),
    ];
    if let Some(secret) = auth
        .get("clientSecret")
        .and_then(Value::as_str)
        .filter(|secret| !secret.is_empty())
    {
        fields.push(("client_secret", secret.to_owned()));
    }
    let response = reqwest::Client::builder()
        .timeout(Duration::from_secs(30))
        .build()
        .map_err(|error| format!("could not create OAuth2 client: {error}"))?
        .post(token_url)
        .form(&fields)
        .header(reqwest::header::ACCEPT, "application/json")
        .send()
        .await
        .map_err(|error| format!("OAuth2 token exchange failed: {error}"))?;
    let status = response.status();
    let body = response
        .text()
        .await
        .map_err(|error| format!("could not read OAuth2 token response: {error}"))?;
    let mut token: Value = serde_json::from_str(&body)
        .map_err(|error| format!("OAuth2 token response was not JSON: {error}"))?;
    if !status.is_success() {
        let detail = token
            .get("error_description")
            .or_else(|| token.get("error"))
            .and_then(Value::as_str)
            .unwrap_or("token endpoint rejected the authorization code");
        return Err(format!(
            "OAuth2 token endpoint returned HTTP {status}: {detail}"
        ));
    }
    if let Some(expires_in) = token
        .get("expires_in")
        .and_then(Value::as_i64)
        .filter(|value| *value > 0)
    {
        token["expires_at"] = json!(chrono::Utc::now().timestamp().saturating_add(expires_in));
    }
    cache_auth_token(&auth, token.clone());
    Ok(token)
}

async fn callback(
    State(state): State<Arc<CallbackState>>,
    headers: HeaderMap,
    Query(query): Query<HashMap<String, String>>,
) -> Response {
    let host = headers
        .get(axum::http::header::HOST)
        .and_then(|value| value.to_str().ok())
        .unwrap_or("");
    let port = state.expected_host.rsplit(':').next().unwrap_or("");
    let localhost_host = format!("localhost:{port}");
    let allowed_host = host == state.expected_host || host.eq_ignore_ascii_case(&localhost_host);
    if !allowed_host {
        return (StatusCode::BAD_REQUEST, "Invalid Host header").into_response();
    }
    let incoming_state = query.get("state").map(String::as_str).unwrap_or("");
    if !constant_time_equal(incoming_state.as_bytes(), state.expected_state.as_bytes()) {
        return (StatusCode::BAD_REQUEST, "OAuth state validation failed").into_response();
    }
    let result = if let Some(error) = query.get("error") {
        let description = query
            .get("error_description")
            .map(String::as_str)
            .unwrap_or("");
        Err(format!("OAuth2 provider returned {error}: {description}"))
    } else if let Some(code) = query.get("code").filter(|code| !code.is_empty()) {
        Ok(code.clone())
    } else {
        Err("OAuth2 callback did not contain an authorization code".into())
    };
    if let Some(sender) = state
        .code_sender
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner)
        .take()
    {
        let _ = sender.send(result.clone());
    }
    if let Some(sender) = state
        .shutdown_sender
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner)
        .take()
    {
        let _ = sender.send(());
    }
    match result {
        Ok(_) => Html("<!doctype html><html><head><title>PebblePost</title></head><body><h1>Authorization complete</h1><p>You can close this window and return to PebblePost.</p></body></html>").into_response(),
        Err(error) => (StatusCode::BAD_REQUEST, error).into_response(),
    }
}

fn random_url_token(length: usize) -> Result<String, String> {
    let mut bytes = vec![0u8; length];
    getrandom::fill(&mut bytes)
        .map_err(|error| format!("secure random generation failed: {error}"))?;
    Ok(base64::engine::general_purpose::URL_SAFE_NO_PAD.encode(bytes))
}

fn constant_time_equal(left: &[u8], right: &[u8]) -> bool {
    let mut difference = left.len() ^ right.len();
    for index in 0..left.len().max(right.len()) {
        difference |= usize::from(*left.get(index).unwrap_or(&0) ^ *right.get(index).unwrap_or(&0));
    }
    difference == 0
}

fn open_browser(url: &str) -> Result<(), std::io::Error> {
    #[cfg(target_os = "macos")]
    let mut command = Command::new("open");
    #[cfg(target_os = "windows")]
    let mut command = {
        let mut command = Command::new("rundll32");
        command.arg("url.dll,FileProtocolHandler");
        command
    };
    #[cfg(all(unix, not(target_os = "macos")))]
    let mut command = Command::new("xdg-open");
    command.arg(url).spawn().map(|_| ())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn oauth_token_validation_honors_missing_expiry_and_clock_skew() {
        let now = 1_800_000_000;
        assert!(token_is_valid(&json!({"access_token":"static"}), now));
        assert!(token_is_valid(
            &json!({"access_token":"cached","expires_at":now + 31}),
            now
        ));
        assert!(!token_is_valid(
            &json!({"access_token":"expired","expires_at":now + 30}),
            now
        ));
        assert!(!token_is_valid(&json!({"access_token":""}), now));
        assert!(!token_is_valid(&json!({"expires_at":now + 600}), now));
    }

    #[test]
    fn oauth_cache_identity_includes_endpoint_client_grant_and_scope() {
        let base = json!({
            "tokenUrl":"https://auth.example.test/token",
            "clientId":"client-a",
            "grantType":"client_credentials",
            "scope":"read write"
        });
        assert_eq!(
            token_cache_key(&base),
            "https://auth.example.test/token|client-a|client_credentials|read write"
        );
        for changed in [
            json!({"tokenUrl":"https://auth.example.test/other","clientId":"client-a","grantType":"client_credentials","scope":"read write"}),
            json!({"tokenUrl":"https://auth.example.test/token","clientId":"client-b","grantType":"client_credentials","scope":"read write"}),
            json!({"tokenUrl":"https://auth.example.test/token","clientId":"client-a","grantType":"refresh_token","scope":"read write"}),
            json!({"tokenUrl":"https://auth.example.test/token","clientId":"client-a","grantType":"client_credentials","scope":"read"}),
        ] {
            assert_ne!(token_cache_key(&base), token_cache_key(&changed));
        }
        assert!(token_cache_key(&json!({})).ends_with("|client_credentials|"));
    }

    #[tokio::test]
    async fn oauth_static_token_is_reused_and_invalid_interactive_state_is_reported() {
        let auth = json!({
            "tokenUrl":"https://auth.example.test/token",
            "clientId":"unit-test-static-token",
            "grantType":"authorization_code",
            "token":"static-access-token"
        });
        let token = get_token(&auth).await.unwrap();
        assert_eq!(token["access_token"], "static-access-token");
        assert_eq!(get_token(&auth).await.unwrap(), token);

        let expired = json!({
            "tokenUrl":"",
            "clientId":"unit-test-expired-token",
            "grantType":"authorization_code",
            "token":"expired-token",
            "tokenExpiresAt":1
        });
        assert!(
            get_token(&expired)
                .await
                .unwrap_err()
                .contains("authorization code flow requires interactive login")
        );
    }
}
