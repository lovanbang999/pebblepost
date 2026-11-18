use std::collections::BTreeMap;
use std::fs;
use std::path::{Path, PathBuf};

use base64::Engine;
use pebblepost_core::{
    PebbleError, create_folder, read_folder, read_request, safe_workspace_path, save_folder,
    save_request,
};
use serde_json::{Map, Value, json};

const MAX_IMPORT_BYTES: usize = 50 * 1024 * 1024;

pub struct ImportResult {
    pub format: String,
    pub collection_name: String,
    pub requests: Vec<(String, Value)>,
    pub folders: Vec<(String, Value)>,
    pub variables: Vec<Value>,
    pub skipped: Vec<Value>,
    pub warnings: Vec<String>,
}

pub fn parse_import(source: &str, content: &str) -> Result<ImportResult, String> {
    if content.len() > MAX_IMPORT_BYTES {
        return Err("File size exceeds maximum allowed size of 50 MB".into());
    }
    let detected = if source.trim().is_empty() {
        detect_import_format(content)
    } else {
        Some(source.trim().to_ascii_lowercase())
    };
    let result = if let Some(format) = detected {
        parse_import_format(&format, content)?
    } else {
        let mut parsed = None;
        for format in ["postman", "insomnia", "openapi", "har", "curl"] {
            if let Ok(result) = parse_import_format(format, content)
                && (!result.requests.is_empty() || !result.skipped.is_empty())
            {
                parsed = Some(result);
                break;
            }
        }
        parsed.ok_or_else(|| "unrecognized or unsupported import format".to_owned())?
    };
    if result.requests.is_empty() && result.skipped.is_empty() {
        return Err("No requests found in the import data".into());
    }
    Ok(result)
}

fn parse_import_format(format: &str, content: &str) -> Result<ImportResult, String> {
    match format {
        "curl" => parse_curl(content),
        "postman" => parse_postman(content),
        "openapi" => parse_openapi(content),
        "har" => parse_har(content),
        "insomnia" => parse_insomnia(content),
        "bruno" => parse_bruno(content),
        other => Err(format!("Unsupported import format: {other}")),
    }
}

pub fn save_import(
    root: &Path,
    out_dir: Option<&str>,
    result: &ImportResult,
) -> Result<(), PebbleError> {
    let root = fs::canonicalize(root)?;
    let base = out_dir
        .map(PathBuf::from)
        .unwrap_or_else(|| PathBuf::from("collections").join(slug(&result.collection_name)));
    ensure_directories(&root, &base)?;
    for (relative, definition) in &result.folders {
        let folder = if relative.is_empty() || relative == "." {
            base.clone()
        } else {
            base.join(relative)
        };
        ensure_directories(&root, &folder)?;
        save_folder(&root, &folder, definition.clone())?;
    }
    for (relative, request) in &result.requests {
        let rel_path = base.join(relative);
        if let Some(parent) = rel_path.parent() {
            ensure_directories(&root, parent)?;
        }
        save_request(&root, &rel_path, request.clone())?;
    }
    Ok(())
}

pub fn export_collection(
    root: &Path,
    folder: &str,
    format: &str,
    collection_name: Option<&str>,
) -> Result<Value, PebbleError> {
    let root = fs::canonicalize(root)?;
    let requested = Path::new(folder);
    let target = if requested.is_absolute() {
        requested.to_path_buf()
    } else {
        root.join(requested)
    };
    let target = safe_workspace_path(&root, &target)?;
    let mut requests = Vec::new();
    let mut folders = Vec::new();
    collect_export(&root, &target, &target, &mut requests, &mut folders)?;
    let default_name = target
        .file_name()
        .and_then(|name| name.to_str())
        .filter(|name| !name.is_empty() && *name != "collections")
        .unwrap_or("PebblePost Export");
    let name = collection_name
        .filter(|name| !name.trim().is_empty())
        .unwrap_or(default_name);
    let normalized = format.to_ascii_lowercase();
    let (content, extension) = match normalized.as_str() {
        "postman" => (
            serde_json::to_string_pretty(&postman_collection(name, &requests, &folders))
                .map_err(PebbleError::from)?,
            "json",
        ),
        "openapi" => (
            serde_json::to_string_pretty(&openapi_collection(name, &requests))
                .map_err(PebbleError::from)?,
            "json",
        ),
        other => {
            return Err(PebbleError::InvalidPath(format!(
                "unsupported export format: {other}"
            )));
        }
    };
    let filename = format!("{}_{}.{}", slug(name), normalized, extension);
    Ok(
        json!({ "format": normalized, "filename": filename, "content": content, "count": requests.len() }),
    )
}

fn detect_import_format(content: &str) -> Option<String> {
    let trimmed = content.trim();
    let lower = trimmed.to_ascii_lowercase();
    if lower.starts_with("curl") {
        return Some("curl".into());
    }
    if trimmed.starts_with("meta {") || trimmed.contains("\nmeta {") {
        return Some("bruno".into());
    }
    if trimmed.starts_with("openapi: 3.")
        || trimmed.starts_with("openapi: '3.")
        || trimmed.starts_with("openapi: \"3.")
    {
        return Some("openapi".into());
    }
    let Ok(value) = serde_json::from_str::<Value>(trimmed) else {
        return None;
    };
    if value
        .pointer("/info/schema")
        .and_then(Value::as_str)
        .is_some_and(|schema| schema.contains("getpostman.com"))
        || (value.get("info").is_some() && value.get("item").is_some())
    {
        return Some("postman".into());
    }
    if value.get("__export_format").and_then(Value::as_f64) == Some(4.0)
        || value.get("_type").and_then(Value::as_str) == Some("export")
        || value.get("resources").is_some()
    {
        return Some("insomnia".into());
    }
    if value.pointer("/log/entries").is_some() {
        return Some("har".into());
    }
    if value
        .get("openapi")
        .and_then(Value::as_str)
        .is_some_and(|version| version.starts_with("3."))
        || value.get("swagger").is_some()
    {
        return Some("openapi".into());
    }
    None
}

fn parse_postman(content: &str) -> Result<ImportResult, String> {
    let value: Value =
        serde_json::from_str(content).map_err(|error| format!("Invalid Postman JSON: {error}"))?;
    if value.get("info").is_none() {
        return Err("Postman collection is missing info".into());
    }
    let name = value
        .pointer("/info/name")
        .and_then(Value::as_str)
        .unwrap_or("Postman Collection")
        .to_owned();
    let mut result = ImportResult {
        format: "postman".into(),
        collection_name: name,
        requests: Vec::new(),
        folders: Vec::new(),
        variables: Vec::new(),
        skipped: Vec::new(),
        warnings: Vec::new(),
    };
    let root_auth = postman_auth(value.get("auth"));
    result.variables = value.get("variable").and_then(Value::as_array).map(|variables| variables.iter().filter_map(|variable| {
        let key = variable.get("key")?.as_str()?;
        Some(json!({ "key": key, "value": variable.get("value").and_then(Value::as_str).unwrap_or(""), "enabled": !variable.get("disabled").and_then(Value::as_bool).unwrap_or(false) }))
    }).collect()).unwrap_or_default();
    let (root_scripts, root_warnings) = postman_scripts(value.get("event"));
    result.warnings.extend(root_warnings);
    if !result.variables.is_empty()
        || root_auth
            .get("type")
            .and_then(Value::as_str)
            .is_some_and(|kind| kind != "none" && kind != "inherit")
        || root_scripts
            .get("preRequest")
            .and_then(Value::as_str)
            .is_some_and(|script| !script.is_empty())
        || root_scripts
            .get("postResponse")
            .and_then(Value::as_str)
            .is_some_and(|script| !script.is_empty())
    {
        result.folders.push((".".into(), json!({
            "schemaVersion": 1,
            "name": result.collection_name.clone(),
            "description": value.pointer("/info/description").and_then(Value::as_str).unwrap_or(""),
            "auth": root_auth.clone(),
            "headers": [],
            "variables": result.variables.clone(),
            "scripts": root_scripts
        })));
    }
    let mut seen_names = BTreeMap::new();
    walk_postman(
        value
            .get("item")
            .and_then(Value::as_array)
            .map(Vec::as_slice)
            .unwrap_or(&[]),
        "",
        &root_auth,
        &mut result,
        &mut seen_names,
    );
    Ok(result)
}

fn walk_postman(
    items: &[Value],
    parent: &str,
    inherited_auth: &Value,
    result: &mut ImportResult,
    seen_names: &mut BTreeMap<String, usize>,
) {
    for item in items {
        let name = item
            .get("name")
            .and_then(Value::as_str)
            .unwrap_or("Request");
        if let Some(children) = item.get("item").and_then(Value::as_array) {
            let folder_name = slug(name);
            let path = if parent.is_empty() {
                folder_name
            } else {
                format!("{parent}/{folder_name}")
            };
            let auth = item
                .get("auth")
                .map(|auth| postman_auth(Some(auth)))
                .filter(|auth| {
                    auth.get("type")
                        .and_then(Value::as_str)
                        .is_some_and(|kind| kind != "none" && kind != "inherit")
                })
                .unwrap_or_else(|| inherited_auth.clone());
            let (scripts, warnings) = postman_scripts(item.get("event"));
            result.warnings.extend(warnings);
            result.folders.push((
                path.clone(),
                json!({
                    "schemaVersion": 1,
                    "name": name,
                    "description": item.get("description").and_then(Value::as_str).unwrap_or(""),
                    "auth": auth.clone(),
                    "headers": [],
                    "variables": [],
                    "scripts": scripts
                }),
            ));
            walk_postman(children, &path, &auth, result, seen_names);
        } else if let Some(req) = item.get("request") {
            let (url, params) = postman_url(req.get("url"));
            let source_name = item.get("name").and_then(Value::as_str).unwrap_or("");
            if url.is_empty() && source_name.is_empty() {
                result.skipped.push(json!({ "name": "Unnamed request", "path": parent, "reason": "Empty request URL and name" }));
                continue;
            }
            let request_name = item
                .get("name")
                .and_then(Value::as_str)
                .filter(|name| !name.is_empty())
                .map(str::to_owned)
                .unwrap_or_else(|| {
                    format!(
                        "{} Request",
                        req.get("method").and_then(Value::as_str).unwrap_or("GET")
                    )
                });
            let path_key = if parent.is_empty() {
                request_name.clone()
            } else {
                format!("{parent}/{request_name}")
            };
            let duplicate_count = seen_names.entry(path_key).or_insert(0);
            *duplicate_count += 1;
            let final_name = if *duplicate_count == 1 {
                request_name
            } else {
                format!("{} ({})", request_name, duplicate_count)
            };
            let method = req.get("method").and_then(Value::as_str).unwrap_or("GET");
            let headers = req.get("header").and_then(Value::as_array).map(|headers| headers.iter().filter_map(|h| {
                let key = h.get("key")?.as_str()?;
                Some(json!({ "key": key, "value": h.get("value").and_then(Value::as_str).unwrap_or(""), "enabled": !h.get("disabled").and_then(Value::as_bool).unwrap_or(false) }))
            }).collect::<Vec<_>>()).unwrap_or_default();
            let mut body = json!({ "type": "none" });
            if let Some(src) = req.get("body") {
                match src.get("mode").and_then(Value::as_str).unwrap_or("") {
                    "raw" => {
                        let raw = src.get("raw").and_then(Value::as_str).unwrap_or("");
                        let trimmed = raw.trim();
                        let looks_like_json = (trimmed.starts_with('{') && trimmed.ends_with('}'))
                            || (trimmed.starts_with('[') && trimmed.ends_with(']'));
                        let kind = if looks_like_json
                            || src.pointer("/options/raw/language").and_then(Value::as_str)
                                == Some("json")
                        {
                            "json"
                        } else {
                            "raw"
                        };
                        body = json!({ "type": kind, "raw": raw });
                    }
                    "urlencoded" => {
                        body = json!({ "type": "urlEncoded", "urlEncoded": postman_kv(src.get("urlencoded")) })
                    }
                    "formdata" => {
                        body = json!({ "type": "formData", "formData": postman_kv(src.get("formdata")) })
                    }
                    "graphql" => {
                        body = json!({ "type": "graphql", "graphql": { "query": src.pointer("/graphql/query").and_then(Value::as_str).unwrap_or(""), "variables": src.pointer("/graphql/variables").and_then(Value::as_str).unwrap_or("") } })
                    }
                    _ => {}
                }
            }
            let auth = req
                .get("auth")
                .map(|auth| postman_auth(Some(auth)))
                .filter(|auth| {
                    auth.get("type")
                        .and_then(Value::as_str)
                        .is_some_and(|kind| kind != "none" && kind != "inherit")
                })
                .unwrap_or_else(|| inherited_auth.clone());
            let (scripts, warnings) = postman_scripts(item.get("event"));
            result.warnings.extend(warnings);
            let mut request = request_definition(
                &final_name,
                method,
                &url,
                headers,
                params,
                body,
                auth,
                req.get("description").and_then(Value::as_str).unwrap_or(""),
            );
            request["scripts"] = scripts;
            let filename = format!("{}.pebble.json", slug(&final_name));
            let relative = if parent.is_empty() {
                filename
            } else {
                format!("{parent}/{filename}")
            };
            result.requests.push((relative, request));
        } else {
            result.skipped.push(
                json!({ "name": name, "path": parent, "reason": "Item has no request definition" }),
            );
        }
    }
}

