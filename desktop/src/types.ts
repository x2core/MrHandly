// Wire types, mirrored from packages/protocol (the source of truth, CLAUDE.md
// §10) and from the Rust event payloads in src-tauri. Kept minimal — the
// desktop shell only needs identity and metrics for the Fleet Strip in M4.

export interface Peer {
  name: string
  address: string
  port: number
}

export interface Info {
  protocol: number
  agent: { version: string; commit: string }
  host: {
    hostname: string
    kernel: string
    distro: string
    arch: string
    cpus: number
    total_memory: number
    boot_time: number
  }
  capabilities: { systemd: boolean; docker: boolean }
}

export interface Metrics {
  timestamp: number
  cpu: { usage: number; per_core: number[] }
  memory: {
    total: number
    used: number
    free: number
    available: number
    buffers: number
    cached: number
    swap_total: number
    swap_used: number
  }
  load: { one: number; five: number; fifteen: number }
  uptime_seconds: number
  network: Record<string, { rx_bytes: number; tx_bytes: number; rx_packets: number; tx_packets: number }>
  disk: Record<string, { reads: number; writes: number; read_sectors: number; write_sectors: number }>
}

// Event payloads emitted by the Rust core (src-tauri/src/lib.rs).
export interface StatusEvent {
  address: string
  online: boolean
  info: Info | null
}

export interface MetricsEvent {
  address: string
  metrics: Metrics
}

// M5 read surfaces.
export interface Service {
  name: string
  description: string
  load_state: string
  active_state: string
  sub_state: string
  writable: boolean
}

export interface Process {
  pid: number
  ppid: number
  name: string
  state: string
  cpu: number
  rss: number
  threads: number
}

export interface Container {
  id: string
  name: string
  image: string
  state: string
  status: string
  created: number
  writable: boolean
}

export interface Image {
  id: string
  tags: string[]
  size: number
  created: number
}

export interface LogRecord {
  /** unix ms, or 0 if unknown */
  timestamp: number
  message: string
  /** journald priority 0..7, or -1 for container logs */
  priority: number
  /** stdout|stderr for container logs, else the unit */
  source: string
}
