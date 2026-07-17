// @oikos/protocol — the single source of truth for the Oikos wire API.
//
// Types declared here are mirrored into Go, byte-for-byte on the JSON shape
// (agent/internal/protocol). If the two sides drift, that is a bug, not a
// variation (CLAUDE.md §10). Agent errors carry a stable `code`; the UI
// switches on `code`, never on message text.

/**
 * Protocol version negotiated between the desktop app and each agent. Bumped
 * only on an incompatible change to the wire format.
 */
export const PROTOCOL_VERSION = 1 as const;

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

/**
 * Stable machine-readable error codes returned by the agent. The UI switches
 * on these; message text is for humans and may change freely. More codes are
 * added as endpoints land (`docker_unavailable`, `unit_not_allowed`, …).
 */
export type ErrorCode =
  | 'internal'
  | 'not_found'
  | 'bad_request'
  | 'peer_forbidden'
  | 'systemd_unavailable'
  | 'unit_not_allowed';

/** The structured error envelope every agent error response conforms to. */
export interface ApiError {
  /** Stable, switchable identifier. */
  code: ErrorCode;
  /** Human-readable description. Never parse this. */
  message: string;
}

// ---------------------------------------------------------------------------
// GET /v1/info — identity and capabilities, cached at agent start.
// ---------------------------------------------------------------------------

export interface Info {
  /** Wire protocol version; matches {@link PROTOCOL_VERSION}. */
  protocol: number;
  agent: AgentInfo;
  host: HostInfo;
  capabilities: Capabilities;
}

export interface AgentInfo {
  /** Semver release, or a git describe for dev builds. */
  version: string;
  /** Full commit SHA the binary was built from. */
  commit: string;
}

export interface HostInfo {
  hostname: string;
  /** Kernel release, e.g. `6.1.0-18-amd64`. */
  kernel: string;
  /** Pretty distro name from /etc/os-release, e.g. `Debian GNU/Linux 12`. */
  distro: string;
  /** GOARCH of the running binary, e.g. `amd64`, `arm64`. */
  arch: string;
  /** Number of logical CPUs. */
  cpus: number;
  /** Total physical memory in bytes. */
  total_memory: number;
  /** Boot time as a Unix timestamp in seconds. */
  boot_time: number;
}

export interface Capabilities {
  /** systemd is the init system and its D-Bus API is reachable. */
  systemd: boolean;
  /** A dialable Docker socket is present (re-probed lazily). */
  docker: boolean;
}

// ---------------------------------------------------------------------------
// GET /v1/metrics and GET /v1/metrics/stream (SSE) — a live host projection.
// ---------------------------------------------------------------------------

export interface Metrics {
  /** Sample time as a Unix timestamp in milliseconds. */
  timestamp: number;
  cpu: CpuMetrics;
  memory: MemoryMetrics;
  load: LoadMetrics;
  /** Seconds since boot. */
  uptime_seconds: number;
  /** Per-interface cumulative counters, keyed by interface name. */
  network: Record<string, NetDevMetrics>;
  /** Per-device cumulative counters, keyed by device name. */
  disk: Record<string, DiskMetrics>;
}

export interface CpuMetrics {
  /** Aggregate busy fraction over the sample interval, 0..1. */
  usage: number;
  /** Per-core busy fraction over the sample interval, 0..1. */
  per_core: number[];
}

export interface MemoryMetrics {
  /** All values in bytes. */
  total: number;
  /** total - available. */
  used: number;
  free: number;
  available: number;
  buffers: number;
  cached: number;
  swap_total: number;
  swap_used: number;
}

export interface LoadMetrics {
  one: number;
  five: number;
  fifteen: number;
}

/** Cumulative byte/packet counters for one network interface. */
export interface NetDevMetrics {
  rx_bytes: number;
  tx_bytes: number;
  rx_packets: number;
  tx_packets: number;
}

/** Cumulative I/O counters for one block device. Sectors are 512 bytes. */
export interface DiskMetrics {
  reads: number;
  writes: number;
  read_sectors: number;
  write_sectors: number;
}

// ---------------------------------------------------------------------------
// Services — systemd units (M2).
//   GET  /v1/services            → Service[]
//   GET  /v1/services/:unit      → Service
//   POST /v1/services/:unit/{start,stop,restart}
//   GET  /v1/services/:unit/logs?follow=1  → SSE of LogLine
// ---------------------------------------------------------------------------

/** A systemd unit as projected by the agent. */
export interface Service {
  /** Unit name, e.g. `nginx.service`. */
  name: string;
  description: string;
  /** loaded | not-found | error | masked … */
  load_state: string;
  /** active | inactive | failed | activating | deactivating … */
  active_state: string;
  /** running | dead | exited | listening … */
  sub_state: string;
  /**
   * Whether write actions (start/stop/restart) are permitted on this unit by
   * the host's unit_allowlist. The UI enables or visibly disables the action
   * controls on this flag — the boundary is shown, not hidden.
   */
  writable: boolean;
}

/** One journald entry, as streamed from GET /v1/services/:unit/logs. */
export interface LogLine {
  /** Entry time as a Unix timestamp in milliseconds. */
  timestamp: number;
  /** syslog priority 0 (emerg) … 7 (debug); defaults to 6 (info). */
  priority: number;
  message: string;
  /** Emitting unit, when journald reports one. */
  unit: string;
}
