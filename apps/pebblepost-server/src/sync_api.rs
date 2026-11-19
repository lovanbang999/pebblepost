use std::collections::{HashMap, HashSet};
use std::fs;
use std::path::{Path, PathBuf};

use axum::Json;
use axum::extract::Query;
use axum::http::StatusCode;
use axum::response::{IntoResponse, Response};
use pebblepost_core::{
    PebbleError, delete_path, read_folder, read_request, safe_workspace_path, save_folder,
    save_request,
};
use serde::Deserialize;
use serde_json::{Value, json};
use sha2::{Digest, Sha256};

#[derive(Deserialize)]
pub struct SyncQuery {
    folder: String,
    spec: Option<String>,
}

pub async fn diff(Query(query): Query<SyncQuery>) -> Response {
    match build_diff(&query.folder, query.spec.as_deref()).await {
        Ok(report) => Json(report).into_response(),
        Err(error) => error_response(StatusCode::BAD_REQUEST, error),
    }
}

pub async fn link(Json(payload): Json<Value>) -> Response {
    let folder = payload
        .get("folderPath")
        .and_then(Value::as_str)
        .unwrap_or("")
        .trim();
    let spec_location = payload
        .get("specLocation")
        .and_then(Value::as_str)
        .unwrap_or("")
        .trim();
    if folder.is_empty() || spec_location.is_empty() {
        return error_response(
            StatusCode::BAD_REQUEST,
            "folderPath and specLocation are required".into(),
        );
    }
    let report = match build_diff(folder, Some(spec_location)).await {
        Ok(report) => report,
        Err(error) => return error_response(StatusCode::BAD_REQUEST, error),
    };
    let (root, target) = match resolve_folder(folder) {
        Ok(paths) => paths,
        Err(error) => return error_response(StatusCode::BAD_REQUEST, error),
    };
    let mut definition = read_folder(&root, &target).unwrap_or_else(|_| json!({}));
    definition["openApiSync"] = json!({ "specLocation": spec_location, "specHash": report["specHash"], "lastSyncedAt": chrono::Utc::now().to_rfc3339() });
    if let Err(error) = save_folder(&root, &target, definition.clone()) {
        return error_response(StatusCode::BAD_REQUEST, error.to_string());
    }
    Json(json!({ "success": true, "specLocation": spec_location, "specHash": report["specHash"], "lastSyncedAt": definition.pointer("/openApiSync/lastSyncedAt"), "folder": definition })).into_response()
}

pub async fn apply(Json(payload): Json<Value>) -> Response {
    let folder = payload
        .get("folderPath")
        .and_then(Value::as_str)
        .unwrap_or("")
        .trim();
    if folder.is_empty() {
        return error_response(StatusCode::BAD_REQUEST, "folderPath is required".into());
    }
    let spec_location = payload.get("specLocation").and_then(Value::as_str);
    let report = match build_diff(folder, spec_location).await {
        Ok(report) => report,
        Err(error) => return error_response(StatusCode::BAD_REQUEST, error),
    };
    let (root, target) = match resolve_folder(folder) {
        Ok(paths) => paths,
        Err(error) => return error_response(StatusCode::BAD_REQUEST, error),
    };
    let accepted = payload
        .get("acceptedEndpoints")
        .and_then(Value::as_array)
        .map(|values| {
            values
                .iter()
                .filter_map(Value::as_str)
                .map(str::to_owned)
                .collect::<HashSet<_>>()
        })
        .unwrap_or_default();
    let force = payload
        .get("force")
        .and_then(Value::as_bool)
        .unwrap_or(false);
    let endpoints = report
        .get("endpoints")
        .and_then(Value::as_array)
        .cloned()
        .unwrap_or_default();
    let mut applied = 0;
    let mut skipped = 0;
    let mut created = Vec::new();
    let mut updated = Vec::new();
    let mut deleted = Vec::new();
    for endpoint in endpoints {
        let id = endpoint.get("id").and_then(Value::as_str).unwrap_or("");
        if endpoint.get("diffType").and_then(Value::as_str) == Some("unchanged") {
            continue;
        }
        if !accepted.is_empty() && !accepted.contains(id) {
            skipped += 1;
            continue;
        }
        let diff_type = endpoint
            .get("diffType")
            .and_then(Value::as_str)
            .unwrap_or("");
        let conflict = endpoint
            .pointer("/conflict")
            .is_some_and(|value| !value.is_null());
        if diff_type == "removed" {
            if conflict && !force {
                skipped += 1;
                continue;
            }
            let Some(file_path) = endpoint.get("existingFilePath").and_then(Value::as_str) else {
                skipped += 1;
                continue;
            };
            match delete_path(&root, Path::new(file_path), true) {
                Ok(()) => {
                    deleted.push(file_path.to_owned());
                    applied += 1;
                }
                Err(_) => skipped += 1,
            }
            continue;
        }
        let Some(mut proposed) = endpoint.get("specRequest").cloned() else {
            skipped += 1;
            continue;
        };
        let existing_path = endpoint.get("existingFilePath").and_then(Value::as_str);
        let destination = if let Some(existing_path) = existing_path {
            if let Ok(existing) = read_request(&root, Path::new(existing_path)) {
                proposed = merge_request(&existing, &proposed);
            }
            PathBuf::from(existing_path)
        } else {
            endpoint
                .get("proposedFilePath")
                .and_then(Value::as_str)
                .map(PathBuf::from)
                .unwrap_or_else(|| {
                    let name = proposed
                        .get("name")
                        .and_then(Value::as_str)
                        .unwrap_or("endpoint");
                    target.join(format!("{}.pebble.json", slug(name)))
                })
        };
        match save_request(&root, &destination, proposed) {
            Ok(path) => {
                if existing_path.is_some() {
                    updated.push(path.to_string_lossy().into_owned());
                } else {
                    created.push(path.to_string_lossy().into_owned());
                }
                applied += 1;
            }
            Err(_) => skipped += 1,
        }
    }
    let synced_at = chrono::Utc::now().to_rfc3339();
    let mut definition = read_folder(&root, &target).unwrap_or_else(|_| json!({}));
    definition["openApiSync"] = json!({
        "specLocation": report.get("specLocation").and_then(Value::as_str).unwrap_or(""),
        "specHash": report.get("specHash").and_then(Value::as_str).unwrap_or(""),
        "lastSyncedAt": synced_at
    });
    if let Err(error) = save_folder(&root, &target, definition) {
        return error_response(StatusCode::BAD_REQUEST, error.to_string());
    }
    Json(json!({ "appliedCount": applied, "skippedCount": skipped, "createdFiles": created, "updatedFiles": updated, "deletedFiles": deleted, "newSpecHash": report["specHash"], "syncedAt": synced_at })).into_response()
}