fn postman_kv(value: Option<&Value>) -> Vec<Value> {
    value
        .and_then(Value::as_array)
        .map(|items| {
            items
                .iter()
                .filter_map(|item| {
                    Some(json!({
                        "key": item.get("key")?.as_str()?,
                        "value": item.get("value").and_then(Value::as_str).unwrap_or(""),
                        "enabled": !item.get("disabled").and_then(Value::as_bool).unwrap_or(false),
                        "type": item.get("type").and_then(Value::as_str).unwrap_or("text")
                    }))
                })
                .collect()
        })
        .unwrap_or_default()
}

fn postman_auth(value: Option<&Value>) -> Value {
    let Some(auth) = value else {
        return json!({ "type": "none" });
    };
    match auth.get("type").and_then(Value::as_str).unwrap_or("none") {
        "bearer" => {
            json!({ "type": "bearer", "token": postman_auth_value(auth, "bearer", "token").unwrap_or("") })
        }
        "basic" => {
            json!({ "type": "basic", "username": postman_auth_value(auth, "basic", "username").unwrap_or(""), "password": postman_auth_value(auth, "basic", "password").unwrap_or("") })
        }
        "apikey" => {
            json!({ "type": "apiKey", "key": postman_auth_value(auth, "apikey", "key").unwrap_or(""), "value": postman_auth_value(auth, "apikey", "value").unwrap_or(""), "addTo": postman_auth_value(auth, "apikey", "in").unwrap_or("header") })
        }
        _ => json!({ "type": "none" }),
    }
}

fn postman_auth_value<'a>(auth: &'a Value, scheme: &str, key: &str) -> Option<&'a str> {
    auth.get(scheme)?
        .as_array()?
        .iter()
        .find(|entry| entry.get("key").and_then(Value::as_str) == Some(key))?
        .get("value")?
        .as_str()
}

fn postman_url(value: Option<&Value>) -> (String, Vec<Value>) {
    match value {
        Some(Value::String(url)) => (url.clone(), Vec::new()),
        Some(Value::Object(url)) => {
            let mut raw = url
                .get("raw")
                .and_then(Value::as_str)
                .unwrap_or("")
                .to_owned();
            let path_segments = match url.get("path") {
                Some(Value::Array(parts)) => parts
                    .iter()
                    .filter_map(Value::as_str)
                    .filter(|part| !part.is_empty())
                    .map(str::to_owned)
                    .collect::<Vec<_>>(),
                Some(Value::String(path)) if !path.is_empty() => path
                    .trim_matches('/')
                    .split('/')
                    .filter(|part| !part.is_empty())
                    .map(str::to_owned)
                    .collect(),
                _ => Vec::new(),
            };
            if !path_segments.is_empty() {
                let full_path = path_segments.join("/");
                if raw.is_empty() {
                    let host = match url.get("host") {
                        Some(Value::Array(parts)) => parts
                            .iter()
                            .filter_map(Value::as_str)
                            .collect::<Vec<_>>()
                            .join("."),
                        Some(Value::String(host)) => host.clone(),
                        _ => String::new(),
                    };
                    let protocol = url.get("protocol").and_then(Value::as_str).unwrap_or("");
                    raw = if !host.is_empty() && !protocol.is_empty() && !host.contains("://") {
                        format!("{protocol}://{host}/{full_path}")
                    } else if !host.is_empty() {
                        format!("{host}/{full_path}")
                    } else {
                        format!("/{full_path}")
                    };
                } else {
                    let ends_with_last = path_segments.last().is_some_and(|last| {
                        raw.trim_end_matches('/').ends_with(&format!("/{last}"))
                    });
                    if !raw.contains(&full_path) && !ends_with_last {
                        raw = format!("{}/{full_path}", raw.trim_end_matches('/'));
                    }
                }
            }
            let params = url.get("query").and_then(Value::as_array).map(|query| query.iter().filter_map(|item| {
                let key = item.get("key")?.as_str()?;
                if key.is_empty() { return None; }
                Some(json!({ "key": key, "value": item.get("value").and_then(Value::as_str).unwrap_or(""), "enabled": !item.get("disabled").and_then(Value::as_bool).unwrap_or(false) }))
            }).collect()).unwrap_or_default();
            (raw, params)
        }
        _ => (String::new(), Vec::new()),
    }
}

fn postman_scripts(events: Option<&Value>) -> (Value, Vec<String>) {
    let mut pre_request = String::new();
    let mut post_response = String::new();
    let mut warnings = Vec::new();
    let Some(events) = events.and_then(Value::as_array) else {
        return (
            json!({ "preRequest": pre_request, "postResponse": post_response }),
            warnings,
        );
    };
    for event in events {
        let raw = match event.pointer("/script/exec") {
            Some(Value::String(script)) => script.clone(),
            Some(Value::Array(lines)) => lines
                .iter()
                .filter_map(Value::as_str)
                .collect::<Vec<_>>()
                .join("\n"),
            _ => String::new(),
        };
        if raw.trim().is_empty() {
            continue;
        }
        let (converted, mut script_warnings) = convert_postman_script(&raw);
        warnings.append(&mut script_warnings);
        let listen = event.get("listen").and_then(Value::as_str).unwrap_or("");
        let target = if listen.eq_ignore_ascii_case("prerequest") {
            &mut pre_request
        } else if listen.eq_ignore_ascii_case("test") {
            &mut post_response
        } else {
            continue;
        };
        if !target.is_empty() {
            target.push_str("\n\n");
        }
        target.push_str(&converted);
    }
    (
        json!({ "preRequest": pre_request, "postResponse": post_response }),
        warnings,
    )
}

fn convert_postman_script(script: &str) -> (String, Vec<String>) {
    let mut output = Vec::new();
    let mut warnings = Vec::new();
    for (index, line) in script.lines().enumerate() {
        let trimmed = line.trim();
        if trimmed.is_empty() || trimmed.starts_with("//") {
            output.push(line.to_owned());
            continue;
        }
        let unsupported = [
            "sendRequest",
            "visualizer",
            "iterationData",
            "cookies",
            "info",
        ]
        .iter()
        .find(|api| line.contains(format!("pm.{api}").as_str()));
        if let Some(api) = unsupported {
            warnings.push(format!(
                "Line {}: Unsupported Postman API 'pm.{api}' cannot be automatically executed",
                index + 1
            ));
            output.push(format!("// [PebblePost] Could not auto-convert: {trimmed}"));
            output.push(format!("// {line}"));
            continue;
        }
        if let Some((indent, name, assertion)) = old_postman_test(line) {
            output.push(format!(
                "{indent}pb.test({name:?}, () => {{ pb.expect({assertion}).toBe(true); }});"
            ));
            continue;
        }

        let mut converted = line.to_owned();
        for (from, to) in [
            ("pm.environment.get", "pb.environment.get"),
            ("pm.environment.set", "pb.environment.set"),
            ("pm.variables.get", "pb.variables.get"),
            ("pm.collectionVariables.get", "pb.variables.get"),
            ("pm.collectionVariables.set", "pb.environment.set"),
            ("pm.globals.get", "pb.environment.get"),
            ("pm.globals.set", "pb.environment.set"),
            ("pm.response.json", "pb.response.json"),
            ("pm.response.text", "pb.response.text"),
            ("pm.test", "pb.test"),
            ("pm.expect", "pb.expect"),
            ("console.log", "pb.console.log"),
            ("console.info", "pb.console.info"),
            ("console.warn", "pb.console.warn"),
            ("console.error", "pb.console.error"),
        ] {
            converted = replace_js_call(&converted, from, to);
        }
        converted = converted
            .replace("pm.response.code", "pb.response.status")
            .replace("pm.response.status", "pb.response.status")
            .replace(").to.eql(", ").toEqual(")
            .replace(").to.equal(", ").toBe(")
            .replace(").to.be.true", ").toBe(true)")
            .replace(").to.be.false", ").toBe(false)");
        if converted.contains("pm.") && !converted.trim_start().starts_with("//") {
            warnings.push(format!(
                "Line {}: Unrecognized 'pm.*' call may require manual adjustment: {trimmed}",
                index + 1
            ));
            output.push(format!(
                "// [PebblePost] Note: Partial conversion — verify pm.* call:\n{converted}"
            ));
        } else {
            output.push(converted);
        }
    }
    (output.join("\n"), warnings)
}

fn replace_js_call(input: &str, from: &str, to: &str) -> String {
    let mut output = String::with_capacity(input.len());
    let mut cursor = 0;
    while let Some(relative) = input[cursor..].find(from) {
        let start = cursor + relative;
        let end = start + from.len();
        let suffix = &input[end..];
        let whitespace = suffix
            .chars()
            .take_while(|character| character.is_whitespace())
            .map(char::len_utf8)
            .sum::<usize>();
        if suffix[whitespace..].starts_with('(') {
            output.push_str(&input[cursor..start]);
            output.push_str(to);
            cursor = end;
        } else {
            output.push_str(&input[cursor..end]);
            cursor = end;
        }
    }
    output.push_str(&input[cursor..]);
    output
}

fn old_postman_test(line: &str) -> Option<(&str, String, String)> {
    let start = line.find("tests[")?;
    let equals = line[start..].find("] = ")? + start;
    let raw_name = line[start + "tests[".len()..equals].trim();
    let name = raw_name
        .trim_matches(|character| character == '\'' || character == '"')
        .to_owned();
    let assertion = line[equals + 4..].trim().trim_end_matches(';').to_owned();
    if name.is_empty() || assertion.is_empty() {
        return None;
    }
    Some((&line[..start], name, assertion))
}

pub fn parse_openapi_document(content: &str) -> Result<Value, String> {
    if let Ok(value) = serde_json::from_str(content) {
        return Ok(value);
    }
    serde_yaml_ng::from_str(content)
        .map_err(|error| format!("Invalid OpenAPI JSON/YAML specification: {error}"))
}

