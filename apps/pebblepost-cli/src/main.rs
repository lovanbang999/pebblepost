use std::collections::HashMap;
use std::fmt::Write as FmtWrite;
use std::fs::{self, OpenOptions};
use std::io::Write;
use std::net::{IpAddr, SocketAddr};
use std::path::{Path, PathBuf};

use clap::{Parser, Subcommand};
use pebblepost_core::{init_workspace, scan_workspace};
use pebblepost_server::{docs, import_export};

#[derive(Debug, Parser)]
#[command(
    name = "pebblepost",
    version,
    about = "Local-first API workspace tools"
)]
struct Cli {
    #[command(subcommand)]
    command: Command,
}

#[derive(Debug, Subcommand)]
enum Command {
    /// Print build and runtime version information.
    Version,
    /// Create or inspect a PebblePost workspace.
    Workspace {
        #[command(subcommand)]
        command: WorkspaceCommand,
    },
    /// Execute requests in a workspace collection.
    Run {
        target: Option<PathBuf>,
        #[arg(short = 'e', long)]
        environment: Option<String>,
        #[arg(short = 'd', long)]
        data: Option<PathBuf>,
        #[arg(short = 'n', long)]
        iterations: Option<usize>,
        #[arg(long, default_value_t = 0)]
        delay: u64,
        #[arg(long)]
        timeout: Option<u64>,
        #[arg(long, default_value_t = 0)]
        retry: usize,
        #[arg(long)]
        bail: bool,
        #[arg(long)]
        ci: bool,
        #[arg(long)]
        dry_run: bool,
        #[arg(long)]
        trust: bool,
        #[arg(long)]
        secret_backend: Option<String>,
        #[arg(long = "var", action = clap::ArgAction::Append)]
        variables: Vec<String>,
        #[arg(long)]
        env_file: Option<PathBuf>,
        #[arg(long = "folder")]
        folder_filter: Option<String>,
        #[arg(long = "request")]
        request_filter: Option<String>,
        #[arg(long = "tag")]
        tag_filter: Option<String>,
        #[arg(long = "reporter", action = clap::ArgAction::Append)]
        reporters: Vec<String>,
        #[arg(long)]
        out: Option<PathBuf>,
    },
    /// Check or apply OpenAPI specification changes for a collection folder.
    Sync {
        target: Option<String>,
        second: Option<String>,
        third: Option<String>,
        #[arg(long)]
        check: bool,
        #[arg(long)]
        apply: bool,
        #[arg(long)]
        spec: Option<String>,
        #[arg(long)]
        force: bool,
        #[arg(long)]
        json: bool,
    },
    /// Serve saved request examples as mock HTTP responses.
    Mock {
        target: Option<PathBuf>,
        #[arg(short = 'p', long, default_value_t = 8080)]
        port: u16,
        #[arg(long, default_value = "127.0.0.1")]
        host: String,
        #[arg(long, default_value_t = 0)]
        delay: u64,
        #[arg(long, default_value_t = 0)]
        status: u16,
        #[arg(long, default_value_t = 0.0)]
        error_rate: f64,
    },
    /// Manage environment secrets stored in files, the OS environment, or the OS keychain.
    Secrets {
        #[command(subcommand)]
        command: SecretsCommand,
    },
    /// Import a supported collection file into a workspace.
    Import {
        source: PathBuf,
        #[arg(short, long)]
        out_dir: Option<String>,
        #[arg(short, long, default_value = "")]
        format: String,
        #[arg(long, default_value = ".")]
        workspace: PathBuf,
        #[arg(long)]
        json: bool,
    },
    /// Export a collection as a Postman collection or OpenAPI document.
    Export {
        #[arg(default_value = "collections")]
        path: String,
        #[arg(short, long, default_value = "postman")]
        format: String,
        #[arg(long)]
        name: Option<String>,
        #[arg(short, long)]
        out: Option<PathBuf>,
        #[arg(long, default_value = ".")]
        workspace: PathBuf,
    },
    /// Generate Markdown, HTML, or OpenAPI documentation for a workspace path.
    Docs {
        #[arg(default_value = ".")]
        path: String,
        #[arg(short, long, default_value = "md")]
        format: String,
        #[arg(long, default_value = ".")]
        workspace: PathBuf,
        #[arg(short, long)]
        out: Option<PathBuf>,
        #[arg(long)]
        stdout: bool,
    },
    /// Run the local PebblePost web API and serve the React application.
    Serve {
        #[arg(long, default_value = "127.0.0.1")]
        host: String,
        #[arg(long, default_value_t = 8080)]
        port: u16,
        #[arg(long)]
        token: Option<String>,
        #[arg(long, default_value = "./data")]
        data_dir: PathBuf,
        #[arg(long, default_value = "web/dist")]
        frontend_dir: PathBuf,
    },
}

#[derive(Debug, Subcommand)]
enum WorkspaceCommand {
    /// Create the standard .pebble and collections directory structure.
    Init {
        path: PathBuf,
        #[arg(long)]
        name: Option<String>,
    },
    /// Scan a workspace and print its tree and environment metadata as JSON.
    Scan {
        #[arg(default_value = ".")]
        path: PathBuf,
    },
}

