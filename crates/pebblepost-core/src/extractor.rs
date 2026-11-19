use std::collections::HashMap;

use serde::{Deserialize, Serialize};
use serde_json::Value;

#[derive(Clone, Debug)]
enum Segment {
    Key(String),
    Index(isize),
    Wildcard,
}

#[derive(Clone, Debug, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct ExtractionResult {
    pub extractor_id: String,
    pub name: String,
    pub scope: String,
    pub path: String,
    pub value: String,
    pub success: bool,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub warning: Option<String>,
}

pub fn extract(extractor: &Value, response_body: &str) -> ExtractionResult {
    extract_with_response(extractor, response_body, None)
}

/// Extract a value from a response body, header, or cookie. `response_headers`
/// accepts the same object shape returned by the HTTP execution API.
pub fn extract_with_response(
    extractor: &Value,
    response_body: &str,
    response_headers: Option<&Value>,
) -> ExtractionResult {
    let extractor_id = extractor
        .get("id")
        .and_then(Value::as_str)
        .unwrap_or_default()
        .to_owned();
    let name = extractor
        .get("name")
        .and_then(Value::as_str)
        .unwrap_or_default()
        .to_owned();
    let path = extractor
        .get("path")
        .and_then(Value::as_str)
        .unwrap_or_default()
        .to_owned();
    let extractor_type = extractor
        .get("type")
        .and_then(Value::as_str)
        .unwrap_or("jsonpath")
        .to_ascii_lowercase();
    let scope = extractor
        .get("scope")
        .and_then(Value::as_str)
        .filter(|scope| !scope.is_empty())
        .unwrap_or("runtime")
        .to_owned();
    let mut result = ExtractionResult {
        extractor_id,
        name: name.clone(),
        scope,
        path: path.clone(),
        value: String::new(),
        success: false,
        warning: None,
    };

    match extractor_type.as_str() {
        "" | "jsonpath" | "json-path" => {
            let body = response_body.trim();
            if body.is_empty() {
                result.warning = Some(format!("Extractor {name:?}: response body is empty"));
                return result;
            }
            let root = match serde_json::from_str::<Value>(body) {
                Ok(root) => root,
                Err(error) => {
                    result.warning = Some(format!(
                        "Extractor {name:?}: response body is not valid JSON: {error}"
                    ));
                    return result;
                }
            };
            match evaluate_json_path(&root, &path) {
                Ok(Some(value)) => {
                    result.value = format_extracted_value(&value);
                    result.success = true;
                }
                Ok(None) => {
                    result.warning = Some(format!(
                        "Extractor {name:?}: JSONPath {path:?} not found in response"
                    ))
                }
                Err(error) => {
                    result.warning = Some(format!(
                        "Extractor {name:?}: invalid JSONPath {path:?}: {error}"
                    ))
                }
            }
        }
        "regex" | "regexp" => match regex::Regex::new(&path) {
            Ok(regex) => {
                if let Some(captures) = regex.captures(response_body) {
                    let group = extractor
                        .get("captureGroup")
                        .or_else(|| extractor.get("group"));
                    let capture = match group {
                        Some(Value::Number(number)) => number
                            .as_u64()
                            .and_then(|index| captures.get(index as usize)),
                        Some(Value::String(name)) => captures.name(name),
                        _ => captures.get(1).or_else(|| captures.get(0)),
                    };
                    if let Some(capture) = capture {
                        result.value = capture.as_str().to_owned();
                        result.success = true;
                    } else {
                        result.warning = Some(format!(
                            "Extractor {name:?}: regex capture group was not found"
                        ));
                    }
                } else {
                    result.warning = Some(format!(
                        "Extractor {name:?}: regex {path:?} did not match response"
                    ));
                }
            }
            Err(error) => {
                result.warning = Some(format!(
                    "Extractor {name:?}: invalid regex {path:?}: {error}"
                ))
            }
        },
        "header" | "response-header" => match response_header(response_headers, &path) {
            Some(value) => {
                result.value = value;
                result.success = true;
            }
            None => {
                result.warning = Some(format!(
                    "Extractor {name:?}: response header {path:?} was not found"
                ))
            }
        },
        "cookie" | "response-cookie" => match response_cookie(response_headers, &path) {
            Some(value) => {
                result.value = value;
                result.success = true;
            }
            None => {
                result.warning = Some(format!(
                    "Extractor {name:?}: response cookie {path:?} was not found"
                ))
            }
        },
        unsupported => {
            result.warning = Some(format!(
                "Extractor {name:?}: unsupported extractor type {unsupported:?}"
            ))
        }
    }
    result
}