fn parse_openapi(content: &str) -> Result<ImportResult, String> {
    let value = parse_openapi_document(content)?;
    let version = value.get("openapi").and_then(Value::as_str).unwrap_or("");
    if !version.starts_with('3') {
        return Err(format!("only OpenAPI 3.x is supported (got {version:?})"));
    }
    let name = value
        .pointer("/info/title")
        .and_then(Value::as_str)
        .unwrap_or("OpenAPI Collection")
        .to_owned();
    let server = value
        .pointer("/servers/0/url")
        .and_then(Value::as_str)
        .unwrap_or("");
    let mut result = ImportResult {
        format: "openapi".into(),
        collection_name: name.clone(),
        requests: Vec::new(),
        folders: Vec::new(),
        variables: Vec::new(),
        skipped: Vec::new(),
        warnings: Vec::new(),
    };
    let Some(paths) = value.get("paths").and_then(Value::as_object) else {
        return Err("OpenAPI document has no paths".into());
    };
    for (path, path_item) in paths {
        let Some(operations) = path_item.as_object() else {
            continue;
        };
        for (method, operation) in operations {
            if !matches!(
                method.as_str(),
                "get" | "post" | "put" | "patch" | "delete" | "head" | "options"
            ) {
                continue;
            }
            let operation_id = operation
                .get("operationId")
                .and_then(Value::as_str)
                .unwrap_or("");
            let name = operation
                .get("summary")
                .and_then(Value::as_str)
                .filter(|value| !value.is_empty())
                .or_else(|| (!operation_id.is_empty()).then_some(operation_id))
                .unwrap_or(path);
            let url = format!("{}{}", server.trim_end_matches('/'), path);
            let mut parameters = path_item
                .get("parameters")
                .and_then(Value::as_array)
                .cloned()
                .unwrap_or_default();
            for parameter in operation
                .get("parameters")
                .and_then(Value::as_array)
                .into_iter()
                .flatten()
            {
                let key = (
                    parameter.get("name").and_then(Value::as_str).unwrap_or(""),
                    parameter.get("in").and_then(Value::as_str).unwrap_or(""),
                );
                parameters.retain(|existing| {
                    (
                        existing.get("name").and_then(Value::as_str).unwrap_or(""),
                        existing.get("in").and_then(Value::as_str).unwrap_or(""),
                    ) != key
                });
                parameters.push(parameter.clone());
            }
            let parameter_values = parameters
                .iter()
                .filter_map(|parameter| {
                    let name = parameter.get("name").and_then(Value::as_str)?;
                    let value = parameter
                        .get("example")
                        .map(openapi_value_string)
                        .or_else(|| {
                            parameter
                                .pointer("/schema/default")
                                .map(openapi_value_string)
                        })
                        .unwrap_or_default();
                    match parameter.get("in").and_then(Value::as_str) {
                        Some("header") => Some((
                            "header",
                            json!({ "key": name, "value": value, "enabled": true }),
                        )),
                        Some("query") => Some((
                            "query",
                            json!({ "key": name, "value": value, "enabled": true }),
                        )),
                        _ => None,
                    }
                })
                .collect::<Vec<_>>();
            let headers = parameter_values
                .iter()
                .filter(|(location, _)| *location == "header")
                .map(|(_, value)| value.clone())
                .collect();
            let query = parameter_values
                .iter()
                .filter(|(location, _)| *location == "query")
                .map(|(_, value)| value.clone())
                .collect();
            let mut body = json!({ "type": "none" });
            let content = operation.pointer("/requestBody/content");
            if let Some(media) = content.and_then(|content| content.get("application/json")) {
                let skeleton = openapi_json_skeleton(media.get("schema"));
                if let Ok(raw) = serde_json::to_string_pretty(&skeleton) {
                    body = json!({ "type": "json", "raw": raw });
                }
            } else if let Some(media) =
                content.and_then(|content| content.get("multipart/form-data"))
            {
                let fields = openapi_schema_fields(media.get("schema"));
                body = json!({ "type": "formData", "formData": fields });
            } else if let Some(media) =
                content.and_then(|content| content.get("application/x-www-form-urlencoded"))
            {
                let fields = openapi_schema_fields(media.get("schema"));
                body = json!({ "type": "urlEncoded", "urlEncoded": fields });
            }
            let auth = openapi_auth(operation, value.get("components"));
            let mut request = request_definition(
                name,
                &method.to_ascii_uppercase(),
                &url,
                headers,
                query,
                body,
                auth,
                operation
                    .get("description")
                    .and_then(Value::as_str)
                    .unwrap_or(""),
            );
            if !operation_id.is_empty() {
                request["id"] = json!(operation_id);
            }
            request["openApiParameters"] = json!(parameters);
            request["openApiResponses"] = operation
                .get("responses")
                .cloned()
                .unwrap_or_else(|| json!({}));
            let filename = format!("{}.pebble.json", slug(name));
            result.requests.push((filename, request));
        }
    }
    Ok(result)
}

fn openapi_value_string(value: &Value) -> String {
    value
        .as_str()
        .map(str::to_owned)
        .unwrap_or_else(|| value.to_string())
}

fn openapi_json_skeleton(schema: Option<&Value>) -> Value {
    let Some(schema) = schema else {
        return json!({});
    };
    if let Some(example) = schema.get("example") {
        return example.clone();
    }
    if let Some(default) = schema.get("default") {
        return default.clone();
    }
    match schema
        .get("type")
        .and_then(Value::as_str)
        .unwrap_or("object")
    {
        "object" => {
            let mut object = serde_json::Map::new();
            if let Some(properties) = schema.get("properties").and_then(Value::as_object) {
                for (key, property) in properties {
                    object.insert(key.clone(), openapi_json_skeleton(Some(property)));
                }
            }
            Value::Object(object)
        }
        "array" => schema
            .get("items")
            .map(|item| Value::Array(vec![openapi_json_skeleton(Some(item))]))
            .unwrap_or_else(|| json!([])),
        "string" => match schema.get("format").and_then(Value::as_str).unwrap_or("") {
            "date-time" => json!("2026-01-01T00:00:00Z"),
            "email" => json!("user@example.com"),
            _ => json!(""),
        },
        "integer" => json!(0),
        "number" => json!(0.0),
        "boolean" => json!(false),
        _ => Value::Null,
    }
}

fn openapi_schema_fields(schema: Option<&Value>) -> Vec<Value> {
    schema
        .and_then(|schema| schema.get("properties"))
        .and_then(Value::as_object)
        .map(|properties| {
            properties
                .keys()
                .map(|key| json!({ "key": key, "value": "", "enabled": true }))
                .collect()
        })
        .unwrap_or_default()
}

fn openapi_auth(operation: &Value, components: Option<&Value>) -> Value {
    let Some(security) = operation
        .get("security")
        .and_then(Value::as_array)
        .filter(|security| !security.is_empty())
    else {
        return json!({ "type": "inherit" });
    };
    let Some(schemes) = components
        .and_then(|components| components.get("securitySchemes"))
        .and_then(Value::as_object)
    else {
        return json!({ "type": "inherit" });
    };
    for requirement in security {
        let Some(requirement) = requirement.as_object() else {
            continue;
        };
        for name in requirement.keys() {
            let Some(scheme) = schemes.get(name) else {
                continue;
            };
            match scheme
                .get("type")
                .and_then(Value::as_str)
                .unwrap_or("")
                .to_ascii_lowercase()
                .as_str()
            {
                "http" => match scheme
                    .get("scheme")
                    .and_then(Value::as_str)
                    .unwrap_or("")
                    .to_ascii_lowercase()
                    .as_str()
                {
                    "bearer" => return json!({ "type": "bearer" }),
                    "basic" => return json!({ "type": "basic" }),
                    _ => {}
                },
                "apikey" => {
                    return json!({ "type": "apiKey", "key": scheme.get("name").and_then(Value::as_str).unwrap_or(""), "addTo": scheme.get("in").and_then(Value::as_str).unwrap_or("header") });
                }
                "oauth2" => return json!({ "type": "oauth2" }),
                _ => {}
            }
        }
    }
    json!({ "type": "inherit" })
}

fn parse_har(content: &str) -> Result<ImportResult, String> {
    let value: Value =
        serde_json::from_str(content).map_err(|error| format!("Invalid HAR JSON: {error}"))?;
    let entries = value
        .pointer("/log/entries")
        .and_then(Value::as_array)
        .ok_or_else(|| "HAR file does not contain log.entries".to_owned())?;
    let creator = value
        .pointer("/log/creator/name")
        .and_then(Value::as_str)
        .filter(|name| !name.is_empty());
    let mut result = ImportResult {
        format: "har".into(),
        collection_name: creator
            .map(|name| format!("HAR ({name})"))
            .unwrap_or_else(|| "HAR Import".into()),
        requests: Vec::new(),
        folders: Vec::new(),
        variables: Vec::new(),
        skipped: Vec::new(),
        warnings: Vec::new(),
    };
    let mut seen_names = BTreeMap::<String, usize>::new();
    for (index, entry) in entries.iter().enumerate() {
        let req = &entry["request"];
        let raw_url = req.get("url").and_then(Value::as_str).unwrap_or("").trim();
        if raw_url.is_empty() {
            result.skipped.push(json!({ "name": format!("Entry #{}", index + 1), "reason": "Missing URL in HAR entry" }));
            continue;
        }
        let method = req
            .get("method")
            .and_then(Value::as_str)
            .filter(|method| !method.trim().is_empty())
            .unwrap_or("GET")
            .trim()
            .to_ascii_uppercase();
        let parsed_url = reqwest::Url::parse(raw_url).ok();
        let path = parsed_url.as_ref().map(reqwest::Url::path).unwrap_or("/");
        let basename = Path::new(path)
            .file_name()
            .and_then(|name| name.to_str())
            .filter(|name| !name.is_empty())
            .unwrap_or_else(|| {
                parsed_url
                    .as_ref()
                    .and_then(reqwest::Url::host_str)
                    .unwrap_or("request")
            });
        let base_name = format!("{method} {basename}");
        let occurrence = seen_names.entry(base_name.clone()).or_insert(0);
        *occurrence += 1;
        let name = if *occurrence == 1 {
            base_name
        } else {
            format!("{base_name} ({occurrence})")
        };

        let mut url = raw_url.to_owned();
        let mut params = Vec::new();
        if let Some(query) = req
            .get("queryString")
            .and_then(Value::as_array)
            .filter(|items| !items.is_empty())
        {
            if let Some(parsed) = parsed_url.clone() {
                let mut cleaned = parsed;
                cleaned.set_query(None);
                cleaned.set_fragment(None);
                url = cleaned.to_string();
            }
            params = query
                .iter()
                .map(|item| {
                    json!({
                        "key": item.get("name").and_then(Value::as_str).unwrap_or(""),
                        "value": item.get("value").and_then(Value::as_str).unwrap_or(""),
                        "enabled": true
                    })
                })
                .collect();
        }

        let mut headers = Vec::new();
        let mut auth = json!({ "type": "none" });
        if let Some(items) = req.get("headers").and_then(Value::as_array) {
            for item in items {
                let key = item.get("name").and_then(Value::as_str).unwrap_or("");
                let value = item.get("value").and_then(Value::as_str).unwrap_or("");
                if key.starts_with(':') {
                    continue;
                }
                if key.eq_ignore_ascii_case("authorization")
                    && let Some((scheme, credential)) = value.split_once(' ')
                {
                    if scheme.eq_ignore_ascii_case("bearer") {
                        auth = json!({ "type": "bearer", "token": credential });
                        continue;
                    }
                    if scheme.eq_ignore_ascii_case("basic") {
                        auth = json!({ "type": "basic", "token": credential });
                        continue;
                    }
                }
                headers.push(json!({ "key": key, "value": value, "enabled": true }));
            }
        }

        let post_data = req.get("postData");
        let mime = post_data
            .and_then(|data| data.get("mimeType"))
            .and_then(Value::as_str)
            .unwrap_or("")
            .to_ascii_lowercase();
        let text = post_data
            .and_then(|data| data.get("text"))
            .and_then(Value::as_str)
            .unwrap_or("");
        let post_params = post_data
            .and_then(|data| data.get("params"))
            .and_then(Value::as_array);
        let body = if mime.contains("application/json") {
            json!({ "type": "json", "raw": text })
        } else if mime.contains("application/x-www-form-urlencoded") {
            let fields = if let Some(fields) = post_params {
                fields
                    .iter()
                    .map(|field| {
                        json!({
                            "key": field.get("name").and_then(Value::as_str).unwrap_or(""),
                            "value": field.get("value").and_then(Value::as_str).unwrap_or(""),
                            "enabled": true
                        })
                    })
                    .collect::<Vec<_>>()
            } else {
                reqwest::Url::parse(&format!("http://mock.local/?{text}")).map(|url| url.query_pairs().map(|(key, value)| json!({ "key": key, "value": value, "enabled": true })).collect()).unwrap_or_default()
            };
            json!({ "type": "urlEncoded", "urlEncoded": fields })
        } else if mime.contains("multipart/form-data") {
            let fields = post_params.into_iter().flatten().map(|field| json!({
                "key": field.get("name").and_then(Value::as_str).unwrap_or(""),
                "value": field.get("value").and_then(Value::as_str).unwrap_or(""),
                "type": if field.get("fileName").and_then(Value::as_str).is_some_and(|name| !name.is_empty()) { "file" } else { "text" },
                "enabled": true
            })).collect::<Vec<_>>();
            json!({ "type": "formData", "formData": fields })
        } else if !text.is_empty() {
            json!({ "type": "raw", "raw": text })
        } else {
            json!({ "type": "none" })
        };
        let request = request_definition(
            &name,
            &method,
            &url,
            headers,
            params,
            body,
            auth,
            "Imported from HAR",
        );
        result
            .requests
            .push((format!("{}.pebble.json", slug(&name)), request));
    }
    Ok(result)
}