#[derive(Debug, Subcommand)]
enum SecretsCommand {
    List {
        #[arg(short = 'e', long = "env", default_value = "dev")]
        environment: String,
        #[arg(short = 'b', long)]
        backend: Option<String>,
        #[arg(short = 'w', long, default_value = ".")]
        workspace: PathBuf,
    },
    Get {
        key: String,
        #[arg(short = 'e', long = "env", default_value = "dev")]
        environment: String,
        #[arg(short = 'b', long)]
        backend: Option<String>,
        #[arg(short = 'w', long, default_value = ".")]
        workspace: PathBuf,
    },
    Set {
        key: String,
        value: String,
        #[arg(short = 'e', long = "env", default_value = "dev")]
        environment: String,
        #[arg(short = 'b', long)]
        backend: Option<String>,
        #[arg(short = 'w', long, default_value = ".")]
        workspace: PathBuf,
    },
    #[command(alias = "rm")]
    Delete {
        key: String,
        #[arg(short = 'e', long = "env", default_value = "dev")]
        environment: String,
        #[arg(short = 'b', long)]
        backend: Option<String>,
        #[arg(short = 'w', long, default_value = ".")]
        workspace: PathBuf,
    },
    Migrate {
        #[arg(short = 'e', long = "env", default_value = "dev")]
        environment: String,
        #[arg(short = 'w', long, default_value = ".")]
        workspace: PathBuf,
        #[arg(long, default_value = "file")]
        from: String,
        #[arg(long, default_value = "keychain")]
        to: String,
        #[arg(short = 'y', long)]
        yes: bool,
    },
    Rollback {
        #[arg(short = 'e', long = "env", default_value = "dev")]
        environment: String,
        #[arg(short = 'w', long, default_value = ".")]
        workspace: PathBuf,
        #[arg(long)]
        backup: PathBuf,
        #[arg(short = 'b', long, default_value = "keychain")]
        backend: String,
    },
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    match Cli::parse().command {
        Command::Version => println!("PebblePost v{}", env!("CARGO_PKG_VERSION")),
        Command::Workspace {
            command: WorkspaceCommand::Init { path, name },
        } => {
            let workspace = init_workspace(&path, name.as_deref())?;
            println!("{}", serde_json::to_string_pretty(&workspace)?);
        }
        Command::Workspace {
            command: WorkspaceCommand::Scan { path },
        } => {
            let result = scan_workspace(&path)?;
            println!("{}", serde_json::to_string_pretty(&result)?);
        }
        Command::Run {
            target,
            environment,
            data,
            iterations,
            delay,
            timeout,
            retry,
            bail,
            ci,
            dry_run,
            trust,
            secret_backend,
            variables,
            env_file,
            folder_filter,
            request_filter,
            tag_filter,
            reporters,
            out,
        } => {
            match run_collection(
                target,
                environment,
                data,
                iterations,
                delay,
                timeout,
                retry,
                bail || ci,
                dry_run,
                trust,
                secret_backend,
                variables,
                env_file,
                folder_filter,
                request_filter,
                tag_filter,
                reporters,
                out,
            )
            .await
            {
                Ok(0) => {}
                Ok(code) => std::process::exit(code),
                Err(error) => {
                    eprintln!("Runner configuration error: {error}");
                    std::process::exit(2);
                }
            }
        }
        Command::Sync {
            target,
            second,
            third,
            check,
            apply,
            spec,
            force,
            json,
        } => match run_sync(target, second, third, check, apply, spec, force, json).await {
            Ok(0) => {}
            Ok(code) => std::process::exit(code),
            Err(error) => {
                eprintln!("OpenAPI sync error: {error}");
                std::process::exit(2);
            }
        },
        Command::Mock {
            target,
            port,
            host,
            delay,
            status,
            error_rate,
        } => {
            if let Err(error) = run_mock(target, port, host, delay, status, error_rate).await {
                eprintln!("Mock server error: {error}");
                std::process::exit(2);
            }
        }
        Command::Secrets { command } => {
            if let Err(error) = run_secrets(command) {
                eprintln!("Secret operation failed: {error}");
                std::process::exit(2);
            }
        }
        Command::Import {
            source,
            out_dir,
            format,
            workspace,
            json,
        } => {
            import_file(&source, out_dir.as_deref(), &format, &workspace, json)?;
        }
        Command::Export {
            path,
            format,
            name,
            out,
            workspace,
        } => {
            export_collection(&workspace, &path, &format, name.as_deref(), out.as_deref())?;
        }
        Command::Docs {
            path,
            format,
            workspace,
            out,
            stdout,
        } => {
            generate_docs(&workspace, &path, &format, out.as_deref(), stdout)?;
        }
        Command::Serve {
            host,
            port,
            token,
            data_dir,
            frontend_dir,
        } => {
            run_server(host, port, token, data_dir, frontend_dir).await?;
        }
    }
    Ok(())
}

