// Package protocol is the Go mirror of packages/protocol. The JSON shape here
// is the contract with the desktop app and must match the TypeScript
// definitions byte-for-byte (CLAUDE.md §10). If the two drift, that is a bug.
package protocol

// Version is the wire protocol version. It must equal PROTOCOL_VERSION in
// packages/protocol/src/index.ts.
const Version = 1

// ErrorCode is a stable, switchable identifier for an agent error. The UI
// switches on the code; the message is for humans only.
type ErrorCode string

const (
	ErrInternal      ErrorCode = "internal"
	ErrNotFound      ErrorCode = "not_found"
	ErrBadRequest    ErrorCode = "bad_request"
	ErrPeerForbidden ErrorCode = "peer_forbidden"
)

// APIError is the structured error envelope for every agent error response.
type APIError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

func (e APIError) Error() string { return string(e.Code) + ": " + e.Message }

// Info is the response of GET /v1/info: identity and capabilities, cached at
// agent start.
type Info struct {
	Protocol     int          `json:"protocol"`
	Agent        AgentInfo    `json:"agent"`
	Host         HostInfo     `json:"host"`
	Capabilities Capabilities `json:"capabilities"`
}

type AgentInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
}

type HostInfo struct {
	Hostname    string `json:"hostname"`
	Kernel      string `json:"kernel"`
	Distro      string `json:"distro"`
	Arch        string `json:"arch"`
	CPUs        int    `json:"cpus"`
	TotalMemory uint64 `json:"total_memory"`
	BootTime    int64  `json:"boot_time"`
}

type Capabilities struct {
	Systemd bool `json:"systemd"`
	Docker  bool `json:"docker"`
}

// Metrics is a single live projection of the host, emitted by GET /v1/metrics
// and each frame of GET /v1/metrics/stream.
type Metrics struct {
	Timestamp     int64                     `json:"timestamp"` // unix milliseconds
	CPU           CPUMetrics                `json:"cpu"`
	Memory        MemoryMetrics             `json:"memory"`
	Load          LoadMetrics               `json:"load"`
	UptimeSeconds float64                   `json:"uptime_seconds"`
	Network       map[string]NetDevMetrics  `json:"network"`
	Disk          map[string]DiskMetrics    `json:"disk"`
}

type CPUMetrics struct {
	Usage   float64   `json:"usage"`    // aggregate busy fraction 0..1
	PerCore []float64 `json:"per_core"` // per-core busy fraction 0..1
}

type MemoryMetrics struct {
	Total     uint64 `json:"total"`
	Used      uint64 `json:"used"`
	Free      uint64 `json:"free"`
	Available uint64 `json:"available"`
	Buffers   uint64 `json:"buffers"`
	Cached    uint64 `json:"cached"`
	SwapTotal uint64 `json:"swap_total"`
	SwapUsed  uint64 `json:"swap_used"`
}

type LoadMetrics struct {
	One     float64 `json:"one"`
	Five    float64 `json:"five"`
	Fifteen float64 `json:"fifteen"`
}

// NetDevMetrics holds cumulative counters for one network interface.
type NetDevMetrics struct {
	RxBytes   uint64 `json:"rx_bytes"`
	TxBytes   uint64 `json:"tx_bytes"`
	RxPackets uint64 `json:"rx_packets"`
	TxPackets uint64 `json:"tx_packets"`
}

// DiskMetrics holds cumulative I/O counters for one block device. Sectors are
// 512 bytes.
type DiskMetrics struct {
	Reads        uint64 `json:"reads"`
	Writes       uint64 `json:"writes"`
	ReadSectors  uint64 `json:"read_sectors"`
	WriteSectors uint64 `json:"write_sectors"`
}