fn parse_insomnia(content: &str) -> Result<ImportResult, String> {
    let value: Value =
        serde_json::from_str(content).map_err(|error| format!("Invalid Insomnia JSON: {error}"))?;
    let resources = value
        .get("resources")
        .and_then(Value::as_array)
        .ok_or_else(|| "Insomnia export has no resources".to_owned())?;
    let collection_name = resources
        .iter()
        .find(|item| item.get("_type").and_then(Value::as_str) == Some("workspace"))
        .and_then(|item| item.get("name"))
        .and_then(Value::as_str)
        .filter(|name| !name.is_empty())
        .unwrap_or("Insomnia Collection");
    let groups = resources
        .iter()
        .filter(|item| item.get("_type").and_then(Value::as_str) == Some("request_group"))
        .filter_map(|item| Some((item.get("_id")?.as_str()?.to_owned(), item)))
        .collect::<BTreeMap<_, _>>();
    let mut result = ImportResult {
        format: "insomnia".into(),
        collection_name: collection_name.to_owned(),
        requests: Vec::new(),
        folders: Vec::new(),
        variables: Vec::new(),
        skipped: Vec::new(),
        warnings: Vec::new(),
    };
    if let Some(environment) = resources
        .iter()
        .find(|item| item.get("_type").and_then(Value::as_str) == Some("environment"))
        .and_then(|item| item.get("data"))
        .and_then(Value::as_object)
    {
        result.variables = environment
            .iter()
            .map(|(key, value)| {
                json!({
                    "key": key,
                    "value": value.as_str().map(str::to_owned).unwrap_or_else(|| value.to_string()),
                    "enabled": true
                })
            })
            .collect();
    }
    if !result.variables.is_empty() {
        result.folders.push((
            ".".into(),
            json!({
                "schemaVersion": 1,
                "name": result.collection_name.clone(),
                "auth": { "type": "inherit" },
                "headers": [],
                "variables": result.variables.clone(),
                "scripts": { "preRequest": "", "postResponse": "" }
            }),
        ));
    }
    let mut seen_folders = Vec::new();
    for item in resources
        .iter()
        .filter(|item| item.get("_type").and_then(Value::as_str) == Some("request"))
    {
        let source_name = item.get("name").and_then(Value::as_str).unwrap_or("");
        let url = item.get("url").and_then(Value::as_str).unwrap_or("");
        if source_name.trim().is_empty() && url.trim().is_empty() {
            result
                .skipped
                .push(json!({ "name": source_name, "reason": "Empty request URL and name" }));
            continue;
        }
        let name = if source_name.is_empty() {
            format!(
                "{} Request",
                item.get("method").and_then(Value::as_str).unwrap_or("GET")
            )
        } else {
            source_name.to_owned()
        };
        let method = item
            .get("method")
            .and_then(Value::as_str)
            .filter(|method| !method.is_empty())
            .unwrap_or("GET")
            .to_ascii_uppercase();
        let headers = item
            .get("headers")
            .and_then(Value::as_array)
            .map(|items| {
                items.iter().map(|header| json!({
            "key": header.get("name").and_then(Value::as_str).unwrap_or(""),
            "value": header.get("value").and_then(Value::as_str).unwrap_or(""),
            "enabled": !header.get("disabled").and_then(Value::as_bool).unwrap_or(false)
        })).collect()
            })
            .unwrap_or_default();
        let params = item
            .get("parameters")
            .and_then(Value::as_array)
            .map(|items| {
                items.iter().map(|param| json!({
            "key": param.get("name").and_then(Value::as_str).unwrap_or(""),
            "value": param.get("value").and_then(Value::as_str).unwrap_or(""),
            "enabled": !param.get("disabled").and_then(Value::as_bool).unwrap_or(false)
        })).collect()
            })
            .unwrap_or_default();
        let auth = insomnia_auth(item.get("authentication"));
        let body = insomnia_body(item.get("body"));
        let mut request = request_definition(
            &name,
            &method,
            url,
            headers,
            params,
            body,
            auth,
            item.get("description")
                .and_then(Value::as_str)
                .unwrap_or(""),
        );
        let chain = insomnia_group_chain(
            item.get("parentId").and_then(Value::as_str).unwrap_or(""),
            &groups,
        );
        let folder_path = chain
            .iter()
            .filter_map(|group| group.get("name").and_then(Value::as_str))
            .map(slug)
            .collect::<Vec<_>>()
            .join("/");
        let mut components = Vec::new();
        for group in &chain {
            let group_name = group
                .get("name")
                .and_then(Value::as_str)
                .unwrap_or("Folder");
            components.push(slug(group_name));
            let path = components.join("/");
            if !seen_folders.contains(&path) {
                seen_folders.push(path.clone());
                result.folders.push((path, json!({
                    "schemaVersion": 1,
                    "name": group_name,
                    "description": group.get("description").and_then(Value::as_str).unwrap_or(""),
                    "auth": { "type": "inherit" },
                    "headers": [],
                    "variables": [],
                    "scripts": { "preRequest": "", "postResponse": "" }
                })));
            }
        }
        let relative = if folder_path.is_empty() {
            format!("{}.pebble.json", slug(&name))
        } else {
            format!("{folder_path}/{}.pebble.json", slug(&name))
        };
        request["scripts"] = json!({ "preRequest": "", "postResponse": "" });
        result.requests.push((relative, request));
    }
    Ok(result)
}

fn insomnia_group_chain<'a>(
    parent_id: &str,
    groups: &BTreeMap<String, &'a Value>,
) -> Vec<&'a Value> {
    let mut chain = Vec::new();
    let mut visited = Vec::new();
    let mut current = parent_id;
    while !current.is_empty() && !visited.iter().any(|id: &String| id.as_str() == current) {
        visited.push(current.to_owned());
        let Some(group) = groups.get(current).copied() else {
            break;
        };
        chain.push(group);
        current = group.get("parentId").and_then(Value::as_str).unwrap_or("");
    }
    chain.reverse();
    chain
}

fn insomnia_auth(value: Option<&Value>) -> Value {
    let Some(auth) = value else {
        return json!({ "type": "none" });
    };
    match auth.get("type").and_then(Value::as_str).unwrap_or("") {
        "bearer" => {
            json!({ "type": "bearer", "token": auth.get("token").and_then(Value::as_str).unwrap_or("") })
        }
        "basic" => {
            json!({ "type": "basic", "username": auth.get("username").and_then(Value::as_str).unwrap_or(""), "password": auth.get("password").and_then(Value::as_str).unwrap_or("") })
        }
        "apikey" => {
            json!({ "type": "apiKey", "key": auth.get("key").and_then(Value::as_str).unwrap_or(""), "value": auth.get("value").and_then(Value::as_str).unwrap_or(""), "addTo": auth.get("addTo").and_then(Value::as_str).unwrap_or("header") })
        }
        _ => json!({ "type": "none" }),
    }
}

fn insomnia_body(value: Option<&Value>) -> Value {
    let Some(body) = value else {
        return json!({ "type": "none" });
    };
    let mime = body
        .get("mimeType")
        .and_then(Value::as_str)
        .unwrap_or("")
        .to_ascii_lowercase();
    let text = body.get("text").and_then(Value::as_str).unwrap_or("");
    if mime.contains("application/json") {
        return json!({ "type": "json", "raw": text });
    }
    if mime.contains("application/x-www-form-urlencoded") {
        let fields = body
            .get("params")
            .and_then(Value::as_array)
            .map(|items| {
                items.iter().map(|item| json!({
            "key": item.get("name").and_then(Value::as_str).unwrap_or(""),
            "value": item.get("value").and_then(Value::as_str).unwrap_or(""),
            "enabled": !item.get("disabled").and_then(Value::as_bool).unwrap_or(false)
        })).collect::<Vec<_>>()
            })
            .unwrap_or_default();
        return json!({ "type": "urlEncoded", "urlEncoded": fields });
    }
    if mime.contains("multipart/form-data") {
        let fields = body.get("params").and_then(Value::as_array).map(|items| items.iter().map(|item| {
            let file = item.get("type").and_then(Value::as_str) == Some("file") || item.get("fileName").and_then(Value::as_str).is_some_and(|name| !name.is_empty());
            json!({
                "key": item.get("name").and_then(Value::as_str).unwrap_or(""),
                "value": item.get("value").and_then(Value::as_str).unwrap_or(""),
                "enabled": !item.get("disabled").and_then(Value::as_bool).unwrap_or(false),
                "type": if file { "file" } else { "text" }
            })
        }).collect::<Vec<_>>()).unwrap_or_default();
        return json!({ "type": "formData", "formData": fields });
    }
    if mime.contains("graphql") {
        return json!({ "type": "graphql", "raw": text });
    }
    if !text.is_empty() {
        return json!({ "type": "raw", "raw": text });
    }
    json!({ "type": "none" })
}

fn parse_bruno(content: &str) -> Result<ImportResult, String> {
    parse_bruno_named(content, "Imported Request")
}