#[allow(clippy::too_many_arguments)]
async fn run_collection(
    target: Option<PathBuf>,
    environment: Option<String>,
    data_file: Option<PathBuf>,
    requested_iterations: Option<usize>,
    delay_ms: u64,
    timeout_ms: Option<u64>,
    retry_count: usize,
    bail: bool,
    dry_run: bool,
    trust: bool,
    secret_backend: Option<String>,
    variable_flags: Vec<String>,
    env_file: Option<PathBuf>,
    folder_filter: Option<String>,
    request_filter: Option<String>,
    tag_filter: Option<String>,
    reporter_flags: Vec<String>,
    out: Option<PathBuf>,
) -> Result<i32, Box<dyn std::error::Error>> {
    if let Some(backend) = secret_backend.as_deref() {
        pebblepost_core::secrets::SecretBackend::parse(backend).map_err(std::io::Error::other)?;
    }
    let target = target.unwrap_or_else(|| {
        if Path::new("collections").is_dir() {
            PathBuf::from("collections")
        } else {
            PathBuf::from(".")
        }
    });
    let target = fs::canonicalize(&target)?;
    let request_paths = discover_requests(&target)?;
    if request_paths.is_empty() {
        return Err(format!(
            "no *.pebble.json request files found in {}",
            target.display()
        )
        .into());
    }
    let workspace = find_workspace_root(&target);
    let mut request_paths = request_paths
        .into_iter()
        .filter_map(|path| {
            match matches_run_filters(
                &path,
                &workspace,
                folder_filter.as_deref(),
                request_filter.as_deref(),
                tag_filter.as_deref(),
            ) {
                Ok(true) => Some(Ok(path)),
                Ok(false) => None,
                Err(error) => Some(Err(error)),
            }
        })
        .collect::<Result<Vec<_>, _>>()?;
    request_paths.sort_by(|left, right| left.to_string_lossy().cmp(&right.to_string_lossy()));
    if request_paths.is_empty() {
        return Err("no requests match the supplied filters".into());
    }

    let trusted = trust || collection_is_trusted(&target);
    if !trusted {
        eprintln!(
            "Notice: Scripts are disabled for this collection. Pass --trust or add a .pebbletrust file to enable them."
        );
    }
    if dry_run {
        println!(
            "Dry run: {} request(s) in {}",
            request_paths.len(),
            target.display()
        );
        for path in &request_paths {
            println!(
                "  {}",
                path.strip_prefix(&workspace).unwrap_or(path).display()
            );
        }
        return Ok(0);
    }

    let mut base_vars = HashMap::new();
    let env_file = env_file.or_else(|| {
        let candidate = workspace.join(".env");
        candidate.is_file().then_some(candidate)
    });
    if let Some(path) = env_file {
        base_vars.extend(read_env_file(&path)?);
    }
    for (key, value) in std::env::vars() {
        if let Some(name) = key.strip_prefix("PEBBLE_VAR_") {
            base_vars.insert(name.to_owned(), value);
        }
    }
    for item in variable_flags {
        let (key, value) = item
            .split_once('=')
            .ok_or_else(|| format!("invalid --var {item:?}: expected KEY=VALUE"))?;
        if key.is_empty() {
            return Err(format!("invalid --var {item:?}: key cannot be empty").into());
        }
        base_vars.insert(key.to_owned(), value.to_owned());
    }

    let data_rows = if let Some(path) = data_file.as_ref() {
        let content = fs::read_to_string(path)?;
        Some(
            pebblepost_server::parse_cli_data(path.to_string_lossy().as_ref(), &content)
                .map_err(std::io::Error::other)?,
        )
    } else {
        None
    };
    let iterations = match data_rows.as_ref().map(Vec::len).unwrap_or(0) {
        count if count > 0 => requested_iterations
            .filter(|requested| *requested > 0)
            .map(|requested| requested.min(count))
            .unwrap_or(count),
        _ => requested_iterations.unwrap_or(1).max(1),
    };
    let request_paths = request_paths
        .iter()
        .map(|path| path.to_string_lossy().into_owned())
        .collect::<Vec<_>>();
    let payload = serde_json::json!({
        "workspacePath": workspace.to_string_lossy(),
        "targetPath": target.to_string_lossy(),
        "requestPaths": request_paths,
        "environmentName": environment,
        "iterations": iterations,
        "delayMs": delay_ms,
        "timeoutMs": timeout_ms,
        "retryCount": retry_count,
        "maxExecutions": 100,
        "bail": bail,
        "trusted": trusted,
        "secretBackend": secret_backend,
        "baseVars": base_vars,
        "dataRows": data_rows,
    });
    let summary = pebblepost_server::run_local_runner(payload)
        .await
        .map_err(std::io::Error::other)?;
    let mut reporters = if reporter_flags.is_empty() {
        vec!["cli".to_owned()]
    } else {
        reporter_flags
    };
    let mut report_configs = reporters
        .drain(..)
        .map(|raw| {
            let (format, path) = raw
                .split_once(':')
                .map(|(format, path)| {
                    (
                        format.trim().to_ascii_lowercase(),
                        Some(PathBuf::from(path.trim())),
                    )
                })
                .unwrap_or_else(|| (raw.trim().to_ascii_lowercase(), None));
            if !matches!(format.as_str(), "cli" | "json" | "junit" | "html") {
                return Err(std::io::Error::other(format!(
                    "unknown reporter {format:?}; use cli, json, junit, or html"
                )));
            }
            Ok((format, path))
        })
        .collect::<Result<Vec<_>, _>>()?;
    if report_configs.is_empty() {
        report_configs.push(("cli".into(), None));
    }
    if let Some(out) = out
        && let Some((_, destination)) = report_configs.last_mut()
        && destination.is_none()
    {
        *destination = Some(out);
    }
    for (format, destination) in report_configs {
        let rendered = render_run_report(&format, &summary)?;
        if let Some(path) = destination {
            write_file(&path, rendered.as_bytes())?;
            eprintln!("Wrote {format} report to {}", path.display());
        } else {
            std::io::stdout().write_all(rendered.as_bytes())?;
        }
    }
    if summary
        .get("error")
        .and_then(serde_json::Value::as_str)
        .is_some()
    {
        return Ok(2);
    }
    if summary
        .get("success")
        .and_then(serde_json::Value::as_bool)
        .unwrap_or(false)
    {
        return Ok(0);
    }
    let results = summary
        .get("results")
        .and_then(serde_json::Value::as_array)
        .cloned()
        .unwrap_or_default();
    let has_assertion_failure = results.iter().any(|item| {
        item.pointer("/result/tests")
            .and_then(serde_json::Value::as_array)
            .is_some_and(|tests| {
                tests.iter().any(|test| {
                    test.get("passed").and_then(serde_json::Value::as_bool) == Some(false)
                })
            })
    });
    let has_network_failure = results.iter().any(|item| {
        item.pointer("/result/networkError")
            .and_then(serde_json::Value::as_bool)
            == Some(true)
    });
    if has_assertion_failure {
        Ok(1)
    } else if has_network_failure {
        Ok(3)
    } else {
        Ok(2)
    }
}

#[allow(clippy::too_many_arguments)]
async fn run_sync(
    target: Option<String>,
    second: Option<String>,
    third: Option<String>,
    check: bool,
    apply: bool,
    spec: Option<String>,
    force: bool,
    json_output: bool,
) -> Result<i32, Box<dyn std::error::Error>> {
    let target = target.unwrap_or_else(|| ".".to_owned());
    if target == "link" {
        let folder = second.ok_or_else(|| {
            std::io::Error::other("usage: pebblepost sync link <folder> <spec-path-or-url>")
        })?;
        let spec_location = third.ok_or_else(|| {
            std::io::Error::other("usage: pebblepost sync link <folder> <spec-path-or-url>")
        })?;
        if spec.is_some() || check || apply || force || json_output {
            return Err(
                "sync link does not accept --check, --apply, --spec, --force, or --json".into(),
            );
        }
        let result = pebblepost_server::run_local_api(
            "POST",
            "/api/sync/link",
            &[],
            Some(serde_json::json!({ "folderPath": folder, "specLocation": spec_location })),
        )
        .await
        .map_err(std::io::Error::other)?;
        println!(
            "Linked {} to {} (hash: {})",
            folder,
            spec_location,
            result
                .get("specHash")
                .and_then(serde_json::Value::as_str)
                .unwrap_or("unknown")
        );
        return Ok(0);
    }
    if second.is_some() || third.is_some() {
        return Err("unexpected positional arguments; use `sync link <folder> <spec>` to link a specification".into());
    }
    let folder = target;
    let checking = check || !apply;
    if checking || json_output {
        let mut query = vec![("folder".to_owned(), folder.clone())];
        if let Some(spec_location) = spec.as_ref() {
            query.push(("spec".to_owned(), spec_location.clone()));
        }
        let report = pebblepost_server::run_local_api("GET", "/api/sync/diff", &query, None)
            .await
            .map_err(std::io::Error::other)?;
        if json_output {
            println!("{}", serde_json::to_string_pretty(&report)?);
        } else {
            print_sync_report(&report);
        }
        if !checking {
            return Ok(0);
        }
        return Ok(
            if report.get("hasDrift").and_then(serde_json::Value::as_bool) == Some(true) {
                1
            } else {
                0
            },
        );
    }
    let payload = serde_json::json!({ "folderPath": folder, "specLocation": spec, "force": force });
    let result = pebblepost_server::run_local_api("POST", "/api/sync/apply", &[], Some(payload))
        .await
        .map_err(std::io::Error::other)?;
    if json_output {
        println!("{}", serde_json::to_string_pretty(&result)?);
    } else {
        println!(
            "Applied {} change(s); skipped {}.",
            result
                .get("appliedCount")
                .and_then(serde_json::Value::as_u64)
                .unwrap_or(0),
            result
                .get("skippedCount")
                .and_then(serde_json::Value::as_u64)
                .unwrap_or(0)
        );
        for field in ["createdFiles", "updatedFiles", "deletedFiles"] {
            if let Some(paths) = result.get(field).and_then(serde_json::Value::as_array) {
                for path in paths.iter().filter_map(serde_json::Value::as_str) {
                    println!("  {field}: {path}");
                }
            }
        }
    }
    Ok(0)
}

