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