fn parse_bruno_named(content: &str, default_name: &str) -> Result<ImportResult, String> {
    let blocks = bruno_blocks(content);
    if blocks.is_empty() {
        return Err("Bruno request is empty".into());
    }
    let name = blocks
        .get("meta")
        .and_then(|lines| {
            lines.iter().find_map(|line| {
                let (key, value) = split_bruno_pair(line);
                (key == "name" && !value.is_empty()).then_some(value)
            })
        })
        .unwrap_or_else(|| default_name.to_owned());
    let mut method = "GET".to_owned();
    let mut url = String::new();
    let mut requested_body_type = String::new();
    let mut auth = json!({ "type": "none" });
    let mut order = None;
    for verb in ["get", "post", "put", "patch", "delete", "options", "head"] {
        if let Some(lines) = blocks.get(verb) {
            method = verb.to_ascii_uppercase();
            for line in lines {
                let (key, value) = split_bruno_pair(line);
                match key.as_str() {
                    "url" => url = value,
                    "body" => requested_body_type = value,
                    "auth" if value == "bearer" || value == "basic" => auth["type"] = json!(value),
                    _ => {}
                }
            }
            break;
        }
    }
    if let Some(lines) = blocks.get("meta") {
        for line in lines {
            let (key, value) = split_bruno_pair(line);
            if key == "seq" {
                order = value.parse::<i64>().ok();
            }
        }
    }
    let headers = blocks
        .get("headers")
        .map(|lines| {
            lines
                .iter()
                .filter_map(|line| bruno_key_value(line))
                .collect::<Vec<_>>()
        })
        .unwrap_or_default();
    let params = blocks
        .get("params:query")
        .map(|lines| {
            lines
                .iter()
                .filter_map(|line| bruno_key_value(line))
                .collect::<Vec<_>>()
        })
        .unwrap_or_default();
    if let Some(lines) = blocks.get("auth:bearer") {
        auth["type"] = json!("bearer");
        if let Some(token) = lines.iter().find_map(|line| {
            let (key, value) = split_bruno_pair(line);
            (key == "token").then_some(value)
        }) {
            auth["token"] = json!(token);
        }
    } else if let Some(lines) = blocks.get("auth:basic") {
        auth["type"] = json!("basic");
        for line in lines {
            let (key, value) = split_bruno_pair(line);
            if key == "username" || key == "password" {
                auth[key.as_str()] = json!(value);
            }
        }
    }
    let mut body = if requested_body_type.is_empty() || requested_body_type == "none" {
        json!({ "type": "none" })
    } else {
        json!({ "type": requested_body_type })
    };
    for (block, body_type) in [
        ("body:json", "json"),
        ("body:text", "raw"),
        ("body:xml", "xml"),
        ("body:graphql", "graphql"),
    ] {
        if let Some(lines) = blocks.get(block) {
            body = json!({ "type": body_type, "raw": lines.join("\n") });
        }
    }
    if let Some(lines) = blocks.get("body:form-urlencoded") {
        let fields = lines
            .iter()
            .filter_map(|line| bruno_key_value(line))
            .collect::<Vec<_>>();
        body = json!({ "type": "urlEncoded", "urlEncoded": fields });
    }
    let mut request = request_definition(&name, &method, &url, headers, params, body, auth, "");
    if let Some(order) = order {
        request["order"] = json!(order);
    }
    if let Some(lines) = blocks.get("script:pre-request") {
        request["scripts"]["preRequest"] = json!(lines.join("\n"));
    }
    if let Some(lines) = blocks.get("tests") {
        request["scripts"]["postResponse"] = json!(lines.join("\n"));
    }
    let mut result = ImportResult {
        format: "bruno".into(),
        collection_name: "Bruno Import".into(),
        requests: Vec::new(),
        folders: Vec::new(),
        variables: Vec::new(),
        skipped: Vec::new(),
        warnings: Vec::new(),
    };
    result
        .requests
        .push((format!("{}.pebble.json", slug(&name)), request));
    Ok(result)
}

pub fn parse_bruno_collection(path: &Path) -> Result<ImportResult, String> {
    let metadata = fs::metadata(path)
        .map_err(|error| format!("Bruno import path could not be read: {error}"))?;
    if !metadata.is_dir() {
        let content = fs::read_to_string(path)
            .map_err(|error| format!("Bruno request could not be read: {error}"))?;
        let name = path
            .file_stem()
            .and_then(|name| name.to_str())
            .unwrap_or("request");
        let mut result = parse_bruno_named(&content, name)?;
        result.collection_name = name.to_owned();
        if let Some((relative_path, request)) = result.requests.first_mut() {
            let destination = format!(
                "{}.pebble.json",
                slug(request.get("name").and_then(Value::as_str).unwrap_or(name))
            );
            *relative_path = destination;
        }
        return Ok(result);
    }

    let mut collection_name = path
        .file_name()
        .and_then(|name| name.to_str())
        .unwrap_or("Bruno Collection")
        .to_owned();
    let config = path.join("bruno.json");
    if let Ok(data) = fs::read(&config)
        && let Ok(value) = serde_json::from_slice::<Value>(&data)
        && let Some(name) = value
            .get("name")
            .and_then(Value::as_str)
            .filter(|name| !name.is_empty())
    {
        collection_name = name.to_owned();
    }
    let mut result = ImportResult {
        format: "bruno".into(),
        collection_name,
        requests: Vec::new(),
        folders: Vec::new(),
        variables: Vec::new(),
        skipped: Vec::new(),
        warnings: Vec::new(),
    };
    walk_bruno_collection(path, path, &mut result)?;
    Ok(result)
}

fn walk_bruno_collection(
    base: &Path,
    current: &Path,
    result: &mut ImportResult,
) -> Result<(), String> {
    let mut entries = fs::read_dir(current)
        .map_err(|error| format!("Bruno collection folder could not be read: {error}"))?
        .collect::<Result<Vec<_>, _>>()
        .map_err(|error| format!("Bruno collection entry could not be read: {error}"))?;
    entries.sort_by_key(|entry| entry.file_name());
    for entry in entries {
        let name = entry.file_name().to_string_lossy().into_owned();
        let path = entry.path();
        let file_type = entry.file_type().map_err(|error| error.to_string())?;
        if file_type.is_dir() {
            if name.starts_with('.') || name == "node_modules" {
                continue;
            }
            walk_bruno_collection(base, &path, result)?;
            continue;
        }
        if !file_type.is_file() || !name.ends_with(".bru") {
            continue;
        }
        let relative = path
            .strip_prefix(base)
            .unwrap_or(&path)
            .to_string_lossy()
            .replace('\\', "/");
        let content = match fs::read_to_string(&path) {
            Ok(content) => content,
            Err(error) => {
                result
                    .skipped
                    .push(json!({ "name": name, "path": relative, "reason": error.to_string() }));
                continue;
            }
        };
        let base_name = path
            .file_stem()
            .and_then(|name| name.to_str())
            .unwrap_or("request");
        let parsed = match parse_bruno_named(&content, base_name) {
            Ok(parsed) => parsed,
            Err(error) => {
                result
                    .skipped
                    .push(json!({ "name": base_name, "path": relative, "reason": error }));
                continue;
            }
        };
        let Some((_, request)) = parsed.requests.into_iter().next() else {
            continue;
        };
        let parent = Path::new(&relative)
            .parent()
            .filter(|parent| !parent.as_os_str().is_empty())
            .map(|parent| parent.to_string_lossy().replace('\\', "/"));
        let filename = format!(
            "{}.pebble.json",
            slug(
                request
                    .get("name")
                    .and_then(Value::as_str)
                    .unwrap_or(base_name)
            )
        );
        let destination = parent
            .map(|parent| format!("{parent}/{filename}"))
            .unwrap_or(filename);
        result.requests.push((destination, request));
    }
    Ok(())
}

fn bruno_blocks(content: &str) -> BTreeMap<String, Vec<String>> {
    let mut blocks = BTreeMap::new();
    let mut current: Option<String> = None;
    let mut lines = Vec::new();
    for line in content.lines() {
        let trimmed = line.trim();
        if let Some(block) = current.as_ref() {
            if trimmed == "}" {
                blocks.insert(block.clone(), std::mem::take(&mut lines));
                current = None;
            } else {
                lines.push(line.to_owned());
            }
        } else if trimmed.ends_with('{') {
            current = Some(trimmed.trim_end_matches('{').trim().to_owned());
        }
    }
    blocks
}

fn split_bruno_pair(line: &str) -> (String, String) {
    match line.split_once(':') {
        Some((key, value)) => (key.trim().to_owned(), value.trim().to_owned()),
        None => (line.trim().to_owned(), String::new()),
    }
}

fn bruno_key_value(line: &str) -> Option<Value> {
    let mut line = line.trim();
    let enabled = !line.starts_with('~');
    if !enabled {
        line = line.strip_prefix('~').unwrap_or(line).trim_start();
    }
    let (key, value) = line.split_once(':')?;
    if key.trim().is_empty() {
        return None;
    }
    Some(json!({ "key": key.trim(), "value": value.trim(), "enabled": enabled }))
}

fn parse_curl(content: &str) -> Result<ImportResult, String> {
    let tokens = shell_tokens(content)?;
    if !tokens.first().is_some_and(|token| {
        token.eq_ignore_ascii_case("curl") || token.eq_ignore_ascii_case("curl.exe")
    }) {
        return Err("Expected a cURL command".into());
    }
    let mut method = "GET".to_owned();
    let mut url = String::new();
    let mut headers = Vec::new();
    let mut auth = json!({ "type": "none" });
    let mut body = json!({ "type": "none" });
    let mut form_fields = Vec::new();
    let mut settings = json!({ "followRedirects": true, "verifySSL": true, "timeoutMs": 30000 });
    let mut i = 1;
    while i < tokens.len() {
        match tokens[i].as_str() {
            "-X" | "--request" => {
                i += 1;
                if let Some(value) = tokens.get(i) {
                    method = value.to_ascii_uppercase();
                }
            }
            "--url" => {
                i += 1;
                if let Some(value) = tokens.get(i) {
                    url = value.to_owned();
                }
            }
            flag if flag.starts_with("--url=") => url = flag[6..].to_owned(),
            flag if flag.starts_with("--request=") => method = flag[10..].to_ascii_uppercase(),
            flag if flag.starts_with("--header=") => {
                if let Some((key, value)) = flag[9..].split_once(':') {
                    let key = key.trim();
                    let value = value.trim();
                    if key.eq_ignore_ascii_case("authorization") {
                        if let Some((scheme, credential)) = value.split_once(' ') {
                            if scheme.eq_ignore_ascii_case("bearer") {
                                auth = json!({ "type": "bearer", "token": credential });
                            } else if scheme.eq_ignore_ascii_case("basic") {
                                let decoded = base64::engine::general_purpose::STANDARD
                                    .decode(credential)
                                    .or_else(|_| {
                                        base64::engine::general_purpose::STANDARD_NO_PAD
                                            .decode(credential)
                                    });
                                if let Ok(decoded) = decoded {
                                    if let Ok(decoded) = String::from_utf8(decoded) {
                                        if let Some((username, password)) = decoded.split_once(':')
                                        {
                                            auth = json!({ "type": "basic", "username": username, "password": password });
                                        } else {
                                            headers.push(json!({ "key": key, "value": value, "enabled": true }));
                                        }
                                    } else {
                                        headers.push(
                                            json!({ "key": key, "value": value, "enabled": true }),
                                        );
                                    }
                                } else {
                                    headers.push(
                                        json!({ "key": key, "value": value, "enabled": true }),
                                    );
                                }
                            } else {
                                headers
                                    .push(json!({ "key": key, "value": value, "enabled": true }));
                            }
                        } else {
                            headers.push(json!({ "key": key, "value": value, "enabled": true }));
                        }
                    } else {
                        headers.push(json!({ "key": key, "value": value, "enabled": true }));
                    }
                }
            }
            flag if flag.starts_with("--data=")
                || flag.starts_with("--data-raw=")
                || flag.starts_with("--data-binary=") =>
            {
                let raw = flag.split_once('=').map(|(_, value)| value).unwrap_or("");
                let trimmed = raw.trim();
                let kind = if trimmed.starts_with('{') || trimmed.starts_with('[') {
                    "json"
                } else {
                    "raw"
                };
                body = json!({ "type": kind, "raw": raw });
                if method == "GET" {
                    method = "POST".into();
                }
            }
            "-H" | "--header" => {
                i += 1;
                if let Some((key, value)) = tokens.get(i).and_then(|value| value.split_once(':')) {
                    let key = key.trim();
                    let value = value.trim();
                    if key.eq_ignore_ascii_case("authorization") {
                        if let Some((scheme, credential)) = value.split_once(' ') {
                            if scheme.eq_ignore_ascii_case("bearer") {
                                auth = json!({ "type": "bearer", "token": credential });
                            } else if scheme.eq_ignore_ascii_case("basic") {
                                let decoded = base64::engine::general_purpose::STANDARD
                                    .decode(credential)
                                    .or_else(|_| {
                                        base64::engine::general_purpose::STANDARD_NO_PAD
                                            .decode(credential)
                                    });
                                if let Ok(decoded) = decoded {
                                    if let Ok(decoded) = String::from_utf8(decoded) {
                                        if let Some((username, password)) = decoded.split_once(':')
                                        {
                                            auth = json!({ "type": "basic", "username": username, "password": password });
                                        } else {
                                            headers.push(json!({ "key": key, "value": value, "enabled": true }));
                                        }
                                    } else {
                                        headers.push(
                                            json!({ "key": key, "value": value, "enabled": true }),
                                        );
                                    }
                                } else {
                                    headers.push(
                                        json!({ "key": key, "value": value, "enabled": true }),
                                    );
                                }
                            } else {
                                headers
                                    .push(json!({ "key": key, "value": value, "enabled": true }));
                            }
                        } else {
                            headers.push(json!({ "key": key, "value": value, "enabled": true }));
                        }
                    } else {
                        headers.push(json!({ "key": key, "value": value, "enabled": true }));
                    }
                }
            }
            "-d" | "--data" | "--data-raw" | "--data-binary" => {
                i += 1;
                if let Some(raw) = tokens.get(i) {
                    let trimmed = raw.trim();
                    let kind = if trimmed.starts_with('{') || trimmed.starts_with('[') {
                        "json"
                    } else {
                        "raw"
                    };
                    body = json!({ "type": kind, "raw": raw });
                    if method == "GET" {
                        method = "POST".into();
                    }
                }
            }
            "--form" | "-F" => {
                i += 1;
                if let Some(field) = tokens.get(i)
                    && let Some((key, value)) = field.split_once('=')
                {
                    form_fields.push(json!({ "key": key, "value": value, "type": if value.starts_with('@') { "file" } else { "text" }, "enabled": true }));
                }
            }
            "-u" | "--user" => {
                i += 1;
                if let Some((username, password)) =
                    tokens.get(i).and_then(|value| value.split_once(':'))
                {
                    auth = json!({ "type": "basic", "username": username, "password": password });
                }
            }
            "--insecure" | "-k" => settings["verifySSL"] = json!(false),
            "-L" | "--location" => settings["followRedirects"] = json!(true),
            "--max-time" => {
                i += 1;
                if let Some(seconds) = tokens.get(i).and_then(|value| value.parse::<u64>().ok()) {
                    settings["timeoutMs"] = json!(seconds.saturating_mul(1000));
                }
            }
            "--compressed" | "-s" | "--silent" | "-v" | "--verbose" | "-i" | "--include" => {}
            flag if flag.starts_with('-') => {}
            value if url.is_empty() => url = value.to_owned(),
            _ => {}
        }
        i += 1;
    }
    if url.is_empty() {
        return Err("No URL found in cURL command".into());
    }
    if !form_fields.is_empty() {
        body = json!({ "type": "formData", "formData": form_fields });
    }

    let mut params = Vec::new();
    if let Ok(mut parsed_url) = reqwest::Url::parse(&url)
        && parsed_url.query().is_some()
    {
        params.extend(
            parsed_url
                .query_pairs()
                .map(|(key, value)| json!({ "key": key, "value": value, "enabled": true })),
        );
        parsed_url.set_query(None);
        url = parsed_url.to_string();
    }
    let name_part = url.trim_end_matches('/').rsplit('/').next().unwrap_or("");
    let name = if name_part.is_empty() {
        format!("{method} Request")
    } else {
        format!("{method} {name_part}")
    };
    let mut result = ImportResult {
        format: "curl".into(),
        collection_name: "cURL Import".into(),
        requests: Vec::new(),
        folders: Vec::new(),
        variables: Vec::new(),
        skipped: Vec::new(),
        warnings: Vec::new(),
    };
    let mut request = request_definition(&name, &method, &url, headers, params, body, auth, "");
    request["settings"] = settings;
    result
        .requests
        .push((format!("{}.pebble.json", slug(&name)), request));
    Ok(result)
}