async fn build_diff(folder: &str, requested_spec: Option<&str>) -> Result<Value, String> {
    let (root, target) = resolve_folder(folder)?;
    let folder_definition = read_folder(&root, &target).unwrap_or_else(|_| json!({}));
    let spec_location = requested_spec
        .filter(|value| !value.trim().is_empty())
        .or_else(|| {
            folder_definition
                .pointer("/openApiSync/specLocation")
                .and_then(Value::as_str)
        })
        .ok_or_else(|| "No OpenAPI specification linked to this folder".to_owned())?;
    let spec_data = load_spec(spec_location, &target).await?;
    let parsed_spec = super::import_export::parse_openapi_document(&spec_data)
        .map_err(|error| format!("Failed to parse OpenAPI specification: {error}"))?;
    let version = parsed_spec
        .get("openapi")
        .and_then(Value::as_str)
        .unwrap_or("");
    if !version.starts_with('3') {
        return Err(format!("only OpenAPI 3.x is supported (got {version:?})"));
    }
    let server_base_path = parsed_spec
        .pointer("/servers/0/url")
        .and_then(Value::as_str)
        .map(server_path)
        .unwrap_or_default();
    let parsed = super::import_export::parse_import("openapi", &spec_data)
        .map_err(|error| format!("Failed to parse OpenAPI specification: {error}"))?;
    let spec_hash = format!("{:x}", Sha256::digest(spec_data.as_bytes()));
    let mut existing = Vec::new();
    collect_local(&root, &target, &mut existing).map_err(|error| error.to_string())?;
    let mut existing_by_id = HashMap::new();
    let mut existing_by_op_id = HashMap::new();
    for (path, request) in existing {
        let id = endpoint_id_with_base(&request, &server_base_path);
        for operation_id in [
            request.get("id").and_then(Value::as_str),
            request.get("name").and_then(Value::as_str),
        ]
        .into_iter()
        .flatten()
        {
            if !operation_id.is_empty() {
                existing_by_op_id.insert(operation_id.to_ascii_lowercase(), id.clone());
            }
        }
        existing_by_id.insert(id, (path, request));
    }
    let mut matched = HashSet::new();
    let mut endpoints = Vec::new();
    for (_, spec_request) in &parsed.requests {
        let id = endpoint_id_with_base(spec_request, &server_base_path);
        let method = spec_request
            .get("method")
            .and_then(Value::as_str)
            .unwrap_or("GET");
        let path = strip_server_path(
            &request_path(
                spec_request
                    .get("url")
                    .and_then(Value::as_str)
                    .unwrap_or("/"),
            ),
            &server_base_path,
        );
        let operation_id = spec_request.get("id").and_then(Value::as_str).unwrap_or("");
        let matched_id = if existing_by_id.contains_key(&id) {
            Some(id.clone())
        } else {
            existing_by_op_id
                .get(&operation_id.to_ascii_lowercase())
                .cloned()
        };
        if let Some(matched_id) = matched_id.filter(|matched_id| !matched.contains(matched_id)) {
            let Some((existing_path, existing_request)) = existing_by_id.get(&matched_id) else {
                continue;
            };
            matched.insert(matched_id);
            let fields = changed_fields(existing_request, spec_request, &server_base_path);
            let diff_type = if fields.is_empty() {
                "unchanged"
            } else {
                "changed"
            };
            endpoints.push(json!({
                "id": id, "operationId": operation_id, "summary": spec_request.get("name"), "method": method,
                "path": path, "diffType": diff_type, "existingFilePath": existing_path,
                "proposedFilePath": existing_path, "fieldDiffs": fields,
                "specRequest": spec_request, "existingRequest": existing_request
            }));
        } else {
            endpoints.push(json!({
                "id": id, "operationId": operation_id, "summary": spec_request.get("name"), "method": method, "path": path,
                "diffType": "added", "proposedFilePath": target.join(format!("{}.pebble.json", slug(spec_request.get("name").and_then(Value::as_str).unwrap_or("endpoint")))).to_string_lossy(),
                "fieldDiffs": [{ "field": "endpoint", "oldValue": null, "newValue": id, "isConflict": false, "description": format!("New endpoint {id} found in specification") }],
                "specRequest": spec_request
            }));
        }
    }
    for (id, (path, request)) in &existing_by_id {
        if matched.contains(id) {
            continue;
        }
        let conflict = conflict_info(request);
        endpoints.push(json!({
            "id": id, "operationId": request.get("id"), "summary": request.get("name"), "method": request.get("method"),
            "path": strip_server_path(&request_path(request.get("url").and_then(Value::as_str).unwrap_or("/")), &server_base_path),
            "diffType": "removed", "existingFilePath": path,
            "fieldDiffs": [{ "field": "endpoint", "oldValue": id, "newValue": null, "isConflict": !conflict.is_null(), "description": format!("Endpoint {id} no longer exists in specification") }],
            "conflict": conflict, "existingRequest": request
        }));
    }
    for endpoint in &mut endpoints {
        omit_empty_diff_fields(endpoint);
    }
    let added = endpoints
        .iter()
        .filter(|endpoint| endpoint["diffType"] == "added")
        .count();
    let removed = endpoints
        .iter()
        .filter(|endpoint| endpoint["diffType"] == "removed")
        .count();
    let changed = endpoints
        .iter()
        .filter(|endpoint| endpoint["diffType"] == "changed")
        .count();
    let conflicts = endpoints
        .iter()
        .filter(|endpoint| {
            endpoint
                .get("conflict")
                .is_some_and(|value| !value.is_null())
        })
        .count();
    Ok(
        json!({ "folderPath": target, "specLocation": spec_location, "specHash": spec_hash, "totalEndpoints": endpoints.len(), "addedCount": added, "removedCount": removed, "changedCount": changed, "conflictCount": conflicts, "hasDrift": added + removed + changed > 0, "endpoints": endpoints }),
    )
}

