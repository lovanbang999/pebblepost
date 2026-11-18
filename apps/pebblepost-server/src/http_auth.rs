use std::collections::{BTreeMap, HashMap};

use hmac::{Hmac, Mac};
use md5::Md5;
use serde_json::Value;
use sha2::{Digest, Sha256};

pub fn request_headers_json(request: &reqwest::Request) -> serde_json::Map<String, Value> {
    let mut headers = serde_json::Map::new();
    for (key, _) in request.headers() {
        let values = request.headers().get_all(key).iter()
            .map(|value| value.to_str().unwrap_or("").to_owned()).collect::<Vec<_>>();
        headers.insert(key.as_str().to_owned(), serde_json::json!(values));
    }
    headers
}

pub fn sign_aws_sigv4(request: &mut reqwest::Request, auth: &Value) -> Result<(), String> {
    let access_key = auth.get("accessKey").and_then(Value::as_str).unwrap_or("").trim();
    let secret_key = auth.get("secretKey").and_then(Value::as_str).unwrap_or("").trim();
    if access_key.is_empty() || secret_key.is_empty() {
        return Err("AWS access key and secret key are required".into());
    }
    let region = auth.get("region").and_then(Value::as_str).unwrap_or("").trim();
    let region = if region.is_empty() { "us-east-1" } else { region };
    let service = auth.get("service").and_then(Value::as_str).unwrap_or("").trim();
    let service = if service.is_empty() { "execute-api" } else { service };
    let session_token = auth.get("sessionToken").and_then(Value::as_str).unwrap_or("").trim();
    let now = chrono::Utc::now();
    let date_time = now.format("%Y%m%dT%H%M%SZ").to_string();
    let date = now.format("%Y%m%d").to_string();
    let body = match request.body() {
        Some(body) => body.as_bytes().ok_or_else(|| "AWS Signature v4 requires a buffered request body".to_owned())?,
        None => &[][..],
    };
    let payload_hash = sha256_hex(body);
    let headers = request.headers_mut();
    headers.insert("x-amz-date", reqwest::header::HeaderValue::from_str(&date_time).map_err(|error| error.to_string())?);
    headers.insert("x-amz-content-sha256", reqwest::header::HeaderValue::from_str(&payload_hash).map_err(|error| error.to_string())?);
    if !session_token.is_empty() {
        headers.insert("x-amz-security-token", reqwest::header::HeaderValue::from_str(session_token).map_err(|error| error.to_string())?);
    }

    let url = request.url();
    let canonical_uri = if url.path().is_empty() { "/" } else { url.path() };
    let mut query_pairs = url.query_pairs().map(|(key, value)| (key.into_owned(), value.into_owned())).collect::<Vec<_>>();
    query_pairs.sort();
    let canonical_query = query_pairs.iter().map(|(key, value)| format!("{}={}", aws_query_escape(key), aws_query_escape(value)))
        .collect::<Vec<_>>().join("&");

    let host_value = request.headers().get(reqwest::header::HOST).and_then(|value| value.to_str().ok()).map(str::to_owned)
        .unwrap_or_else(|| {
            let host = url.host_str().unwrap_or("");
            let host = if host.contains(':') { format!("[{host}]") } else { host.to_owned() };
            url.port().map_or(host.clone(), |port| format!("{host}:{port}"))
        });
    let mut canonical_header_values = BTreeMap::new();
    canonical_header_values.insert("host".to_owned(), host_value.trim().to_owned());
    for (key, value) in request.headers() {
        let name = key.as_str().to_ascii_lowercase();
        if name == "host" || name == "content-type" || name.starts_with("x-amz-") {
            let entry = canonical_header_values.entry(name).or_insert_with(String::new);
            if !entry.is_empty() { entry.push(','); }
            entry.push_str(value.to_str().map_err(|error| format!("invalid signed header value: {error}"))?.trim());
        }
    }
    let canonical_headers = canonical_header_values.iter().map(|(name, value)| format!("{name}:{value}\n")).collect::<String>();
    let signed_headers = canonical_header_values.keys().cloned().collect::<Vec<_>>().join(";");
    let canonical_request = format!("{}\n{}\n{}\n{}\n{}\n{}",
        request.method().as_str(), canonical_uri, canonical_query, canonical_headers, signed_headers, payload_hash);
    let scope = format!("{date}/{region}/{service}/aws4_request");
    let string_to_sign = format!("AWS4-HMAC-SHA256\n{date_time}\n{scope}\n{}", sha256_hex(canonical_request.as_bytes()));
    let date_key = hmac_sha256(format!("AWS4{secret_key}").as_bytes(), date.as_bytes());
    let region_key = hmac_sha256(&date_key, region.as_bytes());
    let service_key = hmac_sha256(&region_key, service.as_bytes());
    let signing_key = hmac_sha256(&service_key, b"aws4_request");
    let signature = hex_lower(&hmac_sha256(&signing_key, string_to_sign.as_bytes()));
    let authorization = format!("AWS4-HMAC-SHA256 Credential={access_key}/{scope}, SignedHeaders={signed_headers}, Signature={signature}");
    request.headers_mut().insert(reqwest::header::AUTHORIZATION,
        reqwest::header::HeaderValue::from_str(&authorization).map_err(|error| error.to_string())?);
    Ok(())
}