fn shell_tokens(input: &str) -> Result<Vec<String>, String> {
    let chars = input.chars().collect::<Vec<_>>();
    let mut tokens = Vec::new();
    let mut current = String::new();
    let mut single = false;
    let mut double = false;
    let mut index = 0;
    while index < chars.len() {
        let ch = chars[index];
        if ch == '\'' && !double {
            single = !single;
        } else if ch == '"' && !single {
            double = !double;
        } else if ch == '\\' && (single || double) {
            if index + 1 < chars.len() {
                index += 1;
                current.push(chars[index]);
            }
        } else if ch.is_whitespace() && !single && !double {
            if !current.is_empty() {
                tokens.push(std::mem::take(&mut current));
            }
            if ch == '\r' && chars.get(index + 1) == Some(&'\n') {
                index += 1;
            }
            if chars.get(index + 1) == Some(&'\\') && chars.get(index + 2) == Some(&'\n') {
                index += 2;
                while chars.get(index + 1).is_some_and(|ch| ch.is_whitespace()) {
                    index += 1;
                }
            }
        } else {
            current.push(ch);
        }
        index += 1;
    }
    if single || double {
        return Err("Unclosed quote in cURL command".into());
    }
    if !current.is_empty() {
        tokens.push(current);
    }
    Ok(tokens)
}

#[allow(clippy::too_many_arguments)]
fn request_definition(
    name: &str,
    method: &str,
    url: &str,
    headers: Vec<Value>,
    params: Vec<Value>,
    body: Value,
    auth: Value,
    description: &str,
) -> Value {
    json!({ "$schema": "https://pebblepost.dev/schemas/v1/request.json", "schemaVersion": 1, "version": "1.0", "name": name, "description": description, "protocol": "http", "method": method, "url": url, "headers": headers, "params": params, "auth": auth, "body": body, "scripts": { "preRequest": "", "postResponse": "" }, "settings": { "followRedirects": true, "verifySSL": true, "timeoutMs": 30000 } })
}

fn collect_export(
    root: &Path,
    directory: &Path,
    base: &Path,
    requests: &mut Vec<Value>,
    folders: &mut Vec<Value>,
) -> Result<(), PebbleError> {
    let mut entries = fs::read_dir(directory)?.collect::<Result<Vec<_>, _>>()?;
    entries.sort_by_key(|entry| entry.file_name().to_string_lossy().to_ascii_lowercase());
    for entry in entries {
        let path = entry.path();
        let name = entry.file_name().to_string_lossy().into_owned();
        if entry.file_type()?.is_dir() {
            let rel = path
                .strip_prefix(base)
                .unwrap_or(&path)
                .to_string_lossy()
                .replace('\\', "/");
            let folder = read_folder(root, &path).unwrap_or_else(|_| json!({ "name": name }));
            folders.push(json!({ "name": folder.get("name").and_then(Value::as_str).unwrap_or(&name), "relPath": rel, "definition": folder }));
            collect_export(root, &path, base, requests, folders)?;
        } else if name.ends_with(".pebble.json") && name != "_folder.pebble.json" {
            let request = read_request(root, &path)?;
            let rel = path
                .strip_prefix(base)
                .unwrap_or(&path)
                .to_string_lossy()
                .replace('\\', "/");
            requests.push(json!({ "name": request.get("name").and_then(Value::as_str).unwrap_or(&name), "relPath": rel, "request": request }));
        }
    }
    Ok(())
}

fn postman_collection(name: &str, requests: &[Value], folders: &[Value]) -> Value {
    #[derive(Default)]
    struct Node {
        name: String,
        description: String,
        definition: Value,
        folders: BTreeMap<String, Node>,
        requests: Vec<Value>,
    }
    fn node_at<'a>(root: &'a mut Node, path: &str) -> &'a mut Node {
        let mut node = root;
        for part in path
            .split('/')
            .filter(|part| !part.is_empty() && *part != ".")
        {
            node = node.folders.entry(part.to_owned()).or_insert_with(|| Node {
                name: part.to_owned(),
                ..Node::default()
            });
        }
        node
    }
    fn convert_request(item: &Value) -> Value {
        let request = &item["request"];
        let url = request.get("url").and_then(Value::as_str).unwrap_or("");
        let headers = request
            .get("headers")
            .and_then(Value::as_array)
            .map(|items| {
                items.iter().map(|header| json!({
            "key": header.get("key").and_then(Value::as_str).unwrap_or(""),
            "value": header.get("value").and_then(Value::as_str).unwrap_or(""),
            "disabled": !header.get("enabled").and_then(Value::as_bool).unwrap_or(true)
        })).collect::<Vec<_>>()
            })
            .unwrap_or_default();
        let query = request
            .get("params")
            .and_then(Value::as_array)
            .map(|items| {
                items.iter().map(|param| json!({
            "key": param.get("key").and_then(Value::as_str).unwrap_or(""),
            "value": param.get("value").and_then(Value::as_str).unwrap_or(""),
            "disabled": !param.get("enabled").and_then(Value::as_bool).unwrap_or(true)
        })).collect::<Vec<_>>()
            })
            .unwrap_or_default();
        let mut postman_url = json!({ "raw": url });
        if !query.is_empty() {
            postman_url["query"] = json!(query);
        }
        let body = request.get("body").and_then(Value::as_object).and_then(|body| match body.get("type").and_then(Value::as_str).unwrap_or("") {
            "json" | "raw" => Some(json!({ "mode": "raw", "raw": body.get("raw").and_then(Value::as_str).unwrap_or("") })),
            "urlEncoded" => Some(json!({ "mode": "urlencoded", "urlencoded": body.get("urlEncoded").cloned().unwrap_or_else(|| json!([])) })),
            "formData" => Some(json!({ "mode": "formdata", "formdata": body.get("formData").cloned().unwrap_or_else(|| json!([])) })),
            "graphql" => Some(json!({ "mode": "graphql", "graphql": {
                "query": body.get("graphql").and_then(|graphql| graphql.get("query")).and_then(Value::as_str).or_else(|| body.get("raw").and_then(Value::as_str)).unwrap_or(""),
                "variables": body.get("graphql").and_then(|graphql| graphql.get("variables")).and_then(Value::as_str).unwrap_or("")
            } })),
            _ => None,
        });
        let mut request_value = json!({
            "method": request.get("method").and_then(Value::as_str).unwrap_or("GET"),
            "url": postman_url,
            "header": headers
        });
        let auth_type = request
            .pointer("/auth/type")
            .and_then(Value::as_str)
            .unwrap_or("none");
        if auth_type != "none" && !auth_type.is_empty() {
            request_value["auth"] = postman_export_auth(request.get("auth"));
        }
        if let Some(body) = body {
            request_value["body"] = body;
        }
        let mut exported = json!({ "name": item.get("name").and_then(Value::as_str).unwrap_or("Request"), "request": request_value });
        let events = postman_export_events(request.get("scripts"));
        if !events.is_empty() {
            exported["event"] = json!(events);
        }
        exported
    }
    fn render(node: &Node) -> Vec<Value> {
        let mut items = node
            .folders
            .values()
            .map(|child| {
                let mut folder = json!({ "name": child.name, "item": render(child) });
                if !child.description.is_empty() {
                    folder["description"] = json!(child.description);
                }
                let auth_type = child
                    .definition
                    .pointer("/auth/type")
                    .and_then(Value::as_str)
                    .unwrap_or("none");
                if auth_type != "none" && !auth_type.is_empty() {
                    folder["auth"] = postman_export_auth(Some(&child.definition["auth"]));
                }
                let events = postman_export_events(child.definition.get("scripts"));
                if !events.is_empty() {
                    folder["event"] = json!(events);
                }
                folder
            })
            .collect::<Vec<_>>();
        items.extend(node.requests.iter().map(convert_request));
        items
    }

    let mut root = Node::default();
    for folder in folders {
        let relative = folder.get("relPath").and_then(Value::as_str).unwrap_or("");
        let node = node_at(&mut root, relative);
        node.name = folder
            .get("name")
            .and_then(Value::as_str)
            .unwrap_or(name)
            .to_owned();
        node.description = folder
            .pointer("/definition/description")
            .and_then(Value::as_str)
            .unwrap_or("")
            .to_owned();
        node.definition = folder
            .get("definition")
            .cloned()
            .unwrap_or_else(|| json!({}));
    }
    for request in requests {
        let relative = request.get("relPath").and_then(Value::as_str).unwrap_or("");
        let parent = Path::new(relative)
            .parent()
            .and_then(Path::to_str)
            .unwrap_or("")
            .replace('\\', "/");
        node_at(&mut root, &parent).requests.push(request.clone());
    }
    let variables = folders
        .iter()
        .find(|folder| {
            folder
                .get("relPath")
                .and_then(Value::as_str)
                .is_none_or(|path| path.is_empty() || path == ".")
        })
        .and_then(|folder| folder.pointer("/definition/variables"))
        .cloned()
        .unwrap_or_else(|| json!([]));
    let mut collection = json!({ "info": { "name": name, "schema": "https://schema.getpostman.com/json/collection/v2.1.0/collection.json" }, "item": render(&root) });
    let root_folder = folders.iter().find(|folder| {
        folder
            .get("relPath")
            .and_then(Value::as_str)
            .is_none_or(|path| path.is_empty() || path == ".")
    });
    if let Some(definition) = root_folder.and_then(|folder| folder.get("definition")) {
        let auth_type = definition
            .pointer("/auth/type")
            .and_then(Value::as_str)
            .unwrap_or("none");
        if auth_type != "none" && auth_type != "inherit" {
            collection["auth"] = postman_export_auth(Some(&definition["auth"]));
        }
        let events = postman_export_events(definition.get("scripts"));
        if !events.is_empty() {
            collection["event"] = json!(events);
        }
    }
    if let Some(items) = variables.as_array().filter(|items| !items.is_empty()) {
        collection["variable"] = json!(items.iter().map(|item| json!({ "key": item.get("key").and_then(Value::as_str).unwrap_or(""), "value": item.get("value").and_then(Value::as_str).unwrap_or(""), "disabled": !item.get("enabled").and_then(Value::as_bool).unwrap_or(true) })).collect::<Vec<_>>());
    }
    collection
}