fn omit_empty_diff_fields(endpoint: &mut Value) {
    let Some(fields) = endpoint.as_object_mut() else {
        return;
    };
    for field in ["operationId", "summary"] {
        if fields
            .get(field)
            .is_none_or(|value| value.as_str().is_none_or(str::is_empty))
        {
            fields.remove(field);
        }
    }
    if fields.get("conflict").is_some_and(Value::is_null) {
        fields.remove("conflict");
    }
    if fields
        .get("fieldDiffs")
        .and_then(Value::as_array)
        .is_some_and(Vec::is_empty)
    {
        fields.remove("fieldDiffs");
    }
}

async fn load_spec(location: &str, folder: &Path) -> Result<String, String> {
    if location.starts_with("https://") || location.starts_with("http://") {
        let url =
            reqwest::Url::parse(location).map_err(|error| format!("Invalid spec URL: {error}"))?;
        let host = url.host_str().unwrap_or("").to_ascii_lowercase();
        if matches!(
            host.as_str(),
            "169.254.169.254" | "instance-data" | "metadata.google.internal" | "100.100.100.200"
        ) {
            return Err(format!(
                "security: requests to cloud metadata service {host:?} are blocked"
            ));
        }
        let client = reqwest::Client::builder()
            .timeout(std::time::Duration::from_secs(15))
            .build()
            .map_err(|error| error.to_string())?;
        let response = client
            .get(url)
            .header(reqwest::header::USER_AGENT, "PebblePost-OpenAPISync/1.0")
            .header(
                reqwest::header::ACCEPT,
                "application/json, application/yaml, text/yaml, */*",
            )
            .send()
            .await
            .map_err(|error| format!("Could not fetch OpenAPI spec: {error}"))?;
        if !response.status().is_success() {
            return Err(format!("OpenAPI spec returned HTTP {}", response.status()));
        }
        const MAX_SPEC_BYTES: usize = 10 * 1024 * 1024;
        let mut response = response;
        let mut data = Vec::new();
        while let Some(chunk) = response.chunk().await.map_err(|error| error.to_string())? {
            let remaining = MAX_SPEC_BYTES.saturating_sub(data.len());
            data.extend_from_slice(&chunk[..chunk.len().min(remaining)]);
            if data.len() == MAX_SPEC_BYTES {
                break;
            }
        }
        return String::from_utf8(data)
            .map_err(|error| format!("OpenAPI spec response is not UTF-8: {error}"));
    }
    let path = Path::new(location);
    let path = if path.is_absolute() {
        path.to_path_buf()
    } else {
        folder.join(path)
    };
    fs::read_to_string(path).map_err(|error| format!("Could not read OpenAPI spec: {error}"))
}

