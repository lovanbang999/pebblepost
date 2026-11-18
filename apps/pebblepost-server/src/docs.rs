use std::fs;
use std::path::Path;

use base64::Engine;
use pebblepost_core::{PebbleError, get_workspace_info, read_folder, read_request, safe_workspace_path};
use serde_json::{Value, json};

pub fn generate(workspace: &Path, target: Option<&str>, format: &str) -> Result<Value, PebbleError> {
    let workspace = fs::canonicalize(workspace)?;
    let target = target.filter(|value| !value.trim().is_empty()).unwrap_or(".");
    let requested = Path::new(target);
    let requested = if requested.is_absolute() { requested.to_path_buf() } else { workspace.join(requested) };
    let target = safe_workspace_path(&workspace, &requested)?;
    let workspace_info = get_workspace_info(&workspace).ok();
    let fallback_title = target.file_name().and_then(|name| name.to_str()).unwrap_or("API");
    let title = workspace_info.as_ref().map(|info| info.name.as_str()).filter(|name| !name.is_empty()).unwrap_or(fallback_title);
    let version = workspace_info.as_ref().map(|info| info.version.as_str()).filter(|version| !version.is_empty()).unwrap_or("1.0.0");
    let mut collection = json!({
        "title": title,
        "description": "",
        "version": version,
        "items": []
    });
    if target.is_dir() {
      if let Some(folder) = fs::read(target.join("_folder.pebble.json")).ok().and_then(|data| serde_json::from_slice::<Value>(&data).ok()) {
        if let Some(name) = folder.get("name").and_then(Value::as_str).filter(|value| !value.is_empty()) {
            collection["title"] = json!(name);
        }
        if let Some(description) = folder.get("description") {
            collection["description"] = description.clone();
        }
      }
    }
    let items = if target.is_dir() {
        scan_items(&workspace, &target, &target)?
    } else if target.to_string_lossy().ends_with(".pebble.json") {
        let request = read_request(&workspace, &target)?;
        let request_root = target.parent().unwrap_or(&target);
        let doc_request = document_request(request_root, &target, &request);
        collection["title"] = request.get("name").cloned().unwrap_or_else(|| json!(target.file_stem().unwrap_or_default().to_string_lossy()));
        collection["description"] = request.get("description").cloned().unwrap_or_else(|| json!(""));
        vec![json!({ "isFolder": false, "request": doc_request })]
    } else {
        return Err(PebbleError::InvalidPath("documentation target must be a folder or a .pebble.json file".into()));
    };
    collection["items"] = json!(items);

    if format.eq_ignore_ascii_case("data") {
        return Ok(collection);
    }
    let (content, filename) = match format.to_ascii_lowercase().as_str() {
        "md" | "markdown" => (render_markdown(&collection), "index.md"),
        "html" | "htm" => (render_html(&collection), "index.html"),
        "openapi" | "yaml" | "yml" => (render_openapi(&collection), "openapi.yaml"),
        "json" => (render_openapi(&collection), "openapi.json"),
        other => return Err(PebbleError::InvalidPath(format!("unsupported documentation format: {other}"))),
    };
    Ok(json!({ "content": content, "filename": filename, "format": format }))
}

fn scan_items(workspace: &Path, directory: &Path, root: &Path) -> Result<Vec<Value>, PebbleError> {
    let folder_definition = read_folder(workspace, directory).unwrap_or_else(|_| json!({}));
    let item_order = folder_definition.get("itemOrder").and_then(Value::as_array)
        .into_iter().flatten().filter_map(Value::as_str).collect::<Vec<_>>();
    let mut folders = Vec::<(String, i64, String, Value)>::new();
    let mut requests = Vec::<(String, i64, String, Value)>::new();
    let entries = fs::read_dir(directory)?.collect::<Result<Vec<_>, _>>()?;
    for entry in entries {
        let name = entry.file_name().to_string_lossy().into_owned();
        if name.starts_with('.') || matches!(name.as_str(), "_folder.pebble.json" | "node_modules" | "dist") {
            continue;
        }
        let path = entry.path();
        let rel_path = path.strip_prefix(root).unwrap_or(&path).to_string_lossy().replace('\\', "/");
        if entry.file_type()?.is_dir() {
            let folder_metadata = fs::read(path.join("_folder.pebble.json")).ok()
                .and_then(|data| serde_json::from_slice::<Value>(&data).ok());
            let folder = folder_metadata.clone().unwrap_or_else(|| read_folder(workspace, &path).unwrap_or_else(|_| json!({})));
            let (prefix_order, clean_name) = parse_order_prefix(&name);
            let display_name = folder.get("name").and_then(Value::as_str).filter(|value| !value.is_empty()).unwrap_or(&clean_name);
            let order = if folder_metadata.is_some() { folder.get("order").and_then(Value::as_i64).unwrap_or_default() } else { prefix_order };
            let child_items = scan_items(workspace, &path, root)?;
            folders.push((name, order, display_name.to_owned(), json!({
                "isFolder": true,
                "folder": {
                    "name": display_name,
                    "description": folder.get("description").and_then(Value::as_str).unwrap_or(""),
                    "relPath": rel_path,
                    "order": order,
                    "items": child_items
                }
            })));
        } else if entry.file_type()?.is_file() && name.ends_with(".pebble.json") {
            if let Ok(request) = read_request(workspace, &path) {
                let clean_name = name.strip_suffix(".pebble.json").unwrap_or(&name).to_owned();
                let request_order = request.get("order").and_then(Value::as_i64).unwrap_or_default();
                let display_name = request.get("name").and_then(Value::as_str).filter(|value| !value.is_empty()).unwrap_or(&clean_name).to_owned();
                requests.push((name, request_order, display_name, json!({ "isFolder": false, "request": document_request(root, &path, &request) })));
            }
        }
    }

    let mut result = Vec::new();
    for ordered_name in item_order {
        if let Some(index) = folders.iter().position(|(name, _, _, _)| name == ordered_name) {
            let (_, _, _, item) = folders.remove(index);
            result.push(item);
        } else if let Some(index) = requests.iter().position(|(name, _, _, _)| name == ordered_name) {
            let (_, _, _, item) = requests.remove(index);
            result.push(item);
        }
    }
    folders.sort_by(|left, right| left.1.cmp(&right.1).then_with(|| left.2.to_ascii_lowercase().cmp(&right.2.to_ascii_lowercase())));
    requests.sort_by(|left, right| left.1.cmp(&right.1).then_with(|| left.2.to_ascii_lowercase().cmp(&right.2.to_ascii_lowercase())));
    result.extend(folders.into_iter().map(|(_, _, _, item)| item));
    result.extend(requests.into_iter().map(|(_, _, _, item)| item));
    Ok(result)
}