pub fn build_digest_authorization(
    method: &str,
    uri: &str,
    username: &str,
    password: &str,
    challenge_header: &str,
    body: Option<&[u8]>,
) -> Result<String, String> {
    let challenge = parse_digest_challenge(challenge_header)?;
    let realm = challenge.get("realm").map(String::as_str).unwrap_or("");
    let nonce = challenge.get("nonce").map(String::as_str).unwrap_or("");
    let algorithm = challenge.get("algorithm").map(String::as_str).unwrap_or("MD5");
    let algorithm_upper = algorithm.to_ascii_uppercase();
    let hash = |input: &[u8]| -> String {
        if algorithm_upper.starts_with("SHA-256") { sha256_hex(input) }
        else { hex_lower(&Md5::digest(input)) }
    };
    let ha1 = hash(format!("{username}:{realm}:{password}").as_bytes());
    let offered_qop = challenge.get("qop").map(String::as_str).unwrap_or("");
    let qop_options = offered_qop.split(',').map(|value| value.trim().to_ascii_lowercase())
        .filter(|value| !value.is_empty()).collect::<Vec<_>>();
    let qop = if qop_options.iter().any(|value| value == "auth") { "auth" }
        else if qop_options.iter().any(|value| value == "auth-int") { "auth-int" }
        else { qop_options.first().map(String::as_str).unwrap_or("") };
    let mut random = [0_u8; 8];
    getrandom::fill(&mut random).map_err(|error| format!("could not create Digest client nonce: {error}"))?;
    let cnonce = hex_lower(&random);
    let nc = "00000001";
    let ha2 = if qop == "auth-int" {
        let body = body.ok_or_else(|| "Digest auth-int requires a buffered request body".to_owned())?;
        let body_hash = hash(body);
        hash(format!("{method}:{uri}:{body_hash}").as_bytes())
    } else { hash(format!("{method}:{uri}").as_bytes()) };
    let response = if qop == "auth" || qop == "auth-int" {
        hash(format!("{ha1}:{nonce}:{nc}:{cnonce}:{qop}:{ha2}").as_bytes())
    } else { hash(format!("{ha1}:{nonce}:{ha2}").as_bytes()) };
    let escape = |value: &str| value.replace('\\', "\\\\").replace('"', "\\\"");
    let mut parts = vec![
        format!("Digest username=\"{}\"", escape(username)),
        format!("realm=\"{}\"", escape(realm)),
        format!("nonce=\"{}\"", escape(nonce)),
        format!("uri=\"{}\"", escape(uri)),
        format!("response=\"{response}\""),
        format!("algorithm=\"{algorithm}\""),
    ];
    if let Some(opaque) = challenge.get("opaque").filter(|value| !value.is_empty()) {
        parts.push(format!("opaque=\"{}\"", escape(opaque)));
    }
    if !qop.is_empty() {
        parts.push(format!("qop={qop}"));
        parts.push(format!("nc={nc}"));
        parts.push(format!("cnonce=\"{cnonce}\""));
    }
    Ok(parts.join(", "))
}