fn resolve_folder(folder: &str) -> Result<(PathBuf, PathBuf), String> {
    let requested = Path::new(folder);
    let target = fs::canonicalize(requested)
        .map_err(|error| format!("Could not access collection folder: {error}"))?;
    if !target.is_dir() {
        return Err("Collection path is not a folder".into());
    }
    let root = target
        .ancestors()
        .find(|ancestor| ancestor.join(".pebble/workspace.json").is_file())
        .unwrap_or(&target)
        .to_path_buf();
    let target = safe_workspace_path(&root, &target).map_err(|error| error.to_string())?;
    Ok((root, target))
}

fn collect_local(
    root: &Path,
    directory: &Path,
    out: &mut Vec<(PathBuf, Value)>,
) -> Result<(), PebbleError> {
    for entry in fs::read_dir(directory)? {
        let entry = entry?;
        let name = entry.file_name().to_string_lossy().into_owned();
        let path = entry.path();
        if entry.file_type()?.is_file()
            && name.ends_with(".pebble.json")
            && name != "_folder.pebble.json"
            && let Ok(request) = read_request(root, &path)
        {
            out.push((path, request));
        }
    }
    Ok(())
}

fn endpoint_id_with_base(request: &Value, server_base_path: &str) -> String {
    format!(
        "{} {}",
        request
            .get("method")
            .and_then(Value::as_str)
            .unwrap_or("GET")
            .to_ascii_uppercase(),
        strip_server_path(
            &request_path(request.get("url").and_then(Value::as_str).unwrap_or("/")),
            server_base_path
        )
    )
}

fn server_path(server: &str) -> String {
    let path = if let Some((_, rest)) = server.split_once("://") {
        rest.find('/')
            .map(|index| rest[index..].to_owned())
            .unwrap_or_default()
    } else {
        server.to_owned()
    };
    path.trim_end_matches('/').to_owned()
}

fn strip_server_path(path: &str, server_base_path: &str) -> String {
    if !server_base_path.is_empty() && path.starts_with(server_base_path) {
        let remainder = &path[server_base_path.len()..];
        if remainder.is_empty() || remainder.starts_with('/') {
            return request_path(remainder);
        }
    }
    request_path(path)
}

fn request_path(url: &str) -> String {
    let mut path = url.trim().to_owned();
    if let Some((_, rest)) = path.split_once("://") {
        path = rest
            .find('/')
            .map(|index| rest[index..].to_owned())
            .unwrap_or_else(|| "/".into());
    }
    while path.starts_with("{{") {
        let Some(end) = path.find("}}") else {
            break;
        };
        path = path[end + 2..].to_owned();
        if let Some(remainder) = path.strip_prefix('/') {
            path = remainder.to_owned();
        }
        if !path.starts_with('/') {
            path.insert(0, '/');
        }
    }
    path = path.split('?').next().unwrap_or(&path).to_owned();
    let segments = path
        .split('/')
        .map(|segment| {
            if segment.starts_with(':') && segment.len() > 1 {
                format!("{{{}}}", &segment[1..])
            } else {
                segment.to_owned()
            }
        })
        .collect::<Vec<_>>();
    let normalized = segments.join("/").trim_end_matches('/').to_owned();
    if normalized.is_empty() {
        "/".into()
    } else {
        normalized
    }
}

