#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::fs;
use std::net::{IpAddr, Ipv4Addr, SocketAddr, TcpListener as StdTcpListener};
use std::path::PathBuf;

use serde::Serialize;
use tauri::{AppHandle, Manager, State, WebviewWindow};
use tauri_plugin_dialog::DialogExt;

#[derive(Clone, Serialize)]
#[serde(rename_all = "camelCase")]
struct ApiConfig {
    base_url: String,
    token: String,
}

#[tauri::command]
fn api_config(state: State<'_, ApiConfig>) -> ApiConfig {
    state.inner().clone()
}

#[tauri::command]
async fn select_directory(app: AppHandle) -> Option<String> {
    let (sender, receiver) = tokio::sync::oneshot::channel();
    app.dialog().file().pick_folder(move |selected| {
        let path = selected
            .and_then(|file| file.into_path().ok())
            .map(|path| path.to_string_lossy().into_owned());
        let _ = sender.send(path);
    });
    receiver.await.ok().flatten()
}

#[tauri::command]
fn window_minimise(window: WebviewWindow) -> Result<(), String> {
    window.minimize().map_err(|error| error.to_string())
}

#[tauri::command]
fn window_toggle_maximise(window: WebviewWindow) -> Result<(), String> {
    if window.is_maximized().map_err(|error| error.to_string())? {
        window.unmaximize().map_err(|error| error.to_string())
    } else {
        window.maximize().map_err(|error| error.to_string())
    }
}

#[tauri::command]
fn window_close(window: WebviewWindow) -> Result<(), String> {
    window.close().map_err(|error| error.to_string())
}

#[tauri::command]
fn restart_app(app: AppHandle) {
    app.restart();
}

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_dialog::init())
        .setup(|app| {
            let frontend_dir = PathBuf::from("web/dist");
            let listener =
                StdTcpListener::bind(SocketAddr::new(IpAddr::V4(Ipv4Addr::LOCALHOST), 0))?;
            listener.set_nonblocking(true)?;
            let address = listener.local_addr()?;
            let data_dir = app.path().app_data_dir()?;
            fs::create_dir_all(&data_dir)?;
            let api = pebblepost_server::router(String::new(), frontend_dir, data_dir)?;
            tauri::async_runtime::spawn(async move {
                match tokio::net::TcpListener::from_std(listener) {
                    Ok(listener) => {
                        if let Err(error) = axum::serve(listener, api).await {
                            eprintln!("PebblePost local API stopped: {error}");
                        }
                    }
                    Err(error) => eprintln!("failed to start PebblePost local API: {error}"),
                }
            });
            app.manage(ApiConfig {
                base_url: format!("http://{address}"),
                token: String::new(),
            });
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            api_config,
            select_directory,
            window_minimise,
            window_toggle_maximise,
            window_close,
            restart_app
        ])
        .run(tauri::generate_context!())
        .expect("failed to run PebblePost Studio");
}