fn parse_order_prefix(name: &str) -> (i64, String) {
    let clean = name.strip_suffix(".pebble.json").or_else(|| name.strip_suffix(".json")).unwrap_or(name);
    if let Some((digits, rest)) = clean.split_once('-') {
        if !digits.is_empty() && digits.chars().all(|character| character.is_ascii_digit()) {
            if let Ok(order) = digits.parse() { return (order, rest.to_owned()); }
        }
    }
    (0, clean.to_owned())
}

fn document_request(root: &Path, path: &Path, request: &Value) -> Value {
    let url = request.get("url").and_then(Value::as_str).unwrap_or("");
    let method = request.get("method").and_then(Value::as_str).unwrap_or("GET");
    let name = request.get("name").and_then(Value::as_str).unwrap_or("Request");
    json!({
        "id": request.get("id").and_then(Value::as_str).unwrap_or(name),
        "name": name,
        "description": request.get("description").and_then(Value::as_str).unwrap_or(""),
        "method": method,
        "url": url,
        "protocol": request.get("protocol").and_then(Value::as_str).unwrap_or("http"),
        "relPath": path.strip_prefix(root).unwrap_or(path).to_string_lossy().replace('\\', "/"),
        "headers": request.get("headers").cloned().unwrap_or_else(|| json!([])),
        "params": request.get("params").cloned().unwrap_or_else(|| json!([])),
        "auth": request.get("auth").cloned().unwrap_or_else(|| json!({ "type": "none" })),
        "body": request.get("body").cloned().unwrap_or_else(|| json!({ "type": "none" })),
        "examples": request.get("examples").cloned().unwrap_or_else(|| json!([])),
        "snippets": snippets(request, method, url)
    })
}

fn snippets(request: &Value, method: &str, url: &str) -> Value {
    let url = full_request_url(request, url);
    let mut headers = request.get("headers").and_then(Value::as_array).into_iter().flatten()
        .filter(|item| item.get("enabled").and_then(Value::as_bool).unwrap_or(true))
        .filter_map(|item| Some((item.get("key")?.as_str()?.to_owned(), item.get("value").and_then(Value::as_str).unwrap_or("").to_owned())))
        .collect::<Vec<_>>();
    let auth = request.get("auth").cloned().unwrap_or_else(|| json!({}));
    let auth_type = auth.get("type").and_then(Value::as_str).unwrap_or("");
    match auth_type {
        "bearer" => {
            let token = auth.get("token").and_then(Value::as_str).unwrap_or("");
            if !token.is_empty() { headers.insert(0, ("Authorization".into(), format!("Bearer {token}"))); }
        }
        "basic" => {
            let username = auth.get("username").and_then(Value::as_str).unwrap_or("");
            let password = auth.get("password").and_then(Value::as_str).unwrap_or("");
            if !username.is_empty() || !password.is_empty() {
                let encoded = base64::engine::general_purpose::STANDARD.encode(format!("{username}:{password}"));
                headers.insert(0, ("Authorization".into(), format!("Basic {encoded}")));
            }
        }
        "apiKey" if matches!(auth.get("addTo").and_then(Value::as_str), None | Some("header")) => {
            let key = auth.get("key").and_then(Value::as_str).filter(|key| !key.is_empty()).unwrap_or("X-API-Key");
            let value = auth.get("value").and_then(Value::as_str).unwrap_or("");
            headers.insert(0, (key.to_owned(), value.to_owned()));
        }
        _ => {}
    }
    let body = request.get("body").cloned().unwrap_or_else(|| json!({}));
    let body_type = body.get("type").and_then(Value::as_str).unwrap_or("none");
    let raw_body = body.get("raw").and_then(Value::as_str).unwrap_or("");
    if matches!(body_type, "json" | "raw") && !headers.iter().any(|(key, _)| key.eq_ignore_ascii_case("content-type")) {
        headers.push(("Content-Type".into(), "application/json".into()));
    }
    let header_object = headers.iter().map(|(key, value)| (key.clone(), json!(value))).collect::<serde_json::Map<_, _>>();
    let header_json = serde_json::to_string_pretty(&Value::Object(header_object)).unwrap_or_else(|_| "{}".into());
    let escaped_url = serde_json::to_string(&url).unwrap_or_else(|_| "\"\"".into());
    let escaped_method = serde_json::to_string(method).unwrap_or_else(|_| "\"GET\"".into());
    let mut curl = format!("curl -X {} '{}'", method, shell_escape(&url));
    for (key, value) in &headers {
        curl.push_str(&format!(" \\\n  -H '{}'", shell_escape(&format!("{key}: {value}"))));
    }
    if matches!(body_type, "json" | "raw") && !raw_body.is_empty() {
        curl.push_str(&format!(" \\\n  --data-raw '{}'", shell_escape(raw_body)));
    }
    if request.pointer("/settings/verifySSL").and_then(Value::as_bool) == Some(false) { curl.push_str(" \\\n  --insecure"); }
    if request.pointer("/settings/followRedirects").and_then(Value::as_bool) == Some(false) { curl.push_str(" \\\n  --no-location"); }

    let rust_headers = headers.iter().map(|(key, value)| format!("\n    .header({}, {})", serde_json::to_string(key).unwrap_or_default(), serde_json::to_string(value).unwrap_or_default())).collect::<String>();
    let rust_body = if matches!(body_type, "json" | "raw") && !raw_body.is_empty() {
        format!("\n    .body({})", serde_json::to_string(raw_body).unwrap_or_else(|_| "String::new()".into()))
    } else { String::new() };
    let node_body = if matches!(body_type, "json" | "raw") && !raw_body.is_empty() {
        format!(",\n  body: {}", serde_json::to_string(raw_body).unwrap_or_else(|_| "\"\"".into()))
    } else { String::new() };
    let python_body = if matches!(body_type, "json" | "raw") && !raw_body.is_empty() {
        format!("\ndata = {}\n\nresponse = requests.request({}, url, headers=headers, data=data)", serde_json::to_string(raw_body).unwrap_or_else(|_| "\"\"".into()), escaped_method)
    } else { format!("\nresponse = requests.request({}, url, headers=headers)", escaped_method) };
    let has_body = matches!(body_type, "json" | "raw") && !raw_body.is_empty();
    let mut csharp_body = format!("\nvar request = new HttpRequestMessage(new HttpMethod({}), url);", escaped_method);
    for (key, value) in &headers {
        let key_literal = serde_json::to_string(key).unwrap_or_default();
        let value_literal = serde_json::to_string(value).unwrap_or_default();
        if key.eq_ignore_ascii_case("content-type") && has_body {
            csharp_body.push_str(&format!("\nrequest.Content = new StringContent({}, System.Text.Encoding.UTF8);\nrequest.Content.Headers.TryAddWithoutValidation({}, {});", serde_json::to_string(raw_body).unwrap_or_else(|_| "\"\"".into()), key_literal, value_literal));
        } else if !key.eq_ignore_ascii_case("content-type") {
            csharp_body.push_str(&format!("\nrequest.Headers.TryAddWithoutValidation({}, {});", key_literal, value_literal));
        }
    }
    if has_body && !headers.iter().any(|(key, _)| key.eq_ignore_ascii_case("content-type")) {
        csharp_body.push_str(&format!("\nrequest.Content = new StringContent({}, System.Text.Encoding.UTF8, \"application/json\");", serde_json::to_string(raw_body).unwrap_or_else(|_| "\"\"".into())));
    }
    csharp_body.push_str("\nvar response = await client.SendAsync(request);");
    json!({
        "curl": curl,
        "rust": format!("let client = reqwest::Client::new();\nlet response = client.request(reqwest::Method::from_bytes({}.as_bytes())?, {}){}{}\n    .send()\n    .await?;", escaped_method, escaped_url, rust_headers, rust_body),
        "node": format!("const response = await fetch({}, {{\n  method: {},\n  headers: {}{}\n}});\nconst data = await response.text();\nconsole.log(response.status, data);", escaped_url, escaped_method, header_json, node_body),
        "python": format!("import requests\n\nurl = {}\nheaders = {}{}\n\nprint(response.status_code)\nprint(response.text)", escaped_url, header_json, python_body),
        "csharp": format!("using System.Net.Http;\n\nusing var client = new HttpClient();\nvar url = {};{}\nConsole.WriteLine(response.StatusCode);\nConsole.WriteLine(await response.Content.ReadAsStringAsync());", escaped_url, csharp_body)
    })
}