fn changed_fields(existing: &Value, proposed: &Value, server_base_path: &str) -> Vec<Value> {
    let mut diffs = Vec::new();
    let local_method = existing.get("method").and_then(Value::as_str).unwrap_or("");
    let spec_method = proposed.get("method").and_then(Value::as_str).unwrap_or("");
    if !local_method.eq_ignore_ascii_case(spec_method) {
        diffs.push(json!({ "field": "method", "oldValue": local_method, "newValue": spec_method, "isConflict": false, "description": format!("HTTP method changed from {local_method} to {spec_method}") }));
    }

    let local_norm = request_path(existing.get("url").and_then(Value::as_str).unwrap_or("/"));
    let spec_norm = request_path(proposed.get("url").and_then(Value::as_str).unwrap_or("/"));
    let local_stripped = strip_server_path(&local_norm, server_base_path);
    let spec_stripped = strip_server_path(&spec_norm, server_base_path);
    if local_norm != spec_norm && local_stripped != spec_stripped {
        diffs.push(json!({ "field": "url", "oldValue": local_norm, "newValue": spec_norm, "isConflict": false, "description": format!("Path changed from {local_norm} to {spec_norm}") }));
    }

    let local_params = existing
        .get("params")
        .and_then(Value::as_array)
        .map(Vec::as_slice)
        .unwrap_or(&[]);
    let spec_params = proposed
        .get("params")
        .and_then(Value::as_array)
        .map(Vec::as_slice)
        .unwrap_or(&[]);
    if let Some(spec_parameters) = proposed.get("openApiParameters").and_then(Value::as_array) {
        if let Some(local_parameters) = existing.get("openApiParameters").and_then(Value::as_array)
        {
            if local_parameters != spec_parameters {
                diffs.push(json!({ "field": "parameters", "oldValue": local_parameters, "newValue": spec_parameters, "isConflict": false, "description": "OpenAPI parameters, required flags, or schemas changed" }));
            }
        } else {
            // Existing collections may predate OpenAPI sync metadata. Compare their
            // visible query parameters and only report header additions; unrelated
            // custom headers are retained by the merge path.
            let local_query = local_params
                .iter()
                .filter_map(|item| item.get("key").and_then(Value::as_str))
                .filter(|key| !key.is_empty())
                .collect::<HashSet<_>>();
            let spec_query = spec_params
                .iter()
                .filter_map(|item| item.get("key").and_then(Value::as_str))
                .filter(|key| !key.is_empty())
                .collect::<HashSet<_>>();
            let local_headers = existing
                .get("headers")
                .and_then(Value::as_array)
                .into_iter()
                .flatten()
                .filter_map(|item| item.get("key").and_then(Value::as_str))
                .map(str::to_ascii_lowercase)
                .collect::<HashSet<_>>();
            let spec_headers = proposed
                .get("headers")
                .and_then(Value::as_array)
                .into_iter()
                .flatten()
                .filter_map(|item| item.get("key").and_then(Value::as_str))
                .collect::<HashSet<_>>();
            let mut added = spec_query
                .difference(&local_query)
                .map(|key| (*key).to_owned())
                .collect::<Vec<_>>();
            added.extend(
                spec_headers
                    .iter()
                    .filter(|key| !local_headers.contains(&key.to_ascii_lowercase()))
                    .map(|key| format!("header:{key}")),
            );
            let mut removed = local_query
                .difference(&spec_query)
                .map(|key| (*key).to_owned())
                .collect::<Vec<_>>();
            added.sort();
            removed.sort();
            if !added.is_empty() || !removed.is_empty() {
                diffs.push(json!({ "field": "parameters", "oldValue": &removed, "newValue": &added, "isConflict": false, "description": format!("Parameters modified: +[{}] -[{}]", added.join(" "), removed.join(" ")) }));
            }
        }
    }

    let local_body = existing.get("body").cloned().unwrap_or_else(|| json!({}));
    let spec_body = proposed.get("body").cloned().unwrap_or_else(|| json!({}));
    let local_body_type = local_body.get("type").and_then(Value::as_str).unwrap_or("");
    let spec_body_type = spec_body.get("type").and_then(Value::as_str).unwrap_or("");
    if local_body_type != spec_body_type {
        diffs.push(json!({ "field": "body.type", "oldValue": local_body_type, "newValue": spec_body_type, "isConflict": false, "description": format!("Body type changed from {local_body_type} to {spec_body_type}") }));
    } else if spec_body_type == "json" {
        let local_raw = local_body.get("raw").and_then(Value::as_str).unwrap_or("");
        let spec_raw = spec_body.get("raw").and_then(Value::as_str).unwrap_or("");
        if !spec_raw.is_empty() && json_structure(local_raw) != json_structure(spec_raw) {
            diffs.push(json!({ "field": "body.schema", "oldValue": local_raw, "newValue": spec_raw, "isConflict": false, "description": "Request body JSON schema structure was updated in spec" }));
        }
    }

    if let Some(spec_responses) = proposed
        .get("openApiResponses")
        .filter(|value| value.as_object().is_some_and(|object| !object.is_empty()))
    {
        let local_responses = existing
            .get("openApiResponses")
            .cloned()
            .unwrap_or(Value::Null);
        if &local_responses != spec_responses {
            diffs.push(json!({ "field": "responses", "oldValue": local_responses, "newValue": spec_responses, "isConflict": false, "description": "OpenAPI response statuses, schemas, or examples changed" }));
        }
    }

    let local_auth = existing
        .pointer("/auth/type")
        .and_then(Value::as_str)
        .unwrap_or("");
    let spec_auth = proposed
        .pointer("/auth/type")
        .and_then(Value::as_str)
        .unwrap_or("");
    if !matches!(spec_auth, "inherit" | "") && !local_auth.eq_ignore_ascii_case(spec_auth) {
        diffs.push(json!({ "field": "auth", "oldValue": local_auth, "newValue": spec_auth, "isConflict": false, "description": format!("Auth requirement changed from {local_auth} to {spec_auth}") }));
    }
    diffs
}

