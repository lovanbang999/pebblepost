use std::fs;
use std::io;
use std::path::Path;

use rusqlite::{Connection, OptionalExtension, params};
use serde_json::{Value, json};
use tokio::sync::Mutex;

pub struct HistoryStore {
    connection: Mutex<Connection>,
}

impl HistoryStore {
    pub fn open(data_dir: &Path) -> io::Result<Self> {
        let directory = data_dir.join("history");
        fs::create_dir_all(&directory)?;
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            fs::set_permissions(&directory, fs::Permissions::from_mode(0o700))?;
        }
        let path = directory.join("history.db");
        let connection = Connection::open(&path).map_err(database_error)?;
        connection.busy_timeout(std::time::Duration::from_secs(5)).map_err(database_error)?;
        connection.execute_batch(
            "PRAGMA journal_mode=WAL;
             CREATE TABLE IF NOT EXISTS history (
               id INTEGER PRIMARY KEY AUTOINCREMENT,
               workspace_path TEXT NOT NULL,
               request_name TEXT NOT NULL DEFAULT '',
               method TEXT NOT NULL DEFAULT '',
               url TEXT NOT NULL DEFAULT '',
               status_code INTEGER NOT NULL DEFAULT 0,
               duration_ms REAL NOT NULL DEFAULT 0,
               size_bytes INTEGER NOT NULL DEFAULT 0,
               response_body TEXT NOT NULL DEFAULT '',
               response_headers TEXT NOT NULL DEFAULT '{}',
               resolved_request TEXT NOT NULL DEFAULT '{}',
               executed_at TEXT NOT NULL
             );
             CREATE INDEX IF NOT EXISTS idx_history_workspace ON history(workspace_path, executed_at DESC);
             CREATE INDEX IF NOT EXISTS idx_history_status ON history(workspace_path, status_code);",
        ).map_err(database_error)?;
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            fs::set_permissions(&path, fs::Permissions::from_mode(0o600))?;
            let wal = directory.join("history.db-wal");
            if wal.exists() { fs::set_permissions(wal, fs::Permissions::from_mode(0o600))?; }
        }
        Ok(Self { connection: Mutex::new(connection) })
    }

    pub async fn record(&self, workspace: &str, request: &Value, result: &Value, secrets: &[String]) -> Result<(), String> {
        if workspace.is_empty() { return Ok(()); }
        let mut body = result.get("body").and_then(Value::as_str).unwrap_or("").to_owned();
        if body.len() > 100 * 1024 {
            let mut end = 100 * 1024;
            while !body.is_char_boundary(end) { end -= 1; }
            body.truncate(end);
            body.push_str("\n…[truncated]");
        }
        body = mask(body, secrets);

        let mut headers = result.pointer("/sentRequest/headers").cloned().unwrap_or_else(|| json!({}));
        if let Some(values) = headers.as_object_mut() {
            for key in ["authorization", "proxy-authorization", "cookie", "set-cookie", "x-api-key", "x-auth-token"] {
                if let Some(value) = values.get_mut(key) { *value = json!(["[REDACTED]"]); }
            }
        }
        let safe_request = json!({
            "method": request.get("method").cloned().unwrap_or(Value::Null),
            "url": request.get("url").cloned().unwrap_or(Value::Null),
            "headers": headers,
        });
        let safe_request = mask(safe_request.to_string(), secrets);
        let mut response_headers = result.get("headers").cloned().unwrap_or_else(|| json!({}));
        if let Some(values) = response_headers.as_object_mut() {
            for (key, value) in values {
                if matches!(key.to_ascii_lowercase().as_str(), "set-cookie" | "cookie" | "authorization" | "proxy-authorization") {
                    *value = json!(["[REDACTED]"]);
                } else {
                    mask_value(value, secrets);
                }
            }
        }
        let response_headers = response_headers.to_string();
        let executed_at = result.get("executedAt").and_then(Value::as_str)
            .filter(|value| !value.is_empty())
            .map(str::to_owned)
            .unwrap_or_else(|| chrono::Utc::now().to_rfc3339());
        let conn = self.connection.lock().await;
        conn.execute(
            "INSERT INTO history (workspace_path, request_name, method, url, status_code, duration_ms, size_bytes, response_body, response_headers, resolved_request, executed_at) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11)",
            params![
                workspace,
                request.get("name").and_then(Value::as_str).unwrap_or(""),
                request.get("method").and_then(Value::as_str).unwrap_or(""),
                request.get("url").and_then(Value::as_str).unwrap_or(""),
                result.get("statusCode").and_then(Value::as_i64).unwrap_or(0),
                result.pointer("/timing/totalDurationMs").and_then(Value::as_f64).unwrap_or(0.0),
                result.get("size").and_then(Value::as_i64).unwrap_or(0),
                body,
                response_headers,
                safe_request,
                executed_at,
            ],
        ).map_err(|error| error.to_string())?;
        conn.execute(
            "DELETE FROM history WHERE workspace_path = ?1 AND id NOT IN (SELECT id FROM history WHERE workspace_path = ?1 ORDER BY executed_at DESC LIMIT 500)",
            [workspace],
        ).map_err(|error| error.to_string())?;
        Ok(())
    }

    pub async fn list(
        &self,
        workspace: &str,
        page: usize,
        limit: usize,
        status: Option<i64>,
        method: Option<&str>,
        search: Option<&str>,
        since: Option<&str>,
    ) -> Result<(Vec<Value>, usize), String> {
        let conn = self.connection.lock().await;
        let mut statement = conn.prepare(
            "SELECT id, workspace_path, request_name, method, url, status_code, duration_ms, size_bytes, response_headers, executed_at FROM history WHERE workspace_path = ?1 ORDER BY executed_at DESC LIMIT 500",
        ).map_err(|error| error.to_string())?;
        let rows = statement.query_map([workspace], |row| {
            let headers: String = row.get(8)?;
            Ok(json!({
                "id": row.get::<_, i64>(0)?,
                "workspacePath": row.get::<_, String>(1)?,
                "requestName": row.get::<_, String>(2)?,
                "method": row.get::<_, String>(3)?,
                "url": row.get::<_, String>(4)?,
                "statusCode": row.get::<_, i64>(5)?,
                "durationMs": row.get::<_, f64>(6)?,
                "sizeBytes": row.get::<_, i64>(7)?,
                "responseHeaders": serde_json::from_str::<Value>(&headers).unwrap_or_else(|_| json!({})),
                "executedAt": row.get::<_, String>(9)?,
            }))
        }).map_err(|error| error.to_string())?;
        let mut entries = Vec::new();
        for row in rows { entries.push(row.map_err(|error| error.to_string())?); }
        let since = since.and_then(|value| chrono::DateTime::parse_from_rfc3339(value).ok())
            .map(|value| value.with_timezone(&chrono::Utc));
        let filtered: Vec<Value> = entries.into_iter().filter(|entry| {
            status.filter(|status| *status > 0).is_none_or(|status| entry.get("statusCode").and_then(Value::as_i64) == Some(status))
                && method.is_none_or(|method| entry.get("method").and_then(Value::as_str).is_some_and(|value| value.eq_ignore_ascii_case(method)))
                && search.is_none_or(|search| {
                    let needle = search.to_lowercase();
                    entry.get("requestName").and_then(Value::as_str).unwrap_or("").to_lowercase().contains(&needle)
                        || entry.get("url").and_then(Value::as_str).unwrap_or("").to_lowercase().contains(&needle)
                })
                && since.as_ref().is_none_or(|since| {
                    entry.get("executedAt").and_then(Value::as_str)
                        .and_then(|value| chrono::DateTime::parse_from_rfc3339(value).ok())
                        .map(|value| value.with_timezone(&chrono::Utc))
                        .is_some_and(|executed_at| executed_at >= since.clone())
                })
        }).collect();
        let total = filtered.len();
        // Match the Go store's defaults. Each workspace is capped at 500 rows,
        // so a larger page cannot return an unbounded result set.
        let limit = if limit == 0 { 50 } else { limit };
        let page = if page == 0 { 1 } else { page };
        let start = page.saturating_sub(1).saturating_mul(limit).min(total);
        let end = start.saturating_add(limit).min(total);
        Ok((filtered[start..end].to_vec(), total))
    }

    pub async fn get(&self, id: i64) -> Result<Option<Value>, String> {
        let conn = self.connection.lock().await;
        conn.query_row(
            "SELECT id, workspace_path, request_name, method, url, status_code, duration_ms, size_bytes, response_body, response_headers, resolved_request, executed_at FROM history WHERE id = ?1",
            [id],
            |row| {
                let headers: String = row.get(9)?;
                let request: String = row.get(10)?;
                Ok(json!({
                    "id": row.get::<_, i64>(0)?,
                    "workspacePath": row.get::<_, String>(1)?,
                    "requestName": row.get::<_, String>(2)?,
                    "method": row.get::<_, String>(3)?,
                    "url": row.get::<_, String>(4)?,
                    "statusCode": row.get::<_, i64>(5)?,
                    "durationMs": row.get::<_, f64>(6)?,
                    "sizeBytes": row.get::<_, i64>(7)?,
                    "responseBody": row.get::<_, String>(8)?,
                    "responseHeaders": serde_json::from_str::<Value>(&headers).unwrap_or_else(|_| json!({})),
                    "resolvedRequest": serde_json::from_str::<Value>(&request).unwrap_or_else(|_| json!({})),
                    "executedAt": row.get::<_, String>(11)?,
                }))
            },
        ).optional().map_err(|error| error.to_string())
    }

    pub async fn delete(&self, id: i64) -> Result<(), String> {
        self.connection.lock().await.execute("DELETE FROM history WHERE id = ?1", [id]).map_err(|error| error.to_string())?;
        Ok(())
    }

    pub async fn clear(&self, workspace: &str) -> Result<(), String> {
        self.connection.lock().await.execute("DELETE FROM history WHERE workspace_path = ?1", [workspace]).map_err(|error| error.to_string())?;
        Ok(())
    }
}

fn mask(text: String, secrets: &[String]) -> String {
    pebblepost_core::security::mask_secrets(&text, secrets)
}

fn mask_value(value: &mut Value, secrets: &[String]) {
    match value {
        Value::String(text) => *text = mask(std::mem::take(text), secrets),
        Value::Array(values) => values.iter_mut().for_each(|value| mask_value(value, secrets)),
        Value::Object(values) => values.values_mut().for_each(|value| mask_value(value, secrets)),
        _ => {}
    }
}

fn database_error(error: rusqlite::Error) -> io::Error {
    io::Error::other(format!("history database error: {error}"))
}