fn postman_export_auth(auth: Option<&Value>) -> Value {
    let Some(auth) = auth else {
        return json!({ "type": "noauth" });
    };
    match auth.get("type").and_then(Value::as_str).unwrap_or("none") {
        "bearer" => {
            json!({ "type": "bearer", "bearer": [{ "key": "token", "value": auth.get("token").and_then(Value::as_str).unwrap_or(""), "type": "string" }] })
        }
        "basic" => {
            json!({ "type": "basic", "basic": [{ "key": "username", "value": auth.get("username").and_then(Value::as_str).unwrap_or(""), "type": "string" }, { "key": "password", "value": auth.get("password").and_then(Value::as_str).unwrap_or(""), "type": "string" }] })
        }
        "apiKey" => {
            json!({ "type": "apikey", "apikey": [{ "key": "key", "value": auth.get("key").and_then(Value::as_str).unwrap_or(""), "type": "string" }, { "key": "value", "value": auth.get("value").and_then(Value::as_str).unwrap_or(""), "type": "string" }, { "key": "in", "value": auth.get("addTo").and_then(Value::as_str).unwrap_or("header"), "type": "string" }] })
        }
        _ => json!({ "type": "noauth" }),
    }
}

fn postman_export_events(scripts: Option<&Value>) -> Vec<Value> {
    let mut events = Vec::new();
    for (key, listen) in [("preRequest", "prerequest"), ("postResponse", "test")] {
        let Some(script) = scripts
            .and_then(|scripts| scripts.get(key))
            .and_then(Value::as_str)
            .filter(|script| !script.trim().is_empty())
        else {
            continue;
        };
        let converted = script
            .replace("pb.environment.get(", "pm.environment.get(")
            .replace("pb.environment.set(", "pm.environment.set(")
            .replace("pb.variables.get(", "pm.variables.get(")
            .replace("pb.response.json()", "pm.response.json()")
            .replace("pb.response.text()", "pm.response.text()")
            .replace("pb.response.status", "pm.response.code")
            .replace("pb.test(", "pm.test(")
            .replace("pb.console.", "console.")
            .replace("pb.expect(", "pm.expect(")
            .replace(").toEqual(", ").to.eql(")
            .replace(").toBe(", ").to.equal(");
        events.push(json!({ "listen": listen, "script": { "type": "text/javascript", "exec": converted.lines().collect::<Vec<_>>() } }));
    }
    events
}

fn openapi_collection(name: &str, requests: &[Value]) -> Value {
    let mut paths = Map::new();
    let mut servers = BTreeMap::<String, ()>::new();
    for item in requests {
        let request = &item["request"];
        let Some(url) = request
            .get("url")
            .and_then(Value::as_str)
            .filter(|url| !url.trim().is_empty())
        else {
            continue;
        };
        let (server, path) = openapi_server_and_path(url);
        if !server.is_empty() {
            servers.insert(server, ());
        }
        let path = openapi_path_parameters(&path);
        let method = request
            .get("method")
            .and_then(Value::as_str)
            .filter(|method| !method.is_empty())
            .unwrap_or("GET")
            .to_ascii_lowercase();
        let operation = openapi_operation(item, request, &path);
        let entry = paths
            .entry(path)
            .or_insert_with(|| Value::Object(Map::new()));
        if let Some(operations) = entry.as_object_mut() {
            operations.insert(method, operation);
        }
    }
    let servers = if servers.is_empty() {
        vec![json!({ "url": "https://api.example.com" })]
    } else {
        servers
            .keys()
            .map(|server| json!({ "url": server }))
            .collect()
    };
    json!({
        "openapi": "3.0.3",
        "info": { "title": if name.trim().is_empty() { "PebblePost API" } else { name }, "version": "1.0.0" },
        "servers": servers,
        "paths": paths
    })
}

fn openapi_server_and_path(raw_url: &str) -> (String, String) {
    let raw_url = raw_url.trim();
    if raw_url.starts_with("{{")
        && let Some(end) = raw_url.find("}}")
    {
        let server_end = end + 2;
        let path = raw_url[server_end..].split('?').next().unwrap_or("");
        return (raw_url[..server_end].to_owned(), path.to_owned());
    }
    if let Ok(url) = reqwest::Url::parse(raw_url)
        && let Some(host) = url.host_str()
    {
        let host = if host.contains(':') {
            format!("[{host}]")
        } else {
            host.to_owned()
        };
        let authority = url
            .port()
            .map_or(host.clone(), |port| format!("{host}:{port}"));
        let path = if url.path().is_empty() {
            "/"
        } else {
            url.path()
        };
        return (format!("{}://{authority}", url.scheme()), path.to_owned());
    }
    let path = raw_url.split('?').next().unwrap_or("");
    (String::new(), path.to_owned())
}

fn openapi_path_parameters(path: &str) -> String {
    let chars = path.chars().collect::<Vec<_>>();
    let mut converted = String::with_capacity(path.len());
    let mut index = 0;
    while index < chars.len() {
        if chars[index] == ':' {
            let start = index + 1;
            let mut end = start;
            while end < chars.len() && (chars[end].is_ascii_alphanumeric() || chars[end] == '_') {
                end += 1;
            }
            if end > start {
                converted.push('{');
                converted.extend(chars[start..end].iter().copied());
                converted.push('}');
                index = end;
                continue;
            }
        }
        converted.push(chars[index]);
        index += 1;
    }
    if converted.starts_with('/') {
        converted
    } else {
        format!("/{converted}")
    }
}

fn openapi_operation(item: &Value, request: &Value, path: &str) -> Value {
    let summary = request
        .get("name")
        .and_then(Value::as_str)
        .or_else(|| item.get("name").and_then(Value::as_str))
        .unwrap_or("Request");
    let description = request
        .get("description")
        .and_then(Value::as_str)
        .unwrap_or("");
    let mut operation = json!({
        "summary": summary,
        "responses": { "200": { "description": "Successful response" } }
    });
    if !description.is_empty() {
        operation["description"] = json!(description);
    }

    if let Some(examples) = request
        .get("examples")
        .and_then(Value::as_array)
        .filter(|items| !items.is_empty())
    {
        let mut responses = Map::new();
        for example in examples {
            let status_code = example
                .get("statusCode")
                .and_then(Value::as_i64)
                .unwrap_or(0);
            let status = if status_code == 0 {
                "default".to_owned()
            } else {
                status_code.to_string()
            };
            let description = example
                .get("statusText")
                .and_then(Value::as_str)
                .filter(|value| !value.is_empty())
                .or_else(|| {
                    example
                        .get("name")
                        .and_then(Value::as_str)
                        .filter(|value| !value.is_empty())
                })
                .unwrap_or("Example response");
            let content_type = example
                .get("contentType")
                .and_then(Value::as_str)
                .filter(|value| !value.is_empty())
                .unwrap_or("application/json");
            let body = example.get("body").and_then(Value::as_str).unwrap_or("");
            let mut response = json!({ "description": description });
            if !body.is_empty() {
                let example_value =
                    serde_json::from_str::<Value>(body).unwrap_or_else(|_| json!(body));
                response["content"] = json!({ (content_type): { "example": example_value } });
            }
            responses.insert(status, response);
        }
        if !responses.is_empty() {
            operation["responses"] = Value::Object(responses);
        }
    }

    let mut parameters = Vec::new();
    let mut seen_path_parameters = Vec::<String>::new();
    let mut inside_parameter = false;
    let mut parameter = String::new();
    for character in path.chars() {
        match character {
            '{' => {
                inside_parameter = true;
                parameter.clear();
            }
            '}' if inside_parameter => {
                if !parameter.is_empty() && !seen_path_parameters.contains(&parameter) {
                    seen_path_parameters.push(parameter.clone());
                    parameters.push(json!({ "name": parameter, "in": "path", "required": true, "schema": { "type": "string" } }));
                }
                inside_parameter = false;
            }
            _ if inside_parameter => parameter.push(character),
            _ => {}
        }
    }
    if let Some(query) = request.get("params").and_then(Value::as_array) {
        for param in query.iter().filter(|param| {
            param
                .get("enabled")
                .and_then(Value::as_bool)
                .unwrap_or(false)
        }) {
            let key = param.get("key").and_then(Value::as_str).unwrap_or("");
            if key.is_empty() {
                continue;
            }
            parameters.push(json!({ "name": key, "in": "query", "required": false, "schema": { "type": "string", "example": param.get("value").and_then(Value::as_str).unwrap_or("") } }));
        }
    }
    if let Some(headers) = request.get("headers").and_then(Value::as_array) {
        for header in headers.iter().filter(|header| {
            header
                .get("enabled")
                .and_then(Value::as_bool)
                .unwrap_or(false)
        }) {
            let key = header.get("key").and_then(Value::as_str).unwrap_or("");
            if key.is_empty()
                || matches!(
                    key.to_ascii_lowercase().as_str(),
                    "content-type" | "authorization" | "accept" | "user-agent"
                )
            {
                continue;
            }
            parameters.push(json!({ "name": key, "in": "header", "required": false, "schema": { "type": "string", "example": header.get("value").and_then(Value::as_str).unwrap_or("") } }));
        }
    }
    if !parameters.is_empty() {
        operation["parameters"] = json!(parameters);
    }

    if let Some(body) = request.get("body").and_then(openapi_request_body) {
        operation["requestBody"] = body;
    }

    let rel_path = item
        .get("relPath")
        .and_then(Value::as_str)
        .unwrap_or("")
        .replace('\\', "/");
    let parts = rel_path.split('/').collect::<Vec<_>>();
    if parts.len() > 1 && !parts[0].is_empty() {
        operation["tags"] = json!([parts[0]]);
    }
    operation
}

