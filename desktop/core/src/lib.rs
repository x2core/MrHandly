//! oikos-core — the desktop app's aggregation logic, free of any Tauri or
//! webkit dependency so it builds and tests anywhere. src-tauri is a thin shell
//! that wires this into commands and events; all HTTP/SSE and the peer list
//! live here.

pub mod client;
pub mod error;
pub mod peers;
pub mod types;

pub use client::AgentClient;
pub use error::{Error, Result};
pub use peers::{Peer, PeersFile, DEFAULT_PORT};
pub use types::{Info, Metrics};