async fn run_mock(
    target: Option<PathBuf>,
    port: u16,
    host: String,
    delay_ms: u64,
    status_code: u16,
    error_rate: f64,
) -> Result<(), Box<dyn std::error::Error>> {
    if !(0.0..=1.0).contains(&error_rate) {
        return Err("--error-rate must be between 0.0 and 1.0".into());
    }
    let target = target.unwrap_or_else(|| PathBuf::from("."));
    let target = fs::canonicalize(&target)?;
    let workspace = find_workspace_root(&target);
    let payload = serde_json::json!({
        "workspacePath": workspace.to_string_lossy(),
        "path": target.to_string_lossy(),
        "host": host,
        "port": port,
        "globalDelayMs": delay_ms,
        "globalStatusCode": status_code,
        "globalErrorRate": error_rate,
    });
    let mock = match pebblepost_server::LocalMock::start(payload).await {
        Ok(mock) => mock,
        Err(error) if error.contains("No requests with saved response examples") => {
            println!(
                "No requests with saved examples were found in {}.",
                target.display()
            );
            println!("Save a response example on a request before starting the mock server.");
            return Ok(());
        }
        Err(error) => return Err(std::io::Error::other(error).into()),
    };
    let status = mock.status();
    if status
        .get("nonLoopback")
        .and_then(serde_json::Value::as_bool)
        == Some(true)
    {
        eprintln!(
            "Warning: mock server is bound to a non-loopback host and may be reachable from other devices."
        );
    }
    println!(
        "PebblePost mock server running at {}",
        status
            .get("url")
            .and_then(serde_json::Value::as_str)
            .unwrap_or("")
    );
    println!(
        "Target: {} ({} routes)",
        target.display(),
        status
            .get("routesCount")
            .and_then(serde_json::Value::as_u64)
            .unwrap_or(0)
    );
    println!("{:<8} {:<32} EXAMPLES", "METHOD", "PATH");
    if let Some(routes) = status.get("routes").and_then(serde_json::Value::as_array) {
        for route in routes {
            let names = route
                .get("exampleNames")
                .and_then(serde_json::Value::as_array)
                .map(|names| {
                    names
                        .iter()
                        .filter_map(serde_json::Value::as_str)
                        .collect::<Vec<_>>()
                        .join(", ")
                })
                .unwrap_or_default();
            println!(
                "{:<8} {:<32} {}",
                route
                    .get("method")
                    .and_then(serde_json::Value::as_str)
                    .unwrap_or("GET"),
                route
                    .get("pathPattern")
                    .and_then(serde_json::Value::as_str)
                    .unwrap_or("/"),
                names
            );
        }
    }
    println!("Press Ctrl+C to stop. Streaming incoming requests:");
    let mut cursor = 0u64;
    let ctrl_c = tokio::signal::ctrl_c();
    tokio::pin!(ctrl_c);
    loop {
        tokio::select! {
            result = &mut ctrl_c => { result?; break; }
            _ = tokio::time::sleep(std::time::Duration::from_millis(250)) => {
                for log in mock.logs_after(cursor) {
                    cursor = log.get("id").and_then(serde_json::Value::as_str)
                        .and_then(|id| id.parse::<u64>().ok()).unwrap_or(cursor);
                    println!("{} {:<7} {:<25} -> HTTP {} {} ({} ms)",
                        log.get("timestamp").and_then(serde_json::Value::as_str).unwrap_or(""),
                        log.get("method").and_then(serde_json::Value::as_str).unwrap_or(""),
                        log.get("path").and_then(serde_json::Value::as_str).unwrap_or(""),
                        log.get("statusCode").and_then(serde_json::Value::as_u64).unwrap_or(0),
                        if log.get("matched").and_then(serde_json::Value::as_bool) == Some(true) { "MATCHED" } else { "UNMATCHED" },
                        log.get("durationMs").and_then(serde_json::Value::as_u64).unwrap_or(0));
                }
            }
        }
    }
    mock.stop().await;
    println!("Mock server stopped.");
    Ok(())
}