fn openapi_request_body(body: &Value) -> Option<Value> {
    match body.get("type").and_then(Value::as_str).unwrap_or("") {
        "json" => {
            let raw = body.get("raw").and_then(Value::as_str).unwrap_or("");
            let schema = if raw.trim().is_empty() {
                json!({ "type": "object" })
            } else {
                serde_json::from_str::<Value>(raw)
                    .map(|value| infer_openapi_schema(&value))
                    .unwrap_or_else(|_| json!({ "type": "object" }))
            };
            Some(
                json!({ "required": true, "content": { "application/json": { "schema": schema } } }),
            )
        }
        "urlEncoded" => {
            let mut properties = Map::new();
            for field in body
                .get("urlEncoded")
                .and_then(Value::as_array)
                .into_iter()
                .flatten()
            {
                let key = field.get("key").and_then(Value::as_str).unwrap_or("");
                if !key.is_empty() {
                    properties.insert(key.to_owned(), json!({ "type": "string", "example": field.get("value").and_then(Value::as_str).unwrap_or("") }));
                }
            }
            Some(
                json!({ "content": { "application/x-www-form-urlencoded": { "schema": { "type": "object", "properties": properties } } } }),
            )
        }
        "formData" => {
            let mut properties = Map::new();
            for field in body
                .get("formData")
                .and_then(Value::as_array)
                .into_iter()
                .flatten()
            {
                let key = field.get("key").and_then(Value::as_str).unwrap_or("");
                if key.is_empty() {
                    continue;
                }
                let schema = if field.get("type").and_then(Value::as_str) == Some("file") {
                    json!({ "type": "string", "format": "binary" })
                } else {
                    json!({ "type": "string", "example": field.get("value").and_then(Value::as_str).unwrap_or("") })
                };
                properties.insert(key.to_owned(), schema);
            }
            Some(
                json!({ "content": { "multipart/form-data": { "schema": { "type": "object", "properties": properties } } } }),
            )
        }
        "raw" => {
            let raw = body.get("raw").and_then(Value::as_str).unwrap_or("");
            if raw.trim().is_empty() {
                return None;
            }
            Some(
                json!({ "content": { "text/plain": { "schema": { "type": "string", "example": raw } } } }),
            )
        }
        _ => None,
    }
}

fn infer_openapi_schema(value: &Value) -> Value {
    match value {
        Value::Null => json!({ "type": "string", "nullable": true }),
        Value::Bool(_) => json!({ "type": "boolean", "example": value }),
        Value::Number(number) => {
            if number.as_i64().is_some() || number.as_u64().is_some() {
                json!({ "type": "integer", "example": value })
            } else if let Some(number) = number.as_f64() {
                if number.fract() == 0.0 && number >= i64::MIN as f64 && number <= i64::MAX as f64 {
                    json!({ "type": "integer", "example": number as i64 })
                } else {
                    json!({ "type": "number", "example": value })
                }
            } else {
                json!({ "type": "number", "example": value })
            }
        }
        Value::String(_) => json!({ "type": "string", "example": value }),
        Value::Array(values) => {
            json!({ "type": "array", "items": values.first().map(infer_openapi_schema).unwrap_or_else(|| json!({ "type": "string" })) })
        }
        Value::Object(values) => {
            let properties = values
                .iter()
                .map(|(key, value)| (key.clone(), infer_openapi_schema(value)))
                .collect::<Map<_, _>>();
            json!({ "type": "object", "properties": properties })
        }
    }
}

fn ensure_directories(root: &Path, relative: &Path) -> Result<(), PebbleError> {
    let mut current = PathBuf::new();
    for component in relative.components() {
        match component {
            std::path::Component::Normal(part) => current.push(part),
            std::path::Component::CurDir => continue,
            _ => {
                return Err(PebbleError::InvalidPath(
                    "import destination escapes workspace".into(),
                ));
            }
        }
        let candidate = root.join(&current);
        if !candidate.exists() {
            create_folder(root, &current)?;
        }
        let safe = safe_workspace_path(root, &candidate)?;
        if !safe.is_dir() {
            return Err(PebbleError::InvalidPath(
                "import destination is not a folder".into(),
            ));
        }
    }
    Ok(())
}

fn slug(value: &str) -> String {
    let cleaned = value
        .chars()
        .map(|ch| {
            if ch.is_ascii_alphanumeric() || ch == '-' || ch == '_' {
                ch.to_ascii_lowercase()
            } else {
                '-'
            }
        })
        .collect::<String>();
    let cleaned = cleaned.trim_matches('-');
    if cleaned.is_empty() {
        "request".into()
    } else {
        cleaned.to_owned()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn curl_import_preserves_request_options_auth_query_and_json_body() {
        let result = parse_import("curl", r#"curl.exe -X put --location -k --max-time 15 -H 'Authorization: Bearer abc123' -H 'X-Trace: yes' -d '{"id":7}' 'https://api.test/users?id=4&active=true'"#).unwrap();
        let request = &result.requests[0].1;
        assert_eq!(request["method"], "PUT");
        assert_eq!(request["url"], "https://api.test/users");
        assert_eq!(request["auth"]["type"], "bearer");
        assert_eq!(request["auth"]["token"], "abc123");
        assert_eq!(request["headers"][0]["key"], "X-Trace");
        assert_eq!(request["params"][0]["key"], "id");
        assert_eq!(request["params"][1]["value"], "true");
        assert_eq!(request["body"]["type"], "json");
        assert_eq!(request["settings"]["verifySSL"], false);
        assert_eq!(request["settings"]["timeoutMs"], 15_000);
    }

    #[test]
    fn curl_import_handles_basic_auth_and_multipart_files() {
        let result = parse_import(
            "curl",
            "curl -u user:password -F file=@report.pdf https://api.test/upload",
        )
        .unwrap();
        let request = &result.requests[0].1;
        assert_eq!(request["auth"]["type"], "basic");
        assert_eq!(request["auth"]["username"], "user");
        assert_eq!(request["body"]["type"], "formData");
        assert_eq!(request["body"]["formData"][0]["type"], "file");

        let header_auth = parse_import(
            "curl",
            "curl --header='Authorization: Basic dTpw' --url=https://api.test/me",
        )
        .unwrap();
        assert_eq!(header_auth.requests[0].1["auth"]["username"], "u");
        assert_eq!(header_auth.requests[0].1["auth"]["password"], "p");
    }

    #[test]
    fn har_import_maps_structured_query_auth_and_json_body() {
        let content = r#"{
            "log": { "creator": { "name": "Browser" }, "entries": [{ "request": {
                "method": "post", "url": "https://api.test/orders?discard=1",
                "queryString": [{ "name": "expand", "value": "items" }],
                "headers": [
                    { "name": ":method", "value": "POST" },
                    { "name": "Authorization", "value": "Bearer secret" },
                    { "name": "Content-Type", "value": "application/json" }
                ],
                "postData": { "mimeType": "application/json", "text": "{\"id\":3}" }
            }}] }
        }"#;
        let result = parse_import("har", content).unwrap();
        assert_eq!(result.collection_name, "HAR (Browser)");
        let request = &result.requests[0].1;
        assert_eq!(request["method"], "POST");
        assert_eq!(request["url"], "https://api.test/orders");
        assert_eq!(request["params"][0]["key"], "expand");
        assert_eq!(request["auth"]["token"], "secret");
        assert_eq!(request["headers"].as_array().unwrap().len(), 1);
        assert_eq!(request["body"]["type"], "json");
    }

    #[test]
    fn bruno_insomnia_and_format_detection_cover_supported_inputs() {
        let bruno = parse_import("bruno", "meta {\n name: List Users\n}\nget {\n url: https://api.test/users\n}\nheaders {\n Accept: application/json\n}").unwrap();
        assert_eq!(bruno.requests[0].1["method"], "GET");
        assert_eq!(bruno.requests[0].1["headers"][0]["key"], "Accept");
        let bruno_form = parse_import("bruno", "meta {\n name: Search\n}\npost {\n url: https://api.test/search\n}\nbody:form-urlencoded {\n q: red blue\n ~page: 2\n}").unwrap();
        assert_eq!(bruno_form.requests[0].1["body"]["type"], "urlEncoded");
        assert_eq!(
            bruno_form.requests[0].1["body"]["urlEncoded"][1]["enabled"],
            false
        );

        let insomnia = parse_import("insomnia", r#"{"_type":"export","__export_format":4,"resources":[{"_id":"wrk","_type":"workspace","name":"Billing"},{"_id":"fld","_type":"request_group","parentId":"wrk","name":"Invoices"},{"_id":"req","_type":"request","parentId":"fld","name":"Create","method":"POST","url":"https://api.test/invoices","body":{"mimeType":"application/json","text":"{\"amount\":1}"}}]}"#).unwrap();
        assert_eq!(insomnia.collection_name, "Billing");
        assert_eq!(insomnia.requests[0].0, "invoices/create.pebble.json");
        assert_eq!(insomnia.requests[0].1["body"]["type"], "json");

        assert_eq!(
            detect_import_format("curl.exe https://api.test"),
            Some("curl".into())
        );
        assert_eq!(detect_import_format("curl"), Some("curl".into()));
        assert_eq!(
            detect_import_format("meta {\n name: Test\n}"),
            Some("bruno".into())
        );
        assert_eq!(
            detect_import_format(r#"{"log":{"entries":[]}}"#),
            Some("har".into())
        );
    }

    #[test]
    fn exporters_emit_postman_collection_and_openapi_operations() {
        let request = json!({
            "name": "Get user", "description": "Fetch a user", "method": "GET",
            "url": "https://api.test/v1/users/:userId", "headers": [],
            "params": [{ "key": "expand", "value": "profile", "enabled": true }],
            "auth": { "type": "none" }, "body": { "type": "none" },
            "scripts": { "preRequest": "", "postResponse": "" },
            "settings": { "followRedirects": true, "verifySSL": true, "timeoutMs": 30000 }
        });
        let requests = vec![
            json!({ "name": "Get user", "relPath": "get-user.pebble.json", "request": request }),
        ];
        let postman = postman_collection("Users", &requests, &[]);
        assert_eq!(
            postman.pointer("/info/name").and_then(Value::as_str),
            Some("Users")
        );
        assert_eq!(
            postman
                .pointer("/item/0/request/method")
                .and_then(Value::as_str),
            Some("GET")
        );

        let openapi = openapi_collection("Users", &requests);
        assert_eq!(
            openapi.get("openapi").and_then(Value::as_str),
            Some("3.0.3")
        );
        assert_eq!(
            openapi.pointer("/servers/0/url").and_then(Value::as_str),
            Some("https://api.test")
        );
        assert_eq!(
            openapi
                .pointer("/paths/~1v1~1users~1{userId}/get/parameters/0/in")
                .and_then(Value::as_str),
            Some("path")
        );
    }

    #[test]
    fn openapi_import_retains_path_parameters_and_response_contracts_for_sync() {
        let spec = json!({
            "openapi": "3.0.3", "info": { "title": "Users", "version": "1" },
            "servers": [{ "url": "https://api.test/v1" }],
            "paths": { "/users/{id}": {
                "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "integer" } }],
                "get": {
                    "summary": "Get user", "parameters": [{ "name": "expand", "in": "query", "required": false, "schema": { "type": "string" } }],
                    "responses": { "200": { "description": "A user", "content": { "application/json": { "schema": { "type": "object" } } } } }
                }
            } }
        });
        let result = parse_import("openapi", &spec.to_string()).unwrap();
        let request = &result.requests[0].1;
        assert_eq!(request["openApiParameters"].as_array().unwrap().len(), 2);
        assert_eq!(request["params"][0]["key"], "expand");
        assert_eq!(request["openApiResponses"]["200"]["description"], "A user");
        assert_eq!(request["url"], "https://api.test/v1/users/{id}");
    }
}
