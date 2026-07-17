//! Oikos desktop — the Tauri shell.
//!
//! This module holds only glue: it turns the peer list and agent client from
//! oikos-core into Tauri commands, and it fans agent status/metrics out to the
//! webview as events. The webview never opens a socket itself (CLAUDE.md §3).
//! All the logic worth testing lives in oikos-core, which builds without a GUI
//! toolkit; this crate is the thin part verified on a real desktop.

use std::collections::HashMap;
use std::path::PathBuf;
use std::sync::Mutex;
use std::time::Duration;

use futures_util::StreamExt;
use oikos_core::{AgentClient, Info, Metrics, Peer, PeersFile};
use serde::Serialize;
use tauri::async_runtime::JoinHandle;
use tauri::{AppHandle, Emitter, Manager, State};

/// Event names the webview subscribes to. Kept in one place so the TS bridge
/// and this shell can't drift.
const EVT_PEERS: &str = "peers:changed";
const EVT_STATUS: &str = "peer:status";
const EVT_METRICS: &str = "peer:metrics";

/// Reconnect / re-probe delay after a peer drops.
const RETRY: Duration = Duration::from_secs(3);

struct AppState {
    path: PathBuf,
    peers: Mutex<PeersFile>,
    tasks: Mutex<HashMap<String, JoinHandle<()>>>,
}

#[derive(Clone, Serialize)]
struct StatusEvent {
    address: String,
    online: bool,
    info: Option<Info>,
}

#[derive(Clone, Serialize)]
struct MetricsEvent {
    address: String,
    metrics: Metrics,
}

#[tauri::command]
fn list_peers(state: State<AppState>) -> Vec<Peer> {
    state.peers.lock().unwrap().peers.clone()
}

#[tauri::command]
fn add_peer(
    app: AppHandle,
    state: State<AppState>,
    name: String,
    address: String,
    port: u16,
) -> Result<(), String> {
    {
        let mut pf = state.peers.lock().unwrap();
        pf.add(&name, &address, port).map_err(stringify)?;
        pf.save(&state.path).map_err(stringify)?;
    }
    supervise_peer(&app, &state.tasks, Peer { name, address, port });
    emit_peers(&app, &state.peers);
    Ok(())
}

#[tauri::command]
fn remove_peer(app: AppHandle, state: State<AppState>, address: String) -> Result<(), String> {
    {
        let mut pf = state.peers.lock().unwrap();
        pf.remove(&address).map_err(stringify)?;
        pf.save(&state.path).map_err(stringify)?;
    }
    if let Some(handle) = state.tasks.lock().unwrap().remove(&address) {
        handle.abort();
    }
    emit_peers(&app, &state.peers);
    Ok(())
}

#[tauri::command]
fn rename_peer(
    app: AppHandle,
    state: State<AppState>,
    address: String,
    name: String,
) -> Result<(), String> {
    {
        let mut pf = state.peers.lock().unwrap();
        pf.rename(&address, &name).map_err(stringify)?;
        pf.save(&state.path).map_err(stringify)?;
    }
    emit_peers(&app, &state.peers);
    Ok(())
}

fn stringify<E: std::fmt::Display>(e: E) -> String {
    e.to_string()
}

fn emit_peers(app: &AppHandle, peers: &Mutex<PeersFile>) {
    let list = peers.lock().unwrap().peers.clone();
    let _ = app.emit(EVT_PEERS, list);
}

/// Spawn (or replace) the supervisor task for one peer.
fn supervise_peer(app: &AppHandle, tasks: &Mutex<HashMap<String, JoinHandle<()>>>, peer: Peer) {
    let address = peer.address.clone();
    let handle = tauri::async_runtime::spawn(supervise(app.clone(), peer));
    if let Some(old) = tasks.lock().unwrap().insert(address, handle) {
        old.abort();
    }
}

/// Per-peer supervisor: probe identity, then stream metrics, emitting events.
/// On any drop it marks the peer offline and retries — the UI degrades to a
/// labelled disconnected state rather than a spinner (CLAUDE.md §7).
async fn supervise(app: AppHandle, peer: Peer) {
    let client = AgentClient::new(peer.base_url());
    loop {
        match client.info().await {
            Ok(info) => {
                let _ = app.emit(
                    EVT_STATUS,
                    StatusEvent { address: peer.address.clone(), online: true, info: Some(info) },
                );
            }
            Err(_) => {
                offline(&app, &peer.address);
                tokio::time::sleep(RETRY).await;
                continue;
            }
        }

        let stream = client.metrics_stream();
        futures_util::pin_mut!(stream);
        while let Some(item) = stream.next().await {
            match item {
                Ok(metrics) => {
                    let _ = app.emit(
                        EVT_METRICS,
                        MetricsEvent { address: peer.address.clone(), metrics },
                    );
                }
                Err(_) => break,
            }
        }
        offline(&app, &peer.address);
        tokio::time::sleep(RETRY).await;
    }
}

fn offline(app: &AppHandle, address: &str) {
    let _ = app.emit(
        EVT_STATUS,
        StatusEvent { address: address.to_string(), online: false, info: None },
    );
}

fn peers_path(app: &AppHandle) -> PathBuf {
    app.path()
        .app_config_dir()
        .unwrap_or_else(|_| PathBuf::from("."))
        .join("peers.toml")
}

/// Entry point invoked from main.rs.
pub fn run() {
    tauri::Builder::default()
        .setup(|app| {
            let handle = app.handle().clone();
            let path = peers_path(&handle);
            let loaded = PeersFile::load(&path).unwrap_or_default();

            app.manage(AppState {
                path,
                peers: Mutex::new(loaded.clone()),
                tasks: Mutex::new(HashMap::new()),
            });

            let state = app.state::<AppState>();
            for peer in loaded.peers.iter().cloned() {
                supervise_peer(&handle, &state.tasks, peer);
            }
            let _ = handle.emit(EVT_PEERS, loaded.peers);
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            list_peers,
            add_peer,
            remove_peer,
            rename_peer
        ])
        .run(tauri::generate_context!())
        .expect("error while running Oikos");
}