fn run_secrets(command: SecretsCommand) -> Result<(), Box<dyn std::error::Error>> {
    use pebblepost_core::secrets::{self, SecretBackend};

    match command {
        SecretsCommand::List {
            environment,
            backend,
            workspace,
        } => {
            let root = fs::canonicalize(workspace)?;
            let backend = secrets::configured_backend(&root, &environment, backend.as_deref())
                .map_err(std::io::Error::other)?;
            let keys =
                secrets::list(&root, &environment, backend).map_err(std::io::Error::other)?;
            println!(
                "Secrets for environment {environment:?} (backend: {}):",
                backend.as_str()
            );
            if keys.is_empty() {
                println!("  (no secrets found)");
            } else {
                for key in keys {
                    println!("  • {key}");
                }
            }
        }
        SecretsCommand::Get {
            key,
            environment,
            backend,
            workspace,
        } => {
            let root = fs::canonicalize(workspace)?;
            let backend = secrets::configured_backend(&root, &environment, backend.as_deref())
                .map_err(std::io::Error::other)?;
            let value =
                secrets::get(&root, &environment, &key, backend).map_err(std::io::Error::other)?;
            println!("{value}");
        }
        SecretsCommand::Set {
            key,
            value,
            environment,
            backend,
            workspace,
        } => {
            let root = fs::canonicalize(workspace)?;
            let backend = secrets::configured_backend(&root, &environment, backend.as_deref())
                .map_err(std::io::Error::other)?;
            secrets::set(&root, &environment, &key, &value, backend)
                .map_err(std::io::Error::other)?;
            println!(
                "Saved secret {key:?} to environment {environment:?} (backend: {}).",
                backend.as_str()
            );
        }
        SecretsCommand::Delete {
            key,
            environment,
            backend,
            workspace,
        } => {
            let root = fs::canonicalize(workspace)?;
            let backend = secrets::configured_backend(&root, &environment, backend.as_deref())
                .map_err(std::io::Error::other)?;
            secrets::delete(&root, &environment, &key, backend).map_err(std::io::Error::other)?;
            println!("Deleted secret {key:?} from environment {environment:?}.");
        }
        SecretsCommand::Migrate {
            environment,
            workspace,
            from,
            to,
            yes,
        } => {
            let root = fs::canonicalize(workspace)?;
            let source = SecretBackend::parse(&from).map_err(std::io::Error::other)?;
            let destination = SecretBackend::parse(&to).map_err(std::io::Error::other)?;
            if source == destination {
                return Err(format!(
                    "source and destination backends are the same ({})",
                    source.as_str()
                )
                .into());
            }
            let keys = secrets::list(&root, &environment, source).map_err(std::io::Error::other)?;
            if keys.is_empty() {
                println!(
                    "No secrets found in {} backend for environment {environment:?} to migrate.",
                    source.as_str()
                );
                return Ok(());
            }
            println!("Migration plan:");
            println!("  Environment:  {environment}");
            println!("  Source:       {}", source.as_str());
            println!("  Destination:  {}", destination.as_str());
            println!("  Secrets ({}):  {}", keys.len(), keys.join(", "));
            if !yes {
                print!("Proceed with migration? [y/N]: ");
                std::io::stdout().flush()?;
                let mut response = String::new();
                std::io::stdin().read_line(&mut response)?;
                if !matches!(response.trim().to_ascii_lowercase().as_str(), "y" | "yes") {
                    println!("Migration aborted.");
                    return Ok(());
                }
            }
            let backup = if source == SecretBackend::File {
                secrets::backup_file(&root, &environment).map_err(std::io::Error::other)?
            } else {
                None
            };
            let mut transferred: Vec<String> = Vec::new();
            for key in &keys {
                let transfer = secrets::get(&root, &environment, key, source)
                    .and_then(|value| secrets::set(&root, &environment, key, &value, destination));
                if let Err(error) = transfer {
                    for copied in &transferred {
                        let _ = secrets::delete(&root, &environment, copied, destination);
                    }
                    if let Some(backup) = backup.as_ref() {
                        let _ = secrets::restore_backup(backup);
                    }
                    return Err(std::io::Error::other(format!("migration failed: {error}")).into());
                }
                transferred.push(key.clone());
            }
            for key in &keys {
                if let Err(error) = secrets::delete(&root, &environment, key, source) {
                    eprintln!("Warning: could not remove {key:?} from source backend: {error}");
                }
            }
            println!("Migration complete.");
            if let Some(backup) = backup {
                println!("Backup saved at: {}", backup.display());
            }
        }
        SecretsCommand::Rollback {
            environment,
            workspace,
            backup,
            backend,
        } => {
            let root = fs::canonicalize(workspace)?;
            let backend = SecretBackend::parse(&backend).map_err(std::io::Error::other)?;
            let restored = secrets::rollback(&root, &environment, &backup, backend)
                .map_err(std::io::Error::other)?;
            println!("Restored secret file from backup: {}", restored.display());
        }
    }
    Ok(())
}

fn print_sync_report(report: &serde_json::Value) {
    println!("OpenAPI synchronization report");
    println!(
        "  Folder: {}",
        report
            .get("folderPath")
            .and_then(serde_json::Value::as_str)
            .unwrap_or(".")
    );
    println!(
        "  Spec:   {}",
        report
            .get("specLocation")
            .and_then(serde_json::Value::as_str)
            .unwrap_or("")
    );
    if report.get("hasDrift").and_then(serde_json::Value::as_bool) != Some(true) {
        println!("Collection is in sync with the OpenAPI specification.");
        return;
    }
    if let Some(endpoints) = report
        .get("endpoints")
        .and_then(serde_json::Value::as_array)
    {
        for endpoint in endpoints {
            let kind = endpoint
                .get("diffType")
                .and_then(serde_json::Value::as_str)
                .unwrap_or("unchanged");
            if kind == "unchanged" {
                continue;
            }
            println!(
                "  {} {:<7} {}",
                match kind {
                    "added" => "+",
                    "removed" => "-",
                    _ => "~",
                },
                endpoint
                    .get("method")
                    .and_then(serde_json::Value::as_str)
                    .unwrap_or("GET"),
                endpoint
                    .get("path")
                    .and_then(serde_json::Value::as_str)
                    .unwrap_or("/")
            );
            if let Some(fields) = endpoint
                .get("fieldDiffs")
                .and_then(serde_json::Value::as_array)
            {
                for field in fields {
                    println!(
                        "      {}",
                        field
                            .get("description")
                            .and_then(serde_json::Value::as_str)
                            .unwrap_or("field changed")
                    );
                }
            }
            if let Some(details) = endpoint
                .pointer("/conflict/details")
                .and_then(serde_json::Value::as_str)
            {
                println!("      Conflict: {details}");
            }
        }
    }
}

fn discover_requests(target: &Path) -> std::io::Result<Vec<PathBuf>> {
    let metadata = fs::metadata(target)?;
    if !metadata.is_dir() {
        if target
            .file_name()
            .and_then(|name| name.to_str())
            .is_some_and(|name| name.ends_with(".pebble.json"))
        {
            return Ok(vec![target.to_path_buf()]);
        }
        return Err(std::io::Error::other(format!(
            "target file {} is not a .pebble.json request",
            target.display()
        )));
    }
    fn walk(directory: &Path, output: &mut Vec<PathBuf>) -> std::io::Result<()> {
        for entry in fs::read_dir(directory)? {
            let entry = entry?;
            let name = entry.file_name().to_string_lossy().into_owned();
            let kind = entry.file_type()?;
            if kind.is_dir() {
                if !name.starts_with('.') && !matches!(name.as_str(), "node_modules" | "dist") {
                    walk(&entry.path(), output)?;
                }
            } else if kind.is_file() && name.ends_with(".pebble.json") {
                output.push(entry.path());
            }
        }
        Ok(())
    }
    let mut files = Vec::new();
    walk(target, &mut files)?;
    files.sort_by(|left, right| left.to_string_lossy().cmp(&right.to_string_lossy()));
    Ok(files)
}

fn find_workspace_root(target: &Path) -> PathBuf {
    let mut current = if target.is_dir() {
        target.to_path_buf()
    } else {
        target.parent().unwrap_or(target).to_path_buf()
    };
    for _ in 0..6 {
        if current.join(".pebble").is_dir() {
            return current;
        }
        let Some(parent) = current.parent() else {
            break;
        };
        if parent == current {
            break;
        }
        current = parent.to_path_buf();
    }
    if target.is_dir() {
        target.to_path_buf()
    } else {
        target.parent().unwrap_or(target).to_path_buf()
    }
}

