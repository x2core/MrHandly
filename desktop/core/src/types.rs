//! Wire types mirrored from `packages/protocol`. The JSON shape must match the
//! agent's output byte-for-byte (CLAUDE.md §10).

use serde::{Deserialize, Serialize};
use std::collections::BTreeMap;

/// Response of `GET /v1/info`.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Info {
    pub protocol: u32,
    pub agent: AgentInfo,
    pub host: HostInfo,
    pub capabilities: Capabilities,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AgentInfo {
    pub version: String,
    pub commit: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct HostInfo {
    pub hostname: String,
    pub kernel: String,
    pub distro: String,
    pub arch: String,
    pub cpus: u32,
    pub total_memory: u64,
    pub boot_time: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Capabilities {
    pub systemd: bool,
    pub docker: bool,
}

/// A single metrics frame from `GET /v1/metrics` or the SSE stream.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Metrics {
    pub timestamp: i64,
    pub cpu: CpuMetrics,
    pub memory: MemoryMetrics,
    pub load: LoadMetrics,
    pub uptime_seconds: f64,
    pub network: BTreeMap<String, NetDevMetrics>,
    pub disk: BTreeMap<String, DiskMetrics>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CpuMetrics {
    pub usage: f64,
    pub per_core: Vec<f64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MemoryMetrics {
    pub total: u64,
    pub used: u64,
    pub free: u64,
    pub available: u64,
    pub buffers: u64,
    pub cached: u64,
    pub swap_total: u64,
    pub swap_used: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LoadMetrics {
    pub one: f64,
    pub five: f64,
    pub fifteen: f64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct NetDevMetrics {
    pub rx_bytes: u64,
    pub tx_bytes: u64,
    pub rx_packets: u64,
    pub tx_packets: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DiskMetrics {
    pub reads: u64,
    pub writes: u64,
    pub read_sectors: u64,
    pub write_sectors: u64,
}

/// The structured error envelope the agent returns (`code` is stable).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ApiError {
    pub code: String,
    pub message: String,
}

// M5 read surfaces: services, processes, docker, logs.

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Service {
    pub name: String,
    pub description: String,
    pub load_state: String,
    pub active_state: String,
    pub sub_state: String,
    pub writable: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Process {
    pub pid: i32,
    pub ppid: i32,
    pub name: String,
    pub state: String,
    pub cpu: f64,
    pub rss: u64,
    pub threads: i32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Container {
    pub id: String,
    pub name: String,
    pub image: String,
    pub state: String,
    pub status: String,
    pub created: i64,
    pub writable: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Image {
    pub id: String,
    pub tags: Vec<String>,
    pub size: i64,
    pub created: i64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LogLine {
    pub timestamp: i64,
    pub priority: i32,
    pub message: String,
    pub unit: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ContainerLog {
    pub stream: String,
    pub timestamp: i64,
    pub message: String,
}