fn shell_escape(value: &str) -> String { value.replace('\'', "'\\''") }

fn full_request_url(request: &Value, url: &str) -> String {
    let mut pairs = Vec::new();
    if let Some(params) = request.get("params").and_then(Value::as_array) {
        for param in params.iter().filter(|param| param.get("enabled").and_then(Value::as_bool).unwrap_or(true)) {
            let key = param.get("key").and_then(Value::as_str).unwrap_or("");
            let value = param.get("value").and_then(Value::as_str).unwrap_or("");
            if !key.trim().is_empty() { pairs.push(format!("{}={}", percent_encode(key), percent_encode(value))); }
        }
    }
    let auth = request.get("auth").cloned().unwrap_or_else(|| json!({}));
    if auth.get("type").and_then(Value::as_str) == Some("apiKey") && auth.get("addTo").and_then(Value::as_str) == Some("query") {
        let key = auth.get("key").and_then(Value::as_str).filter(|key| !key.is_empty()).unwrap_or("X-API-Key");
        let value = auth.get("value").and_then(Value::as_str).unwrap_or("");
        pairs.push(format!("{}={}", percent_encode(key), percent_encode(value)));
    }
    if pairs.is_empty() { return url.to_owned(); }
    format!("{}{}{}", url, if url.contains('?') { "&" } else { "?" }, pairs.join("&"))
}

fn percent_encode(value: &str) -> String {
    let mut encoded = String::new();
    for byte in value.bytes() {
        if byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'_' | b'.' | b'~') { encoded.push(byte as char); }
        else { encoded.push_str(&format!("%{byte:02X}")); }
    }
    encoded
}

fn collect_requests(items: &[Value], out: &mut Vec<Value>) {
    for item in items {
        if item.get("isFolder").and_then(Value::as_bool).unwrap_or(false) {
            if let Some(children) = item.pointer("/folder/items").and_then(Value::as_array) { collect_requests(children, out); }
        } else if let Some(request) = item.get("request") { out.push(request.clone()); }
    }
}

fn render_markdown(collection: &Value) -> String {
    let title = collection.get("title").and_then(Value::as_str).filter(|value| !value.is_empty()).unwrap_or("API Documentation");
    let description = collection.get("description").and_then(Value::as_str).unwrap_or("");
    let items = collection.get("items").and_then(Value::as_array).map(Vec::as_slice).unwrap_or(&[]);
    let mut out = format!("# {title}\n\n");
    if !description.is_empty() { out.push_str(description); out.push_str("\n\n"); }
    out.push_str("## Table of Contents\n\n");
    write_markdown_toc(&mut out, items, 0);
    out.push_str("\n---\n\n");
    write_markdown_items(&mut out, items, 2);
    out
}