fn json_structure(raw: &str) -> Value {
    let Ok(value) = serde_json::from_str::<Value>(raw) else {
        return Value::Null;
    };
    match value {
        Value::Object(object) => Value::Object(
            object
                .into_iter()
                .map(|(key, value)| {
                    let type_name = match value {
                        Value::Null => "<nil>",
                        Value::Bool(_) => "bool",
                        Value::Number(_) => "float64",
                        Value::String(_) => "string",
                        Value::Array(_) => "[]interface {}",
                        Value::Object(_) => "map[string]interface {}",
                    };
                    (key, json!(type_name))
                })
                .collect(),
        ),
        Value::Array(_) => json!("[]interface {}"),
        Value::Null => json!("<nil>"),
        Value::Bool(_) => json!("bool"),
        Value::Number(_) => json!("float64"),
        Value::String(_) => json!("string"),
    }
}

fn conflict_info(request: &Value) -> Value {
    let has_scripts = request
        .pointer("/scripts/preRequest")
        .and_then(Value::as_str)
        .is_some_and(|value| !value.trim().is_empty())
        || request
            .pointer("/scripts/postResponse")
            .and_then(Value::as_str)
            .is_some_and(|value| !value.trim().is_empty());
    let examples = request
        .get("examples")
        .and_then(Value::as_array)
        .map(Vec::len)
        .unwrap_or(0);
    let extractors = request
        .get("extractors")
        .and_then(Value::as_array)
        .map(Vec::len)
        .unwrap_or(0);
    let headers = request
        .get("headers")
        .and_then(Value::as_array)
        .map(Vec::len)
        .unwrap_or(0);
    let has_examples = examples > 0;
    let has_extractors = extractors > 0;
    let has_custom_headers = headers > 0;
    if !has_scripts && !has_examples && !has_extractors && !has_custom_headers {
        return Value::Null;
    }
    let mut reasons = Vec::new();
    if has_scripts {
        reasons.push("pre-request / test scripts".to_owned());
    }
    if has_examples {
        reasons.push(format!("{examples} saved examples"));
    }
    if has_extractors {
        reasons.push(format!("{extractors} variable extractors"));
    }
    if has_custom_headers {
        reasons.push(format!("{headers} custom headers"));
    }
    json!({ "hasUserScripts": has_scripts, "hasUserExamples": has_examples, "hasUserExtractors": has_extractors, "hasUserCustomHeaders": has_custom_headers, "details": format!("Request contains user-defined assets: {}", reasons.join(", ")) })
}