pub fn extract_all(
    extractors: &[Value],
    response_body: &str,
) -> (Vec<ExtractionResult>, HashMap<String, String>, Vec<String>) {
    extract_all_with_response(extractors, response_body, None)
}

pub fn extract_all_with_response(
    extractors: &[Value],
    response_body: &str,
    response_headers: Option<&Value>,
) -> (Vec<ExtractionResult>, HashMap<String, String>, Vec<String>) {
    let mut results = Vec::new();
    let mut extracted = HashMap::new();
    let mut warnings = Vec::new();
    for extractor in extractors.iter().filter(|item| {
        item.get("enabled")
            .and_then(Value::as_bool)
            .unwrap_or(false)
    }) {
        let result = extract_with_response(extractor, response_body, response_headers);
        if result.success {
            extracted.insert(result.name.clone(), result.value.clone());
        }
        if let Some(warning) = result.warning.as_ref() {
            warnings.push(warning.clone());
        }
        results.push(result);
    }
    (results, extracted, warnings)
}

fn response_header(headers: Option<&Value>, requested_name: &str) -> Option<String> {
    let headers = headers?.as_object()?;
    headers
        .iter()
        .find(|(name, _)| name.eq_ignore_ascii_case(requested_name))
        .and_then(|(_, value)| match value {
            Value::Array(values) => values.first(),
            value => Some(value),
        })
        .and_then(|value| {
            value
                .as_str()
                .map(str::to_owned)
                .or_else(|| (!value.is_null()).then(|| value.to_string()))
        })
}

fn response_cookie(headers: Option<&Value>, cookie_name: &str) -> Option<String> {
    let headers = headers?.as_object()?;
    for (name, header_value) in headers {
        if !name.eq_ignore_ascii_case("set-cookie") && !name.eq_ignore_ascii_case("cookie") {
            continue;
        }
        let values: Vec<&Value> = match header_value {
            Value::Array(values) => values.iter().collect(),
            value => vec![value],
        };
        for value in values.into_iter().filter_map(Value::as_str) {
            for pair in value.split(';') {
                if let Some((name, value)) = pair.trim().split_once('=')
                    && name.trim() == cookie_name
                {
                    return Some(value.trim().to_owned());
                }
            }
        }
    }
    None
}

pub fn evaluate_json_path(root: &Value, path: &str) -> Result<Option<Value>, String> {
    let segments = parse_json_path(path)?;
    let mut current = vec![root.clone()];
    for segment in segments {
        let mut next = Vec::new();
        for value in current {
            match &segment {
                Segment::Key(key) => {
                    if let Some(value) = value.get(key) {
                        next.push(value.clone());
                    }
                }
                Segment::Index(index) => {
                    if let Some(values) = value.as_array() {
                        let resolved = if *index < 0 {
                            values.len() as isize + index
                        } else {
                            *index
                        };
                        if resolved >= 0
                            && let Some(value) = values.get(resolved as usize)
                        {
                            next.push(value.clone());
                        }
                    }
                }
                Segment::Wildcard => match value {
                    Value::Array(values) => next.extend(values),
                    Value::Object(values) => next.extend(values.into_values()),
                    _ => {}
                },
            }
        }
        if next.is_empty() {
            return Ok(None);
        }
        current = next;
    }
    match current.len() {
        0 => Ok(None),
        1 => Ok(current.pop()),
        _ => Ok(Some(Value::Array(current))),
    }
}