fn write_markdown_toc(out: &mut String, items: &[Value], depth: usize) {
    for item in items {
        if item.get("isFolder").and_then(Value::as_bool).unwrap_or(false) {
            if let Some(folder) = item.get("folder") {
                let name = folder.get("name").and_then(Value::as_str).unwrap_or("Folder");
                out.push_str(&format!("{}- [{}](#{})\n", "  ".repeat(depth), name.replace('[', "\\[").replace(']', "\\]"), slugify(name)));
                if let Some(children) = folder.get("items").and_then(Value::as_array) { write_markdown_toc(out, children, depth + 1); }
            }
        } else if let Some(request) = item.get("request") {
            let method = request.get("method").and_then(Value::as_str).filter(|value| !value.is_empty()).unwrap_or("GET");
            let name = request.get("name").and_then(Value::as_str).unwrap_or("Request");
            out.push_str(&format!("{}- [`{method}` {}](#{})\n", "  ".repeat(depth), name.replace('[', "\\[").replace(']', "\\]"), slugify(&format!("{method}-{name}"))));
        }
    }
}

fn write_markdown_items(out: &mut String, items: &[Value], heading_level: usize) {
    for item in items {
        if item.get("isFolder").and_then(Value::as_bool).unwrap_or(false) {
            if let Some(folder) = item.get("folder") {
                let name = folder.get("name").and_then(Value::as_str).unwrap_or("Folder");
                let level = heading_level.min(6);
                out.push_str(&format!("{} {name}\n\n", "#".repeat(level)));
                if let Some(description) = folder.get("description").and_then(Value::as_str).filter(|value| !value.is_empty()) { out.push_str(description); out.push_str("\n\n"); }
                if let Some(children) = folder.get("items").and_then(Value::as_array) { write_markdown_items(out, children, heading_level + 1); }
            }
        } else if let Some(request) = item.get("request") {
            write_request_markdown(out, request, heading_level.min(6));
        }
    }
}

fn write_request_markdown(out: &mut String, request: &Value, heading_level: usize) {
    let method = request.get("method").and_then(Value::as_str).filter(|value| !value.is_empty()).unwrap_or("GET");
    let name = request.get("name").and_then(Value::as_str).unwrap_or("Request");
    let url = request.get("url").and_then(Value::as_str).unwrap_or("");
    let heading = "#".repeat(heading_level.max(2).min(6));
    out.push_str(&format!("{heading} {method} {name}\n\n"));
    if let Some(description) = request.get("description").and_then(Value::as_str).filter(|value| !value.is_empty()) { out.push_str(description); out.push_str("\n\n"); }
    out.push_str(&format!("**Endpoint**: `{method} {url}`\n\n"));
    write_markdown_key_values(out, "Query Parameters", "Parameter", "Example Value", request.get("params"));
    write_markdown_key_values(out, "Headers", "Header", "Value", request.get("headers"));

    if let Some(body) = request.get("body") {
        match body.get("type").and_then(Value::as_str).unwrap_or("none") {
            "json" | "raw" => if let Some(raw) = body.get("raw").and_then(Value::as_str).filter(|raw| !raw.trim().is_empty()) {
                out.push_str(&format!("#### Request Body\n\n```json\n{raw}\n```\n\n"));
            },
            "urlEncoded" => write_markdown_key_values(out, "Form URL-Encoded", "Key", "Value", body.get("urlEncoded")),
            "formData" => write_markdown_key_values(out, "Form Data", "Key", "Value", body.get("formData")),
            _ => {}
        }
    }

    if let Some(snippets) = request.get("snippets") {
        out.push_str("#### Code Samples\n\n");
        for (label, key, language) in [("cURL", "curl", "bash"), ("Rust", "rust", "rust"), ("Node.js (Fetch)", "node", "javascript"), ("Python", "python", "python"), ("C#", "csharp", "csharp")] {
            let code = snippets.get(key).and_then(Value::as_str).unwrap_or("");
            out.push_str(&format!("<details><summary><b>{label}</b></summary>\n\n```{language}\n{code}\n```\n\n</details>\n\n"));
        }
    }

    if let Some(examples) = request.get("examples").and_then(Value::as_array).filter(|items| !items.is_empty()) {
        out.push_str("#### Example Responses\n\n");
        for example in examples {
            let status = example.get("statusCode").and_then(Value::as_i64).unwrap_or_default();
            let status_text = example.get("statusText").and_then(Value::as_str).unwrap_or("");
            let fallback_name = format!("{status} {status_text}");
            let name = example.get("name").and_then(Value::as_str).filter(|name| !name.is_empty()).unwrap_or(&fallback_name);
            out.push_str(&format!("##### {name} (`{status} {status_text}`)\n\n"));
            if let Some(headers) = example.get("headers").and_then(Value::as_array).filter(|headers| !headers.is_empty()) {
                out.push_str("<details><summary>Response Headers</summary>\n\n| Header | Value |\n| :--- | :--- |\n");
                for header in headers { out.push_str(&format!("| `{}` | `{}` |\n", value_text(header.get("key")), value_text(header.get("value")))); }
                out.push_str("\n</details>\n\n");
            }
            if let Some(body) = example.get("body").and_then(Value::as_str).filter(|body| !body.is_empty()) {
                let content_type = example.get("contentType").and_then(Value::as_str).unwrap_or("");
                let trimmed = body.trim_start();
                let language = if content_type.contains("json") || trimmed.starts_with('{') || trimmed.starts_with('[') { "json" } else { "text" };
                out.push_str(&format!("```{language}\n{body}\n```\n\n"));
            }
        }
    }
    out.push_str("---\n\n");
}