fn merge_request(existing: &Value, spec: &Value) -> Value {
    let mut merged = existing.clone();
    {
        let field = "method";
        if let Some(value) = spec.get(field) {
            merged[field] = value.clone();
        }
    }
    let existing_url = existing.get("url").and_then(Value::as_str).unwrap_or("");
    let spec_url = spec.get("url").and_then(Value::as_str).unwrap_or("");
    merged["url"] = json!(merge_url(existing_url, spec_url));
    merged["params"] = merge_key_values(existing.get("params"), spec.get("params"));
    merged["headers"] = merge_key_values(existing.get("headers"), spec.get("headers"));
    merged["body"] = merge_body(existing.get("body"), spec.get("body"));
    for field in ["openApiParameters", "openApiResponses"] {
        if let Some(value) = spec.get(field) {
            merged[field] = value.clone();
        }
    }

    let existing_auth = existing.get("auth").cloned().unwrap_or_else(|| json!({}));
    let spec_auth = spec.get("auth").cloned().unwrap_or_else(|| json!({}));
    let existing_type = existing_auth
        .get("type")
        .and_then(Value::as_str)
        .unwrap_or("");
    let spec_type = spec_auth.get("type").and_then(Value::as_str).unwrap_or("");
    if matches!(existing_type, "inherit" | "none" | "") && !matches!(spec_type, "inherit" | "") {
        let mut auth = existing_auth;
        auth["type"] = json!(spec_type);
        for field in ["key", "addTo"] {
            if spec_auth
                .get(field)
                .and_then(Value::as_str)
                .is_some_and(|value| !value.is_empty())
            {
                auth[field] = spec_auth[field].clone();
            }
        }
        merged["auth"] = auth;
    }
    let existing_description = existing
        .get("description")
        .and_then(Value::as_str)
        .unwrap_or("");
    if existing_description.is_empty()
        && let Some(description) = spec.get("description")
    {
        merged["description"] = description.clone();
    }
    // The merged value starts from the existing request, so scripts, examples,
    // extractors, settings, identity, and other user-owned data remain intact.
    merged
}

fn merge_url(existing: &str, spec: &str) -> String {
    let spec_path = request_path(spec);
    if existing.starts_with("{{")
        && let Some(end) = existing.find("}}")
    {
        return format!("{}{}", &existing[..end + 2], spec_path);
    }
    if let Some((scheme, rest)) = existing.split_once("://") {
        let host = rest.split('/').next().unwrap_or(rest);
        return format!("{scheme}://{host}{spec_path}");
    }
    if !spec.is_empty() {
        spec.to_owned()
    } else {
        existing.to_owned()
    }
}

fn merge_key_values(existing: Option<&Value>, spec: Option<&Value>) -> Value {
    let existing_items = existing
        .and_then(Value::as_array)
        .map(Vec::as_slice)
        .unwrap_or(&[]);
    let spec_items = spec
        .and_then(Value::as_array)
        .map(Vec::as_slice)
        .unwrap_or(&[]);
    let spec_by_key = spec_items
        .iter()
        .filter_map(|item| {
            let key = item.get("key").and_then(Value::as_str)?;
            Some((key.to_ascii_lowercase(), item))
        })
        .collect::<HashMap<_, _>>();
    let mut merged = Vec::new();
    let mut seen = HashSet::new();
    for item in existing_items {
        let key = item.get("key").and_then(Value::as_str).unwrap_or("");
        let normalized = key.to_ascii_lowercase();
        seen.insert(normalized.clone());
        if let Some(spec_item) = spec_by_key.get(&normalized) {
            let old_value = item.get("value").and_then(Value::as_str).unwrap_or("");
            let value = if old_value.is_empty() {
                spec_item.get("value").cloned().unwrap_or_else(|| json!(""))
            } else {
                json!(old_value)
            };
            merged.push(json!({
                "key": spec_item.get("key").and_then(Value::as_str).unwrap_or(key),
                "value": value,
                "enabled": item.get("enabled").and_then(Value::as_bool).unwrap_or(true)
            }));
        } else {
            merged.push(item.clone());
        }
    }
    for item in spec_items {
        let key = item.get("key").and_then(Value::as_str).unwrap_or("");
        if !seen.contains(&key.to_ascii_lowercase()) {
            merged.push(item.clone());
        }
    }
    Value::Array(merged)
}

fn merge_body(existing: Option<&Value>, spec: Option<&Value>) -> Value {
    let Some(spec) = spec else {
        return existing
            .cloned()
            .unwrap_or_else(|| json!({ "type": "none" }));
    };
    let spec_type = spec.get("type").and_then(Value::as_str).unwrap_or("");
    if spec_type.is_empty() || spec_type == "none" {
        return existing
            .cloned()
            .unwrap_or_else(|| json!({ "type": "none" }));
    }
    if spec_type != "json" {
        return spec.clone();
    }
    let Some(existing) = existing else {
        return spec.clone();
    };
    if existing.get("type").and_then(Value::as_str) != Some("json") {
        return spec.clone();
    }
    let old_raw = existing.get("raw").and_then(Value::as_str).unwrap_or("");
    if old_raw.trim().is_empty() {
        return spec.clone();
    }
    let Ok(Value::Object(old_json)) = serde_json::from_str::<Value>(old_raw) else {
        return spec.clone();
    };
    let Some(spec_raw) = spec.get("raw").and_then(Value::as_str) else {
        return existing.clone();
    };
    let Ok(Value::Object(spec_json)) = serde_json::from_str::<Value>(spec_raw) else {
        return existing.clone();
    };
    let merged_json = deep_merge_json(Value::Object(old_json), Value::Object(spec_json));
    let Ok(raw) = serde_json::to_string_pretty(&merged_json) else {
        return existing.clone();
    };
    let mut body = existing.clone();
    body["type"] = json!("json");
    body["raw"] = json!(raw);
    body
}