fn parse_digest_challenge(header: &str) -> Result<HashMap<String, String>, String> {
    let trimmed = header.trim();
    let (scheme, raw) = trimmed.split_once(char::is_whitespace).ok_or_else(|| "invalid Digest challenge".to_owned())?;
    if !scheme.eq_ignore_ascii_case("digest") { return Err("not a Digest challenge".into()); }
    let mut params = HashMap::new();
    let mut start = 0;
    let mut quoted = false;
    let mut escaped = false;
    for (index, ch) in raw.char_indices().chain(std::iter::once((raw.len(), ','))) {
        if escaped { escaped = false; continue; }
        if ch == '\\' && quoted { escaped = true; continue; }
        if ch == '"' { quoted = !quoted; continue; }
        if ch != ',' || quoted { continue; }
        let part = raw[start..index].trim();
        start = index.saturating_add(ch.len_utf8());
        let Some((key, value)) = part.split_once('=') else { continue; };
        let value = value.trim();
        let value = if value.starts_with('"') && value.ends_with('"') && value.len() >= 2 {
            value[1..value.len() - 1].replace("\\\"", "\"").replace("\\\\", "\\")
        } else { value.to_owned() };
        params.insert(key.trim().to_ascii_lowercase(), value);
    }
    if !params.contains_key("nonce") { return Err("Digest challenge does not contain a nonce".into()); }
    params.entry("algorithm".into()).or_insert_with(|| "MD5".into());
    Ok(params)
}

fn aws_query_escape(value: &str) -> String {
    let mut escaped = String::with_capacity(value.len());
    for byte in value.bytes() {
        if byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'_' | b'.' | b'~') {
            escaped.push(byte as char);
        } else {
            escaped.push('%');
            escaped.push(char::from_digit((byte >> 4) as u32, 16).unwrap_or('0').to_ascii_uppercase());
            escaped.push(char::from_digit((byte & 0x0f) as u32, 16).unwrap_or('0').to_ascii_uppercase());
        }
    }
    escaped
}

fn sha256_hex(bytes: &[u8]) -> String { hex_lower(&Sha256::digest(bytes)) }

fn hmac_sha256(key: &[u8], message: &[u8]) -> Vec<u8> {
    let mut mac = Hmac::<Sha256>::new_from_slice(key).expect("HMAC accepts any key length");
    mac.update(message);
    mac.finalize().into_bytes().to_vec()
}

fn hex_lower(bytes: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut out = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        out.push(HEX[(byte >> 4) as usize] as char);
        out.push(HEX[(byte & 0x0f) as usize] as char);
    }
    out
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn aws_sigv4_signer_sets_payload_date_session_and_authorization_headers() {
        let url = reqwest::Url::parse("https://execute-api.us-east-1.amazonaws.com/v1/orders?b=2&a=1").unwrap();
        let mut request = reqwest::Request::new(reqwest::Method::POST, url);
        *request.body_mut() = Some(reqwest::Body::from("{\"id\":7}"));
        request.headers_mut().insert("content-type", reqwest::header::HeaderValue::from_static("application/json"));
        sign_aws_sigv4(&mut request, &json!({
            "accessKey": "AKIDEXAMPLE", "secretKey": "secret", "region": "us-east-1",
            "service": "execute-api", "sessionToken": "session"
        })).unwrap();
        let authorization = request.headers().get("authorization").unwrap().to_str().unwrap();
        assert!(authorization.starts_with("AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/"));
        assert!(authorization.contains("SignedHeaders=content-type;host;x-amz-content-sha256;x-amz-date;x-amz-security-token"));
        assert!(authorization.rsplit("Signature=").next().is_some_and(|value| value.len() == 64));
        assert_eq!(request.headers()["x-amz-security-token"], "session");
        assert_eq!(request.headers()["x-amz-content-sha256"].to_str().unwrap(), sha256_hex(br#"{"id":7}"#));
    }

    #[test]
    fn digest_authorization_supports_auth_and_auth_int_challenges() {
        let auth = build_digest_authorization(
            "POST", "/resource?x=1", "user", "password",
            r#"Digest realm="users, realm", nonce="abc123", qop="auth,auth-int", algorithm=SHA-256, opaque="opaque""#,
            Some(b"body"),
        ).unwrap();
        assert!(auth.starts_with("Digest username=\"user\""));
        assert!(auth.contains("realm=\"users, realm\""));
        assert!(auth.contains("nonce=\"abc123\""));
        assert!(auth.contains("algorithm=\"SHA-256\""));
        assert!(auth.contains("qop=auth,"));
        assert!(auth.contains("opaque=\"opaque\""));
        let response = auth.split("response=\"").nth(1).and_then(|value| value.split('"').next()).unwrap();
        assert_eq!(response.len(), 64);

        let auth_int = build_digest_authorization("POST", "/", "user", "password", "Digest realm=\"r\", nonce=\"n\", qop=\"auth-int\"", Some(b"body")).unwrap();
        assert!(auth_int.contains("qop=auth-int,"));
        assert!(build_digest_authorization("GET", "/", "user", "password", "Basic abc", None).is_err());
    }
}