fn collection_is_trusted(target: &Path) -> bool {
    if ["PEBBLEPOST_TRUST", "PEBBLEPOST_TRUST_SCRIPTS"]
        .iter()
        .any(|key| std::env::var(key).ok().as_deref() == Some("1"))
    {
        return true;
    }
    let start = if target.is_dir() {
        target
    } else {
        target.parent().unwrap_or(target)
    };
    start
        .ancestors()
        .any(|directory| directory.join(".pebbletrust").is_file())
        || Path::new(".pebbletrust").is_file()
}

fn matches_run_filters(
    path: &Path,
    workspace: &Path,
    folder: Option<&str>,
    request: Option<&str>,
    tag: Option<&str>,
) -> std::io::Result<bool> {
    if folder.is_none() && request.is_none() && tag.is_none() {
        return Ok(true);
    }
    let request_value: serde_json::Value =
        serde_json::from_slice(&fs::read(path)?).map_err(std::io::Error::other)?;
    let relative = path
        .strip_prefix(workspace)
        .unwrap_or(path)
        .to_string_lossy()
        .replace('\\', "/");
    let name = request_value
        .get("name")
        .and_then(serde_json::Value::as_str)
        .unwrap_or("");
    if let Some(pattern) = folder {
        let folder_path = Path::new(&relative).parent().unwrap_or(Path::new(""));
        let folder_text = folder_path.to_string_lossy().replace('\\', "/");
        let base = folder_path
            .file_name()
            .and_then(|part| part.to_str())
            .unwrap_or("");
        if !glob_matches(pattern, &folder_text) && !glob_matches(pattern, base) {
            return Ok(false);
        }
    }
    if let Some(pattern) = request {
        let filename = path
            .file_name()
            .and_then(|part| part.to_str())
            .unwrap_or("");
        if !glob_matches(pattern, name)
            && !glob_matches(pattern, &relative)
            && !glob_matches(pattern, filename)
            && !name
                .to_ascii_lowercase()
                .contains(&pattern.to_ascii_lowercase())
            && !relative
                .to_ascii_lowercase()
                .contains(&pattern.to_ascii_lowercase())
        {
            return Ok(false);
        }
    }
    if let Some(tag) = tag {
        let matched = request_value
            .get("tags")
            .and_then(serde_json::Value::as_array)
            .is_some_and(|tags| {
                tags.iter()
                    .filter_map(serde_json::Value::as_str)
                    .any(|candidate| candidate.eq_ignore_ascii_case(tag))
            });
        if !matched {
            return Ok(false);
        }
    }
    Ok(true)
}

fn glob_matches(pattern: &str, value: &str) -> bool {
    #[derive(Clone)]
    enum Token {
        Literal(char),
        Any,
        Star,
        Class {
            negated: bool,
            ranges: Vec<(char, char)>,
        },
    }

    let chars = pattern.chars().collect::<Vec<_>>();
    let mut tokens = Vec::new();
    let mut index = 0;
    while index < chars.len() {
        match chars[index] {
            '*' => {
                if !matches!(tokens.last(), Some(Token::Star)) {
                    tokens.push(Token::Star);
                }
                index += 1;
            }
            '?' => {
                tokens.push(Token::Any);
                index += 1;
            }
            '\\' => {
                index += 1;
                let Some(literal) = chars.get(index).copied() else {
                    return false;
                };
                tokens.push(Token::Literal(literal));
                index += 1;
            }
            '[' => {
                index += 1;
                let negated = chars.get(index) == Some(&'^');
                if negated {
                    index += 1;
                }
                let mut items = Vec::new();
                while index < chars.len() && chars[index] != ']' {
                    if chars[index] == '\\' {
                        index += 1;
                        let Some(literal) = chars.get(index).copied() else {
                            return false;
                        };
                        items.push(literal);
                    } else {
                        items.push(chars[index]);
                    }
                    index += 1;
                }
                if index == chars.len() || items.is_empty() {
                    return false;
                }
                index += 1;
                let mut ranges = Vec::new();
                let mut item = 0;
                while item < items.len() {
                    if item + 2 < items.len() && items[item + 1] == '-' {
                        if items[item] > items[item + 2] {
                            return false;
                        }
                        ranges.push((items[item], items[item + 2]));
                        item += 3;
                    } else {
                        ranges.push((items[item], items[item]));
                        item += 1;
                    }
                }
                tokens.push(Token::Class { negated, ranges });
            }
            literal => {
                tokens.push(Token::Literal(literal));
                index += 1;
            }
        }
    }

    fn matches_from(
        tokens: &[Token],
        value: &[char],
        pattern_index: usize,
        value_index: usize,
        memo: &mut [Vec<Option<bool>>],
    ) -> bool {
        if let Some(cached) = memo[pattern_index][value_index] {
            return cached;
        }
        let matched = if pattern_index == tokens.len() {
            value_index == value.len()
        } else {
            match &tokens[pattern_index] {
                Token::Literal(expected) => {
                    value.get(value_index) == Some(expected)
                        && matches_from(tokens, value, pattern_index + 1, value_index + 1, memo)
                }
                Token::Any => {
                    value.get(value_index).is_some_and(|actual| *actual != '/')
                        && matches_from(tokens, value, pattern_index + 1, value_index + 1, memo)
                }
                Token::Class { negated, ranges } => {
                    value.get(value_index).is_some_and(|actual| {
                        *actual != '/'
                            && (ranges
                                .iter()
                                .any(|(start, end)| start <= actual && actual <= end)
                                != *negated)
                    }) && matches_from(tokens, value, pattern_index + 1, value_index + 1, memo)
                }
                Token::Star => {
                    matches_from(tokens, value, pattern_index + 1, value_index, memo)
                        || value.get(value_index).is_some_and(|actual| *actual != '/')
                            && matches_from(tokens, value, pattern_index, value_index + 1, memo)
                }
            }
        };
        memo[pattern_index][value_index] = Some(matched);
        matched
    }

    let value = value.chars().collect::<Vec<_>>();
    let mut memo = vec![vec![None; value.len() + 1]; tokens.len() + 1];
    matches_from(&tokens, &value, 0, 0, &mut memo)
}

