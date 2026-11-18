use std::env;
use std::fs::{self, OpenOptions};
use std::io::Write;
use std::path::PathBuf;

use clap::Parser;
use pebblepost_server::router;

#[derive(Parser)]
#[command(name = "pebblepost-server", about = "PebblePost API server")]
struct Args {
    #[arg(long)]
    version: bool,
    #[arg(long)]
    host: Option<String>,
    #[arg(long)]
    port: Option<u16>,
    #[arg(long)]
    token: Option<String>,
    #[arg(long = "data-dir")]
    data_dir: Option<PathBuf>,
}

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args = Args::parse();
    if args.version {
        println!("PebblePost Server v{} (commit: {}, date: {}, built by: {})",
            env!("CARGO_PKG_VERSION"), option_env!("PEBBLEPOST_COMMIT").unwrap_or("none"),
            option_env!("PEBBLEPOST_BUILD_DATE").unwrap_or("unknown"), option_env!("PEBBLEPOST_BUILT_BY").unwrap_or("source"));
        return Ok(());
    }
    let host = args.host.unwrap_or_else(|| env_or("PEBBLEPOST_HOST", &env_or("PEBBLE_HOST", "127.0.0.1")));
    let port = match args.port {
        Some(port) => port,
        None => env_or("PORT", "8080").parse::<u16>()?,
    };
    let data_dir = args.data_dir.unwrap_or_else(|| PathBuf::from(env_or("PEBBLEPOST_DATA_DIR", &env_or("DATA_DIR", "./data"))));
    fs::create_dir_all(&data_dir)?;

    let mut token = args.token.unwrap_or_else(|| env::var("PEBBLEPOST_TOKEN").or_else(|_| env::var("PEBBLE_TOKEN")).unwrap_or_default());
    let is_loopback = matches!(host.as_str(), "127.0.0.1" | "::1" | "localhost");
    if !is_loopback && token.is_empty() {
        return Err("--token or PEBBLEPOST_TOKEN (or PEBBLE_TOKEN) is required when --host is not 127.0.0.1".into());
    }
    if token.is_empty() {
        let mut bytes = [0_u8; 16];
        getrandom::fill(&mut bytes)?;
        token = bytes.iter().map(|byte| format!("{byte:02x}")).collect();
    }
    write_token(&data_dir.join(".token"), &token)?;
    let token_chars = token.chars().collect::<Vec<_>>();
    let masked_token = if token_chars.len() > 8 {
        format!("{}...{}", token_chars.iter().copied().take(4).collect::<String>(), token_chars.iter().copied().skip(token_chars.len() - 4).collect::<String>())
    } else {
        token.clone()
    };
    eprintln!("PebblePost API token: {masked_token} (saved to {})", data_dir.join(".token").display());

    let frontend_dir = PathBuf::from(env_or("PEBBLEPOST_FRONTEND_DIR", "web/dist"));
    let app = router(token, frontend_dir, data_dir)?;
    let listener = tokio::net::TcpListener::bind((host.as_str(), port)).await?;
    eprintln!("PebblePost server listening on {}", listener.local_addr()?);
    axum::serve(listener, app).await?;
    Ok(())
}

fn env_or(key: &str, fallback: &str) -> String {
    env::var(key).unwrap_or_else(|_| fallback.to_owned())
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