fn write_markdown_key_values(out: &mut String, title: &str, first_column: &str, second_column: &str, values: Option<&Value>) {
    let rows = values.and_then(Value::as_array).into_iter().flatten().filter(|row| row.get("enabled").and_then(Value::as_bool).unwrap_or(true))
        .filter(|row| row.get("key").and_then(Value::as_str).is_some_and(|key| !key.trim().is_empty())).collect::<Vec<_>>();
    if rows.is_empty() { return; }
    out.push_str(&format!("#### {title}\n\n| {first_column} | {second_column} |\n| :--- | :--- |\n"));
    for row in rows { out.push_str(&format!("| `{}` | `{}` |\n", value_text(row.get("key")), value_text(row.get("value")))); }
    out.push('\n');
}

fn value_text(value: Option<&Value>) -> &str { value.and_then(Value::as_str).unwrap_or("") }

fn slugify(text: &str) -> String {
    let slug = text.to_ascii_lowercase().chars().map(|character| {
        if character.is_ascii_alphanumeric() || character == '_' { character }
        else if matches!(character, ' ' | '/' | '.' | '-' | ':') { '-' }
        else { '\0' }
    }).filter(|character| *character != '\0').collect::<String>();
    let slug = slug.trim_matches('-').to_owned();
    if slug.is_empty() { "item".into() } else { slug }
}