fn read_env_file(path: &Path) -> std::io::Result<HashMap<String, String>> {
    let contents = fs::read_to_string(path)?;
    let mut variables = HashMap::new();
    for raw in contents.lines() {
        let mut line = raw.trim();
        if line.is_empty() || line.starts_with('#') {
            continue;
        }
        if let Some(rest) = line.strip_prefix("export ") {
            line = rest.trim();
        }
        let Some((key, raw_value)) = line.split_once('=') else {
            continue;
        };
        let key = key.trim();
        if key.is_empty() {
            continue;
        }
        let mut value = raw_value.trim().to_owned();
        if value.len() >= 2
            && ((value.starts_with('"') && value.ends_with('"'))
                || (value.starts_with('\'') && value.ends_with('\'')))
        {
            let quote = value.remove(0);
            value.pop();
            if quote == '"' {
                value = value
                    .replace("\\n", "\n")
                    .replace("\\t", "\t")
                    .replace("\\\"", "\"")
                    .replace("\\\\", "\\");
            }
        } else if let Some(comment) = value.find(" #") {
            value.truncate(comment);
            value = value.trim_end().to_owned();
        }
        variables.insert(key.to_owned(), resolve_os_environment_refs(&value));
    }
    Ok(variables)
}

fn resolve_os_environment_refs(value: &str) -> String {
    let mut output = String::new();
    let mut cursor = 0;
    while let Some(relative_start) = value[cursor..].find("${OS_ENV:") {
        let start = cursor + relative_start;
        output.push_str(&value[cursor..start]);
        let variable_start = start + "${OS_ENV:".len();
        let Some(relative_end) = value[variable_start..].find('}') else {
            output.push_str(&value[start..]);
            return output;
        };
        let end = variable_start + relative_end;
        let name = value[variable_start..end].trim();
        output.push_str(&std::env::var(name).unwrap_or_else(|_| value[start..end + 1].to_owned()));
        cursor = end + 1;
    }
    output.push_str(&value[cursor..]);
    output
}