fn parse_json_path(path: &str) -> Result<Vec<Segment>, String> {
    let mut path = path.trim();
    if path.is_empty() || path == "$" {
        return Ok(Vec::new());
    }
    if let Some(rest) = path.strip_prefix("$.") {
        path = rest;
    } else if let Some(rest) = path.strip_prefix('$') {
        path = rest;
    }

    let chars = path.chars().collect::<Vec<_>>();
    let mut segments = Vec::new();
    let mut buffer = String::new();
    let mut index = 0;
    while index < chars.len() {
        match chars[index] {
            '.' => {
                if !buffer.is_empty() {
                    segments.push(Segment::Key(std::mem::take(&mut buffer)));
                }
                index += 1;
            }
            '[' => {
                if !buffer.is_empty() {
                    segments.push(Segment::Key(std::mem::take(&mut buffer)));
                }
                index += 1;
                let start = index;
                let mut quoted = false;
                let mut quote = '\0';
                while index < chars.len() {
                    let ch = chars[index];
                    if quoted {
                        if ch == quote {
                            quoted = false;
                        }
                    } else if ch == '\'' || ch == '"' {
                        quoted = true;
                        quote = ch;
                    } else if ch == ']' {
                        break;
                    }
                    index += 1;
                }
                if index == chars.len() {
                    return Err(format!("unclosed bracket in JSONPath {path:?}"));
                }
                let inner = chars[start..index]
                    .iter()
                    .collect::<String>()
                    .trim()
                    .to_owned();
                let unquoted = if inner.len() >= 2
                    && ((inner.starts_with('\'') && inner.ends_with('\''))
                        || (inner.starts_with('"') && inner.ends_with('"')))
                {
                    inner[1..inner.len() - 1].to_owned()
                } else {
                    inner.clone()
                };
                if unquoted == "*" {
                    segments.push(Segment::Wildcard);
                } else if let Ok(array_index) = unquoted.parse::<isize>() {
                    segments.push(Segment::Index(array_index));
                } else if !unquoted.is_empty() {
                    segments.push(Segment::Key(unquoted));
                }
                index += 1;
            }
            ch => {
                buffer.push(ch);
                index += 1;
            }
        }
    }
    if !buffer.is_empty() {
        segments.push(Segment::Key(buffer));
    }
    Ok(segments)
}

fn format_extracted_value(value: &Value) -> String {
    match value {
        Value::Null => String::new(),
        Value::String(value) => value.clone(),
        Value::Number(number) => {
            if let Some(integer) = number.as_i64() {
                integer.to_string()
            } else if let Some(integer) = number.as_u64() {
                integer.to_string()
            } else if let Some(float) = number.as_f64() {
                if float.is_finite() && float.fract() == 0.0 {
                    format!("{float:.0}")
                } else {
                    number.to_string()
                }
            } else {
                number.to_string()
            }
        }
        Value::Bool(value) => value.to_string(),
        Value::Array(_) | Value::Object(_) => value.to_string(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn extracts_jsonpath_regex_response_headers_and_cookies() {
        let response_body = r#"{"data":{"token":"abc-123"},"message":"user=alice"}"#;
        let headers = serde_json::json!({
            "Content-Type": ["application/json"],
            "set-cookie": ["session_id=sess-456; Path=/; HttpOnly"]
        });
        let extractors = vec![
            serde_json::json!({ "id": "json", "name": "TOKEN", "type": "jsonpath", "path": "$.data.token", "enabled": true }),
            serde_json::json!({ "id": "regex", "name": "USER", "type": "regex", "path": "user=([a-z]+)", "enabled": true }),
            serde_json::json!({ "id": "header", "name": "CONTENT_TYPE", "type": "header", "path": "content-type", "enabled": true }),
            serde_json::json!({ "id": "cookie", "name": "SESSION", "type": "cookie", "path": "session_id", "enabled": true }),
        ];

        let (results, variables, warnings) =
            extract_all_with_response(&extractors, response_body, Some(&headers));
        assert_eq!(results.len(), 4);
        assert!(warnings.is_empty());
        assert_eq!(variables["TOKEN"], "abc-123");
        assert_eq!(variables["USER"], "alice");
        assert_eq!(variables["CONTENT_TYPE"], "application/json");
        assert_eq!(variables["SESSION"], "sess-456");
    }

    #[test]
    fn regex_can_select_a_named_capture_and_reports_invalid_patterns() {
        let named = extract(
            &serde_json::json!({
                "name": "ID", "type": "regex", "path": r"id=(?P<value>\d+)", "group": "value"
            }),
            "id=42",
        );
        assert!(named.success);
        assert_eq!(named.value, "42");

        let invalid = extract(
            &serde_json::json!({ "name": "BAD", "type": "regex", "path": "[", "enabled": true }),
            "text",
        );
        assert!(!invalid.success);
        assert!(invalid.warning.unwrap().contains("invalid regex"));
    }
}