fn deep_merge_json(existing: Value, spec: Value) -> Value {
    let mut existing = match existing {
        Value::Object(value) => value,
        other => return other,
    };
    let spec = match spec {
        Value::Object(value) => value,
        _ => return Value::Object(existing),
    };
    for (key, spec_value) in spec {
        if let Some(existing_value) = existing.remove(&key) {
            let value = match (existing_value, spec_value) {
                (Value::Object(old), Value::Object(new)) => {
                    deep_merge_json(Value::Object(old), Value::Object(new))
                }
                (old, _) => old,
            };
            existing.insert(key, value);
        } else {
            existing.insert(key, spec_value);
        }
    }
    Value::Object(existing)
}

fn slug(value: &str) -> String {
    let mut clean = String::new();
    for ch in value.trim().to_lowercase().chars() {
        if ch.is_ascii_alphanumeric() || ch == '-' || ch == '_' {
            clean.push(ch);
        } else if matches!(ch, ' ' | '.' | '/' | '\\') {
            clean.push('-');
        }
    }
    let clean = clean.trim_matches('-');
    if clean.is_empty() {
        "request".into()
    } else {
        clean.to_owned()
    }
}

fn error_response(status: StatusCode, message: String) -> Response {
    (status, Json(json!({ "error": message }))).into_response()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn sync_diff_detects_openapi_parameter_and_response_changes() {
        let local = json!({
            "method": "GET", "url": "https://api.test/v1/users", "params": [], "headers": [],
            "body": { "type": "none" }, "auth": { "type": "none" }
        });
        let proposed = json!({
            "method": "GET", "url": "https://api.test/v1/users", "params": [{ "key": "active", "value": "true", "enabled": true }],
            "headers": [], "body": { "type": "none" }, "auth": { "type": "inherit" },
            "openApiParameters": [{ "name": "active", "in": "query", "required": false, "schema": { "type": "boolean" } }],
            "openApiResponses": { "200": { "description": "Users", "content": { "application/json": { "schema": { "type": "array" } } } } }
        });
        let diffs = changed_fields(&local, &proposed, "/v1");
        assert!(diffs.iter().any(|diff| diff["field"] == "parameters"));
        assert!(diffs.iter().any(|diff| diff["field"] == "responses"));
    }

    #[test]
    fn sync_merge_tracks_spec_metadata_and_keeps_custom_request_assets() {
        let local = json!({
            "method": "GET", "url": "https://api.test/old", "params": [],
            "headers": [{ "key": "X-Custom", "value": "keep", "enabled": true }],
            "auth": { "type": "none" }, "body": { "type": "none" },
            "scripts": { "preRequest": "console.log('keep')", "postResponse": "" },
            "examples": [{ "name": "Saved", "statusCode": 200 }], "extractors": [{ "name": "token" }]
        });
        let proposed = json!({
            "method": "GET", "url": "https://spec.test/v2/users", "params": [{ "key": "active", "value": "", "enabled": true }],
            "headers": [{ "key": "X-Api-Version", "value": "2", "enabled": true }],
            "auth": { "type": "inherit" }, "body": { "type": "none" },
            "openApiParameters": [{ "name": "active", "in": "query", "required": false }],
            "openApiResponses": { "200": { "description": "OK" } }
        });
        let merged = merge_request(&local, &proposed);
        assert_eq!(merged["url"], "https://api.test/v2/users");
        assert_eq!(merged["headers"].as_array().unwrap().len(), 2);
        assert_eq!(merged["headers"][0]["key"], "X-Custom");
        assert_eq!(merged["scripts"]["preRequest"], "console.log('keep')");
        assert_eq!(merged["examples"][0]["name"], "Saved");
        assert_eq!(merged["openApiResponses"], proposed["openApiResponses"]);
        assert_eq!(merged["openApiParameters"], proposed["openApiParameters"]);
    }
}
