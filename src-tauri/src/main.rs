#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::fs;
use std::net::TcpListener;
use std::path::{Path, PathBuf};
use std::sync::Mutex;

use tauri::{Manager, State};
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;

#[derive(Clone, serde::Serialize)]
struct DesktopBootstrap {
  token: String,
  // Legacy compatibility base URL retained for legacy API integrations.
  api_v1_base_url: String,
  api_base_url: String,
  // Current desktop workbench still consumes the v1 websocket endpoint.
  ws_base_url: String
}

struct DesktopState {
  data_dir: PathBuf,
  port: u16,
  sidecar: Mutex<Option<CommandChild>>,
}

#[tauri::command]
fn desktop_bootstrap(state: State<'_, DesktopState>) -> Result<DesktopBootstrap, String> {
  let token = read_admin_token(&state.data_dir)?;
  let host = format!("http://127.0.0.1:{}", state.port);
  Ok(DesktopBootstrap {
    token,
    api_v1_base_url: format!("{}/api/v1", host),
    api_base_url: format!("{}/api", host),
    ws_base_url: format!("{}/api/v1", host),
  })
}

fn main() {
  tauri::Builder::default()
    .plugin(tauri_plugin_shell::init())
    .setup(|app| {
      let handle = app.handle().clone();
      let data_dir = resolve_data_dir(&handle)?;
      fs::create_dir_all(&data_dir).map_err(|e| format!("create data dir: {e}"))?;

      let port = reserve_free_port().map_err(|e| format!("reserve port: {e}"))?;
      let child = spawn_sidecar(&handle, &data_dir, port)?;

      app.manage(DesktopState {
        data_dir,
        port,
        sidecar: Mutex::new(Some(child)),
      });

      Ok(())
    })
    .invoke_handler(tauri::generate_handler![desktop_bootstrap])
    .run(tauri::generate_context!())
    .expect("error while running tauri application");
}

fn resolve_data_dir(handle: &tauri::AppHandle) -> Result<PathBuf, String> {
  let base = handle
    .path()
    .app_data_dir()
    .map_err(|e| format!("无法解析 app_data_dir: {e}"))?;
  Ok(base.join("ai-workflow"))
}

fn reserve_free_port() -> std::io::Result<u16> {
  let listener = TcpListener::bind("127.0.0.1:0")?;
  let port = listener.local_addr()?.port();
  drop(listener);
  Ok(port)
}

fn spawn_sidecar(handle: &tauri::AppHandle, data_dir: &Path, port: u16) -> Result<CommandChild, String> {
  let data_dir_env = data_dir.to_string_lossy().to_string();

  let mut cmd = handle
    .shell()
    .sidecar("ai-flow")
    .map_err(|e| format!("new sidecar: {e}"))?;
  cmd = cmd
    .args(["server", "--port", &port.to_string()])
    .env("AI_WORKFLOW_DATA_DIR", data_dir_env);

  let (mut rx, child) = cmd.spawn().map_err(|e| format!("spawn sidecar: {e}"))?;
  tauri::async_runtime::spawn(async move {
    while let Some(event) = rx.recv().await {
      match event {
        CommandEvent::Stdout(line) => {
          let text = String::from_utf8_lossy(&line);
          println!("[ai-flow stdout] {}", text.trim_end());
        }
        CommandEvent::Stderr(line) => {
          let text = String::from_utf8_lossy(&line);
          eprintln!("[ai-flow stderr] {}", text.trim_end());
        }
        CommandEvent::Error(err) => {
          eprintln!("[ai-flow error] {}", err);
        }
        CommandEvent::Terminated(payload) => {
          eprintln!("[ai-flow terminated] {:?}", payload);
          break;
        }
        _ => {}
      }
    }
  });

  Ok(child)
}

fn read_admin_token(data_dir: &Path) -> Result<String, String> {
  let secrets_toml = data_dir.join("secrets.toml");
  if secrets_toml.exists() {
    return read_admin_token_from_toml(&secrets_toml);
  }

  let secrets_yaml = data_dir.join("secrets.yaml");
  if secrets_yaml.exists() {
    return Err("暂不支持从 secrets.yaml 读取 token；请删除 secrets.yaml 以生成 secrets.toml".to_string());
  }

  Err("secrets.toml 尚未生成（后端可能仍在启动中），请稍后重试".to_string())
}

#[derive(serde::Deserialize)]
struct SecretsFile {
  tokens: Option<std::collections::BTreeMap<String, TokenEntry>>,
}

#[derive(serde::Deserialize)]
struct TokenEntry {
  token: Option<String>,
}

fn read_admin_token_from_toml(path: &Path) -> Result<String, String> {
  let content = fs::read_to_string(path).map_err(|e| format!("read secrets.toml: {e}"))?;
  let parsed: SecretsFile = toml::from_str(&content).map_err(|e| format!("parse secrets.toml: {e}"))?;
  let token = parsed
    .tokens
    .and_then(|map| map.get("admin").and_then(|entry| entry.token.clone()))
    .unwrap_or_default()
    .trim()
    .to_string();
  if token.is_empty() {
    return Err("secrets.toml 缺少 tokens.admin.token".to_string());
  }
  Ok(token)
}