fn render_run_report(
    format: &str,
    summary: &serde_json::Value,
) -> Result<String, Box<dyn std::error::Error>> {
    if format == "json" {
        return Ok(format!("{}\n", serde_json::to_string_pretty(summary)?));
    }
    let results = summary
        .get("results")
        .and_then(serde_json::Value::as_array)
        .cloned()
        .unwrap_or_default();
    if format == "junit" {
        let total_tests = summary
            .get("totalTests")
            .and_then(serde_json::Value::as_u64)
            .unwrap_or(0);
        let failures = summary
            .get("failedTests")
            .and_then(serde_json::Value::as_u64)
            .unwrap_or(0);
        let errors = results
            .iter()
            .filter(|item| {
                item.pointer("/result/error")
                    .and_then(serde_json::Value::as_str)
                    .is_some()
            })
            .count();
        let mut report = String::new();
        writeln!(&mut report, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>")?;
        writeln!(
            &mut report,
            "<testsuite name=\"PebblePost\" tests=\"{total_tests}\" failures=\"{failures}\" errors=\"{errors}\">"
        )?;
        for item in results {
            let name = item
                .get("relPath")
                .and_then(serde_json::Value::as_str)
                .unwrap_or("request");
            let seconds = item
                .get("duration")
                .and_then(serde_json::Value::as_u64)
                .unwrap_or(0) as f64
                / 1_000_000_000.0;
            writeln!(
                &mut report,
                "  <testcase name=\"{}\" time=\"{seconds:.3}\">",
                xml_escape(name)
            )?;
            if item.get("passed").and_then(serde_json::Value::as_bool) == Some(false) {
                let reason = item
                    .get("error")
                    .and_then(serde_json::Value::as_str)
                    .unwrap_or("request failed");
                writeln!(
                    &mut report,
                    "    <failure message=\"{}\" />",
                    xml_escape(reason)
                )?;
            }
            writeln!(&mut report, "  </testcase>")?;
        }
        writeln!(&mut report, "</testsuite>")?;
        return Ok(report);
    }
    if format == "html" {
        let mut report = String::from(
            "<!doctype html><html><head><meta charset=\"utf-8\"><title>PebblePost Run</title></head><body><h1>PebblePost Run Report</h1><table><thead><tr><th>Result</th><th>Request</th><th>Status</th><th>Duration</th></tr></thead><tbody>",
        );
        for item in results {
            let passed = item
                .get("passed")
                .and_then(serde_json::Value::as_bool)
                .unwrap_or(false);
            let name = item
                .get("relPath")
                .and_then(serde_json::Value::as_str)
                .unwrap_or("request");
            let status = item
                .pointer("/result/statusCode")
                .and_then(serde_json::Value::as_u64)
                .unwrap_or(0);
            let duration = item
                .get("duration")
                .and_then(serde_json::Value::as_u64)
                .unwrap_or(0) as f64
                / 1_000_000.0;
            writeln!(
                &mut report,
                "<tr><td>{}</td><td>{}</td><td>{status}</td><td>{duration:.1} ms</td></tr>",
                if passed { "PASS" } else { "FAIL" },
                html_escape(name)
            )?;
        }
        writeln!(
            &mut report,
            "</tbody></table><p>{} passed, {} failed, {} test assertions</p></body></html>",
            summary
                .get("passedRequests")
                .and_then(serde_json::Value::as_u64)
                .unwrap_or(0),
            summary
                .get("failedRequests")
                .and_then(serde_json::Value::as_u64)
                .unwrap_or(0),
            summary
                .get("totalTests")
                .and_then(serde_json::Value::as_u64)
                .unwrap_or(0)
        )?;
        return Ok(report);
    }
    let mut report = String::new();
    for item in &results {
        let passed = item
            .get("passed")
            .and_then(serde_json::Value::as_bool)
            .unwrap_or(false);
        let name = item
            .get("relPath")
            .and_then(serde_json::Value::as_str)
            .unwrap_or("request");
        let method = item
            .pointer("/request/method")
            .and_then(serde_json::Value::as_str)
            .unwrap_or("GET");
        let status = item
            .pointer("/result/statusCode")
            .and_then(serde_json::Value::as_u64)
            .unwrap_or(0);
        let duration = item
            .get("duration")
            .and_then(serde_json::Value::as_u64)
            .unwrap_or(0) as f64
            / 1_000_000.0;
        let retries = item
            .pointer("/result/retryCount")
            .and_then(serde_json::Value::as_u64)
            .unwrap_or(0);
        let retry_suffix = if retries > 0 {
            format!(" [retried {retries}x]")
        } else {
            String::new()
        };
        writeln!(
            &mut report,
            "{} {:<7} {} -> HTTP {} ({duration:.1} ms){retry_suffix}",
            if passed { "PASS" } else { "FAIL" },
            method,
            name,
            status
        )?;
    }
    writeln!(
        &mut report,
        "\n{} passed, {} failed requests; {} tests ({} failed) in {:.1} ms",
        summary
            .get("passedRequests")
            .and_then(serde_json::Value::as_u64)
            .unwrap_or(0),
        summary
            .get("failedRequests")
            .and_then(serde_json::Value::as_u64)
            .unwrap_or(0),
        summary
            .get("totalTests")
            .and_then(serde_json::Value::as_u64)
            .unwrap_or(0),
        summary
            .get("failedTests")
            .and_then(serde_json::Value::as_u64)
            .unwrap_or(0),
        summary
            .get("totalDurationMs")
            .and_then(serde_json::Value::as_f64)
            .unwrap_or(0.0)
    )?;
    Ok(report)
}

fn xml_escape(value: &str) -> String {
    value
        .replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
        .replace('"', "&quot;")
        .replace('\'', "&apos;")
}

fn html_escape(value: &str) -> String {
    xml_escape(value)
}

fn import_file(
    source: &Path,
    out_dir: Option<&str>,
    format: &str,
    workspace: &Path,
    json_output: bool,
) -> Result<(), Box<dyn std::error::Error>> {
    const MAX_IMPORT_BYTES: u64 = 50 * 1024 * 1024;
    if fs::metadata(source)?.len() > MAX_IMPORT_BYTES {
        return Err("import file exceeds the 50 MB limit".into());
    }
    let imported = if source.is_dir() {
        import_export::parse_bruno_collection(source).map_err(std::io::Error::other)?
    } else {
        let content = fs::read_to_string(source)?;
        let detected_format = if format.is_empty()
            && source
                .extension()
                .and_then(|extension| extension.to_str())
                .is_some_and(|extension| extension.eq_ignore_ascii_case("bru"))
        {
            "bruno"
        } else {
            format
        };
        import_export::parse_import(detected_format, &content).map_err(std::io::Error::other)?
    };
    import_export::save_import(workspace, out_dir, &imported)?;
    let destination = out_dir
        .map(str::to_owned)
        .unwrap_or_else(|| format!("collections/{}", slug(&imported.collection_name)));
    let report = serde_json::json!({
        "collectionName": &imported.collection_name,
        "sourceFormat": &imported.format,
        "totalRequests": imported.requests.len(),
        "totalFolders": imported.folders.len(),
        "totalVariables": imported.variables.len(),
        "variables": &imported.variables,
        "skipped": &imported.skipped,
        "warnings": &imported.warnings,
        "outDir": destination
    });
    if json_output {
        println!("{}", serde_json::to_string_pretty(&report)?);
    } else {
        println!(
            "Imported {} request(s) from {} into {}",
            report["totalRequests"].as_u64().unwrap_or(0),
            source.display(),
            report["outDir"].as_str().unwrap_or("collections")
        );
        if let Some(warnings) = report["warnings"].as_array() {
            for warning in warnings.iter().filter_map(serde_json::Value::as_str) {
                eprintln!("Warning: {warning}");
            }
        }
    }
    Ok(())
}

fn slug(value: &str) -> String {
    let mut output = String::new();
    let mut separator = false;
    for character in value.chars() {
        if character.is_ascii_alphanumeric() {
            if separator && !output.is_empty() {
                output.push('-');
            }
            output.push(character.to_ascii_lowercase());
            separator = false;
        } else {
            separator = true;
        }
    }
    if output.is_empty() {
        "collection".to_owned()
    } else {
        output
    }
}

fn export_collection(
    workspace: &Path,
    collection_path: &str,
    format: &str,
    name: Option<&str>,
    output: Option<&Path>,
) -> Result<(), Box<dyn std::error::Error>> {
    let export = import_export::export_collection(workspace, collection_path, format, name)?;
    let content = export["content"].as_str().unwrap_or_default();
    let destination = output
        .map(Path::to_path_buf)
        .unwrap_or_else(|| PathBuf::from(export["filename"].as_str().unwrap_or("export.json")));
    write_file(&destination, content.as_bytes())?;
    println!(
        "Exported {} request(s) to {}",
        export["count"].as_u64().unwrap_or(0),
        destination.display()
    );
    Ok(())
}

fn generate_docs(
    workspace: &Path,
    target: &str,
    format: &str,
    output: Option<&Path>,
    stdout: bool,
) -> Result<(), Box<dyn std::error::Error>> {
    let generated = docs::generate(workspace, Some(target), format)?;
    let content = generated["content"].as_str().unwrap_or_default();
    if stdout {
        std::io::stdout().write_all(content.as_bytes())?;
        return Ok(());
    }
    let filename = generated["filename"].as_str().unwrap_or("index.md");
    let destination = match output {
        Some(path) if path.extension().is_some() => path.to_path_buf(),
        Some(directory) => directory.join(filename),
        None => PathBuf::from("docs").join(filename),
    };
    write_file(&destination, content.as_bytes())?;
    println!(
        "Generated {} documentation at {}",
        format,
        destination.display()
    );
    Ok(())
}

fn write_file(path: &Path, contents: &[u8]) -> std::io::Result<()> {
    if let Some(parent) = path
        .parent()
        .filter(|parent| !parent.as_os_str().is_empty())
    {
        fs::create_dir_all(parent)?;
    }
    fs::write(path, contents)
}

async fn run_server(
    host: String,
    port: u16,
    token: Option<String>,
    data_dir: PathBuf,
    frontend_dir: PathBuf,
) -> Result<(), Box<dyn std::error::Error>> {
    let host: IpAddr = host.parse()?;
    fs::create_dir_all(&data_dir)?;
    let token = token
        .or_else(|| std::env::var("PEBBLEPOST_TOKEN").ok())
        .unwrap_or_default();
    if !host.is_loopback() && token.is_empty() {
        return Err("a non-loopback server requires --token or PEBBLEPOST_TOKEN".into());
    }
    if !token.is_empty() {
        write_token(&data_dir.join(".token"), &token)?;
        eprintln!("PebblePost API token saved under {}", data_dir.display());
    }
    let listener = tokio::net::TcpListener::bind(SocketAddr::new(host, port)).await?;
    eprintln!("PebblePost listening on {}", listener.local_addr()?);
    axum::serve(
        listener,
        pebblepost_server::router(token, frontend_dir, data_dir)?,
    )
    .await?;
    Ok(())
}

fn write_token(path: &std::path::Path, token: &str) -> std::io::Result<()> {
    let mut options = OpenOptions::new();
    options.create(true).truncate(true).write(true);
    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt;
        options.mode(0o600);
    }
    let mut file = options.open(path)?;
    file.write_all(token.as_bytes())?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(path, fs::Permissions::from_mode(0o600))?;
    }
    Ok(())
}