fn render_html(collection: &Value) -> String {
    let title = html_escape(collection.get("title").and_then(Value::as_str).filter(|title| !title.is_empty()).unwrap_or("API Documentation"));
    let description = html_escape(collection.get("description").and_then(Value::as_str).unwrap_or(""));
    let items = collection.get("items").and_then(Value::as_array).map(Vec::as_slice).unwrap_or(&[]);
    let mut sidebar = String::new();
    let mut content = String::new();
    write_html_tree(&mut sidebar, &mut content, items);
    let template = r#"<!doctype html>
<html lang="en"><head>
  <meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%%TITLE%% - PebblePost</title>
  <style>
    :root{color-scheme:light dark;--bg:#fff;--fg:#18181b;--muted:#71717a;--border:#e4e4e7;--side:#f4f4f5;--card:#fff;--code:#18181b;--codefg:#f4f4f5;--accent:#2563eb;--font:system-ui,-apple-system,"Segoe UI",sans-serif;--mono:ui-monospace,SFMono-Regular,Menlo,monospace}
    @media(prefers-color-scheme:dark){:root{--bg:#09090b;--fg:#fafafa;--muted:#a1a1aa;--border:#27272a;--side:#111113;--card:#18181b;--code:#000;--accent:#60a5fa}}
    [data-theme=light]{--bg:#fff;--fg:#18181b;--muted:#71717a;--border:#e4e4e7;--side:#f4f4f5;--card:#fff;--code:#18181b;--accent:#2563eb}
    [data-theme=dark]{--bg:#09090b;--fg:#fafafa;--muted:#a1a1aa;--border:#27272a;--side:#111113;--card:#18181b;--code:#000;--accent:#60a5fa}
    *{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--fg);font:15px/1.55 var(--font);display:flex;min-height:100vh}
    aside{width:300px;flex:0 0 300px;background:var(--side);border-right:1px solid var(--border);padding:20px 14px;position:sticky;top:0;height:100vh;overflow:auto}
    .brand{font-size:18px;font-weight:700;margin:0 4px 3px}.sidebar-desc{font-size:12px;color:var(--muted);margin:0 4px 18px}
    #search{width:100%;padding:8px 10px;border:1px solid var(--border);border-radius:7px;background:var(--card);color:var(--fg);margin-bottom:14px}
    nav{display:flex;flex-direction:column;gap:2px}.nav-folder{padding:12px 8px 4px;color:var(--muted);font-size:11px;font-weight:700;letter-spacing:.06em;text-transform:uppercase}
    .nav-link{display:flex;gap:8px;align-items:center;padding:6px 8px;border-radius:6px;color:var(--fg);text-decoration:none;font-size:13px}
    .nav-link:hover{background:color-mix(in srgb,var(--accent) 10%,transparent)}.method{display:inline-block;min-width:48px;text-align:center;border-radius:4px;padding:1px 5px;font-size:10px;font-weight:800;background:#e0e7ff;color:#3730a3}
    .method.post{background:#dcfce7;color:#166534}.method.put,.method.patch{background:#fef3c7;color:#92400e}.method.delete{background:#fee2e2;color:#991b1b}.method.other{background:#e4e4e7;color:#3f3f46}
    main{width:min(100%,1100px);padding:36px clamp(22px,5vw,72px) 80px;margin:0 auto}.top{display:flex;justify-content:flex-end;margin-bottom:24px}
    button{font:inherit;color:var(--fg);background:var(--card);border:1px solid var(--border);border-radius:6px;padding:5px 10px;cursor:pointer}
    .hero{border-bottom:1px solid var(--border);padding-bottom:22px;margin-bottom:32px}.hero h1{font-size:34px;line-height:1.2;margin:0 0 10px}.hero p{color:var(--muted);margin:0}
    .folder-section{margin:34px 0}.folder-section>h2{font-size:24px;border-bottom:1px solid var(--border);padding-bottom:8px}.folder-desc{color:var(--muted)}
    .request-card{background:var(--card);border:1px solid var(--border);border-radius:10px;padding:20px;margin:20px 0;scroll-margin-top:20px}
    .request-title{display:flex;align-items:center;gap:10px;margin:0 0 10px;font-size:20px}.endpoint{font-family:var(--mono);font-size:13px;overflow-wrap:anywhere;background:var(--side);padding:10px 12px;border-radius:6px}
    .request-desc{color:var(--muted);white-space:pre-wrap}.request-card h4{font-size:14px;margin:20px 0 8px}
    table{width:100%;border-collapse:collapse;font-size:13px}th,td{text-align:left;padding:7px 9px;border-bottom:1px solid var(--border);overflow-wrap:anywhere}th{color:var(--muted)}
    pre{background:var(--code);color:var(--codefg);padding:14px;border-radius:7px;overflow:auto;font:12px/1.6 var(--mono);white-space:pre-wrap;overflow-wrap:anywhere}
    details{border:1px solid var(--border);border-radius:7px;padding:10px 12px;margin:8px 0}summary{cursor:pointer;font-weight:600}.sample-label{color:var(--muted);font-size:12px}
    @media(max-width:760px){body{display:block}aside{position:relative;width:100%;height:auto;max-height:42vh;border-right:0;border-bottom:1px solid var(--border)}main{padding:24px 16px 60px}.hero h1{font-size:28px}}
  </style></head>
<body><aside><div class="brand">%%TITLE%%</div><p class="sidebar-desc">%%DESCRIPTION%%</p><input id="search" type="search" placeholder="Search endpoints…" aria-label="Search endpoints"><nav>%%SIDEBAR%%</nav></aside>
<main><div class="top"><button id="theme" type="button">Toggle theme</button></div><header class="hero"><h1>%%TITLE%%</h1><p>%%DESCRIPTION%%</p></header>%%CONTENT%%</main>
<script>
  const root=document.documentElement;document.getElementById('theme').addEventListener('click',()=>{root.dataset.theme=root.dataset.theme==='dark'?'light':'dark'});
  document.getElementById('search').addEventListener('input',event=>{const query=event.target.value.toLowerCase();document.querySelectorAll('.nav-link').forEach(item=>item.hidden=!item.dataset.search.includes(query));document.querySelectorAll('.request-card').forEach(item=>item.hidden=!item.dataset.search.includes(query));});
</script></body></html>"#;
    fill_html_template(template, &[("%%TITLE%%", title.as_str()), ("%%DESCRIPTION%%", description.as_str()), ("%%SIDEBAR%%", sidebar.as_str()), ("%%CONTENT%%", content.as_str())])
}

fn fill_html_template(template: &str, replacements: &[(&str, &str)]) -> String {
    let mut output = String::with_capacity(template.len() + replacements.iter().map(|(_, value)| value.len()).sum::<usize>());
    let mut offset = 0;
    while let Some(relative_start) = template[offset..].find("%%") {
        let start = offset + relative_start;
        let Some(relative_end) = template[start + 2..].find("%%") else { break; };
        let end = start + 2 + relative_end + 2;
        let token = &template[start..end];
        output.push_str(&template[offset..start]);
        let mut replacement = None;
        for (placeholder, value) in replacements {
            if *placeholder == token { replacement = Some(*value); break; }
        }
        output.push_str(replacement.unwrap_or(token));
        offset = end;
    }
    output.push_str(&template[offset..]);
    output
}

fn write_html_tree(sidebar: &mut String, content: &mut String, items: &[Value]) {
    for item in items {
        if item.get("isFolder").and_then(Value::as_bool).unwrap_or(false) {
            if let Some(folder) = item.get("folder") {
                let name = folder.get("name").and_then(Value::as_str).unwrap_or("Folder");
                let safe_name = html_escape(name);
                sidebar.push_str(&format!("<div class=\"nav-folder\">{safe_name}</div>"));
                content.push_str(&format!("<section class=\"folder-section\"><h2>{safe_name}</h2>"));
                if let Some(description) = folder.get("description").and_then(Value::as_str).filter(|value| !value.is_empty()) {
                    content.push_str(&format!("<p class=\"folder-desc\">{}</p>", html_escape(description)));
                }
                content.push_str("<div class=\"folder-items\">");
                if let Some(children) = folder.get("items").and_then(Value::as_array) { write_html_tree(sidebar, content, children); }
                content.push_str("</div></section>");
            }
        } else if let Some(request) = item.get("request") {
            write_html_request(sidebar, content, request);
        }
    }
}

fn write_html_request(sidebar: &mut String, content: &mut String, request: &Value) {
    let name = request.get("name").and_then(Value::as_str).unwrap_or("Request");
    let method = request.get("method").and_then(Value::as_str).filter(|value| !value.is_empty()).unwrap_or("GET");
    let url = request.get("url").and_then(Value::as_str).unwrap_or("");
    let id = slugify(&format!("{method}-{name}"));
    let search = html_escape(&format!("{method} {name} {url}").to_ascii_lowercase());
    let safe_name = html_escape(name);
    let safe_method = html_escape(method);
    let class = match method.to_ascii_lowercase().as_str() { "get" => "get", "post" => "post", "put" => "put", "patch" => "patch", "delete" => "delete", _ => "other" };
    sidebar.push_str(&format!("<a class=\"nav-link\" href=\"#{id}\" data-search=\"{search}\"><span class=\"method {class}\">{safe_method}</span><span>{safe_name}</span></a>"));
    content.push_str(&format!("<article class=\"request-card\" id=\"{id}\" data-search=\"{search}\"><h3 class=\"request-title\"><span class=\"method {class}\">{safe_method}</span>{safe_name}</h3>"));
    content.push_str(&format!("<div class=\"endpoint\">{} {}</div>", safe_method, html_escape(url)));
    if let Some(description) = request.get("description").and_then(Value::as_str).filter(|value| !value.is_empty()) {
        content.push_str(&format!("<p class=\"request-desc\">{}</p>", html_escape(description)));
    }
    write_html_key_values(content, "Query Parameters", request.get("params"));
    write_html_key_values(content, "Headers", request.get("headers"));
    if let Some(body) = request.get("body") {
        if let Some(raw) = body.get("raw").and_then(Value::as_str).filter(|raw| !raw.trim().is_empty()) {
            content.push_str(&format!("<h4>Request Body</h4><pre>{}</pre>", html_escape(raw)));
        }
    }
    if let Some(snippets) = request.get("snippets") {
        content.push_str("<h4>Client Code Samples</h4>");
        for (label, key) in [("cURL", "curl"), ("Rust", "rust"), ("Node.js", "node"), ("Python", "python"), ("C#", "csharp")] {
            let code = snippets.get(key).and_then(Value::as_str).unwrap_or("");
            content.push_str(&format!("<details><summary>{label}</summary><pre><code>{}</code></pre></details>", html_escape(code)));
        }
    }
    if let Some(examples) = request.get("examples").and_then(Value::as_array).filter(|examples| !examples.is_empty()) {
        content.push_str("<h4>Example Responses</h4>");
        for example in examples {
            let name = example.get("name").and_then(Value::as_str).unwrap_or("Response");
            let status = example.get("statusCode").and_then(Value::as_i64).unwrap_or_default();
            let status_text = example.get("statusText").and_then(Value::as_str).unwrap_or("");
            content.push_str(&format!("<details><summary>{} <span class=\"sample-label\">{} {}</span></summary>", html_escape(name), status, html_escape(status_text)));
            write_html_key_values(content, "Response Headers", example.get("headers"));
            if let Some(body) = example.get("body").and_then(Value::as_str).filter(|body| !body.is_empty()) {
                content.push_str(&format!("<pre>{}</pre>", html_escape(body)));
            }
            content.push_str("</details>");
        }
    }
    content.push_str("</article>");
}

fn write_html_key_values(out: &mut String, title: &str, values: Option<&Value>) {
    let rows = values.and_then(Value::as_array).into_iter().flatten()
        .filter(|row| row.get("enabled").and_then(Value::as_bool).unwrap_or(true))
        .filter(|row| row.get("key").and_then(Value::as_str).is_some_and(|key| !key.trim().is_empty())).collect::<Vec<_>>();
    if rows.is_empty() { return; }
    out.push_str(&format!("<h4>{}</h4><table><thead><tr><th>Name</th><th>Value</th></tr></thead><tbody>", html_escape(title)));
    for row in rows {
        out.push_str(&format!("<tr><td>{}</td><td>{}</td></tr>", html_escape(value_text(row.get("key"))), html_escape(value_text(row.get("value")))));
    }
    out.push_str("</tbody></table>");
}

fn render_openapi(collection: &Value) -> String {
    let mut requests = Vec::new();
    if let Some(items) = collection.get("items").and_then(Value::as_array) { collect_requests(items, &mut requests); }
    let mut servers = std::collections::BTreeSet::new();
    let mut paths = serde_json::Map::new();
    for request in requests {
        let url = request.get("url").and_then(Value::as_str).unwrap_or("/");
        let (server, path) = openapi_server_path(url);
        if !server.is_empty() { servers.insert(server); }
        let path = openapi_path_parameters(&path);
        let method = request.get("method").and_then(Value::as_str).filter(|value| !value.is_empty()).unwrap_or("GET").to_ascii_lowercase();
        let mut operation = openapi_operation(&request, &path);
        if let Some(parent) = request.get("relPath").and_then(Value::as_str).and_then(|path| path.split('/').next()).filter(|part| !part.is_empty()) {
            if let Some(object) = operation.as_object_mut() { object.insert("tags".into(), json!([parent])); }
        }
        let path_item = paths.entry(path).or_insert_with(|| json!({}));
        if let Some(path_item) = path_item.as_object_mut() { path_item.insert(method, operation); }
    }
    let servers = if servers.is_empty() { vec![json!({ "url": "https://api.example.com" })] }
        else { servers.into_iter().map(|url| json!({ "url": url })).collect() };
    serde_json::to_string_pretty(&json!({
        "openapi": "3.0.3",
        "info": {
            "title": collection.get("title").and_then(Value::as_str).filter(|value| !value.is_empty()).unwrap_or("PebblePost API"),
            "version": collection.get("version").and_then(Value::as_str).filter(|value| !value.is_empty()).unwrap_or("1.0.0"),
            "description": collection.get("description").and_then(Value::as_str).unwrap_or("")
        },
        "servers": servers,
        "paths": paths
    })).unwrap_or_else(|_| "{}".into())
}

fn openapi_server_path(raw_url: &str) -> (String, String) {
    let raw_url = raw_url.trim();
    if raw_url.starts_with("{{") {
        if let Some(end) = raw_url.find("}}") {
            let server = raw_url[..end + 2].to_owned();
            let path = raw_url[end + 2..].split('?').next().unwrap_or("");
            return (server, if path.is_empty() { "/".into() } else { path.to_owned() });
        }
    }
    if let Some((scheme, rest)) = raw_url.split_once("://") {
        let boundary = rest.find(|character| matches!(character, '/' | '?' | '#')).unwrap_or(rest.len());
        let authority = &rest[..boundary];
        let suffix = &rest[boundary..];
        let path = suffix.split('?').next().unwrap_or("").split('#').next().unwrap_or("");
        let path = if path.is_empty() { "/" } else { path };
        return (format!("{scheme}://{authority}"), path.to_owned());
    }
    let path = raw_url.split('?').next().unwrap_or(raw_url);
    (String::new(), if path.is_empty() { "/".into() } else { path.to_owned() })
}

fn openapi_path_parameters(path: &str) -> String {
    let path = path.split('?').next().unwrap_or(path);
    let mut result = String::new();
    let characters = path.chars().collect::<Vec<_>>();
    let mut index = 0;
    while index < characters.len() {
        if characters[index] == ':' && characters.get(index + 1).is_some_and(|character| character.is_ascii_alphanumeric() || *character == '_') {
            index += 1;
            let start = index;
            while index < characters.len() && (characters[index].is_ascii_alphanumeric() || characters[index] == '_') { index += 1; }
            result.push('{');
            result.extend(characters[start..index].iter().copied());
            result.push('}');
        } else {
            result.push(characters[index]);
            index += 1;
        }
    }
    if result.starts_with('/') { result } else { format!("/{result}") }
}

fn openapi_operation(request: &Value, path: &str) -> Value {
    let name = request.get("name").and_then(Value::as_str).unwrap_or("Request");
    let description = request.get("description").and_then(Value::as_str).unwrap_or("");
    let mut operation = json!({ "summary": name, "responses": { "200": { "description": "Successful response" } } });
    if !description.is_empty() { operation["description"] = json!(description); }

    if let Some(examples) = request.get("examples").and_then(Value::as_array).filter(|items| !items.is_empty()) {
        let mut responses = serde_json::Map::new();
        for example in examples {
            let status = example.get("statusCode").and_then(Value::as_i64).unwrap_or_default();
            let status_key = if status == 0 { "default".to_owned() } else { status.to_string() };
            let response_description = example.get("statusText").and_then(Value::as_str).filter(|value| !value.is_empty())
                .or_else(|| example.get("name").and_then(Value::as_str).filter(|value| !value.is_empty())).unwrap_or("Example response");
            let content_type = example.get("contentType").and_then(Value::as_str).filter(|value| !value.is_empty()).unwrap_or("application/json");
            let mut response = json!({ "description": response_description });
            if let Some(body) = example.get("body").and_then(Value::as_str).filter(|body| !body.is_empty()) {
                let value = serde_json::from_str::<Value>(body).unwrap_or_else(|_| json!(body));
                let mut content = serde_json::Map::new();
                content.insert(content_type.to_owned(), json!({ "example": value }));
                response["content"] = Value::Object(content);
            }
            responses.insert(status_key, response);
        }
        operation["responses"] = Value::Object(responses);
    }

    let mut parameters = Vec::new();
    let mut seen_path_parameters = std::collections::HashSet::new();
    let mut remaining = path;
    while let Some(start) = remaining.find('{') {
        let after_start = &remaining[start + 1..];
        let Some(end) = after_start.find('}') else { break; };
        let name = &after_start[..end];
        if !name.is_empty() && seen_path_parameters.insert(name.to_owned()) {
            parameters.push(json!({ "name": name, "in": "path", "required": true, "schema": { "type": "string" } }));
        }
        remaining = &after_start[end + 1..];
    }
    append_openapi_parameters(&mut parameters, request.get("params"), "query");
    if let Some(headers) = request.get("headers").and_then(Value::as_array) {
        for header in headers.iter().filter(|header| header.get("enabled").and_then(Value::as_bool).unwrap_or(false)) {
            let key = header.get("key").and_then(Value::as_str).unwrap_or("");
            if key.trim().is_empty() || matches!(key.to_ascii_lowercase().as_str(), "content-type" | "authorization" | "accept" | "user-agent") { continue; }
            parameters.push(json!({ "name": key, "in": "header", "required": false, "schema": { "type": "string", "example": header.get("value").and_then(Value::as_str).unwrap_or("") } }));
        }
    }
    if !parameters.is_empty() { operation["parameters"] = json!(parameters); }
    if let Some(body) = request.get("body") {
        if let Some(request_body) = openapi_request_body(body) { operation["requestBody"] = request_body; }
    }
    operation
}

fn append_openapi_parameters(parameters: &mut Vec<Value>, values: Option<&Value>, location: &str) {
    if let Some(values) = values.and_then(Value::as_array) {
        for value in values.iter().filter(|value| value.get("enabled").and_then(Value::as_bool).unwrap_or(false)) {
            let key = value.get("key").and_then(Value::as_str).unwrap_or("");
            if key.is_empty() { continue; }
            parameters.push(json!({ "name": key, "in": location, "required": false, "schema": { "type": "string", "example": value.get("value").and_then(Value::as_str).unwrap_or("") } }));
        }
    }
}

fn openapi_request_body(body: &Value) -> Option<Value> {
    match body.get("type").and_then(Value::as_str).unwrap_or("none") {
        "json" => {
            let raw = body.get("raw").and_then(Value::as_str).unwrap_or("");
            let schema = serde_json::from_str::<Value>(raw).map(|value| infer_openapi_schema(&value)).unwrap_or_else(|_| json!({ "type": "object" }));
            Some(json!({ "required": true, "content": { "application/json": { "schema": schema } } }))
        }
        "urlEncoded" | "formData" => {
            let is_form = body.get("type").and_then(Value::as_str) == Some("formData");
            let items = body.get(if is_form { "formData" } else { "urlEncoded" }).and_then(Value::as_array).cloned().unwrap_or_default();
            let mut properties = serde_json::Map::new();
            for item in items {
                let key = item.get("key").and_then(Value::as_str).unwrap_or("");
                if key.is_empty() { continue; }
                let property = if is_form && item.get("type").and_then(Value::as_str) == Some("file") { json!({ "type": "string", "format": "binary" }) }
                    else { json!({ "type": "string", "example": item.get("value").and_then(Value::as_str).unwrap_or("") }) };
                properties.insert(key.to_owned(), property);
            }
            let content_type = if is_form { "multipart/form-data" } else { "application/x-www-form-urlencoded" };
            let mut media_type = serde_json::Map::new();
            media_type.insert(content_type.to_owned(), json!({ "schema": { "type": "object", "properties": properties } }));
            let mut content = serde_json::Map::new();
            content.insert("content".into(), Value::Object(media_type));
            Some(Value::Object(content))
        }
        "raw" => {
            let raw = body.get("raw").and_then(Value::as_str).unwrap_or("");
            if raw.trim().is_empty() { None } else { Some(json!({ "content": { "text/plain": { "schema": { "type": "string", "example": raw } } } })) }
        }
        _ => None,
    }
}

fn infer_openapi_schema(value: &Value) -> Value {
    match value {
        Value::Null => json!({ "type": "string", "nullable": true }),
        Value::Bool(_) => json!({ "type": "boolean", "example": value }),
        Value::Number(number) if number.as_i64().is_some() => json!({ "type": "integer", "example": value }),
        Value::Number(_) => json!({ "type": "number", "example": value }),
        Value::String(_) => json!({ "type": "string", "example": value }),
        Value::Array(items) => json!({ "type": "array", "items": items.first().map(infer_openapi_schema).unwrap_or_else(|| json!({})) }),
        Value::Object(object) => {
            let properties = object.iter().map(|(key, value)| (key.clone(), infer_openapi_schema(value))).collect::<serde_json::Map<_, _>>();
            json!({ "type": "object", "properties": properties })
        }
    }
}

fn html_escape(value: &str) -> String {
    value.replace('&', "&amp;").replace('<', "&lt;").replace('>', "&gt;").replace('"', "&#34;").replace('\'', "&#39;")
}

