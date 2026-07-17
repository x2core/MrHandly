// Package metrics maps the raw procfs readings into a protocol.Metrics frame,
// including the CPU-usage deltas that need state across two reads. One
// Collector backs both the one-shot GET /v1/metrics and the sampler's stream,
// so the two can never disagree on how a number is computed.
//
// A Collector is not safe for concurrent use: it owns a procfs.Reader and the
// previous CPU sample. The sampler drives one Collector from its single
// goroutine; one-shot handlers construct a throwaway Collector.
package metrics

import (
	"time"

	"github.com/x2core/mrhandly/agent/internal/procfs"
	"github.com/x2core/mrhandly/agent/internal/protocol"
)

// Collector reads a host's metrics under a fixed procfs root.
type Collector struct {
	r   *procfs.Reader
	now func() time.Time

	prev      procfs.Stat
	prevValid bool
}

// NewCollector returns a Collector rooted at root. now is injectable for
// deterministic timestamps in tests; pass time.Now in production.
func NewCollector(root string, now func() time.Time) *Collector {
	if now == nil {
		now = time.Now
	}
	return &Collector{r: procfs.New(root), now: now}
}

// Reset invalidates the CPU baseline. The sampler calls it when its goroutine
// (re)starts so the first frame after an idle gap does not report a delta
// spanning that gap.
func (c *Collector) Reset() { c.prevValid = false }

// Collect reads every source and returns one metrics frame. The first call
// after a Reset reports CPU usage averaged since boot; subsequent calls report
// usage over the interval between calls.
func (c *Collector) Collect() (protocol.Metrics, error) {
	var cur procfs.Stat
	if err := c.r.Stat(&cur); err != nil {
		return protocol.Metrics{}, err
	}
	mem, err := c.r.MemInfo()
	if err != nil {
		return protocol.Metrics{}, err
	}
	load, err := c.r.LoadAvg()
	if err != nil {
		return protocol.Metrics{}, err
	}
	uptime, err := c.r.Uptime()
	if err != nil {
		return protocol.Metrics{}, err
	}
	net, err := c.r.NetDev()
	if err != nil {
		return protocol.Metrics{}, err
	}
	disk, err := c.r.DiskStats()
	if err != nil {
		return protocol.Metrics{}, err
	}

	m := protocol.Metrics{
		Timestamp:     c.now().UnixMilli(),
		CPU:           c.cpu(cur),
		Memory:        mapMemory(mem),
		Load:          protocol.LoadMetrics{One: load.One, Five: load.Five, Fifteen: load.Fifteen},
		UptimeSeconds: uptime,
		Network:       mapNet(net),
		Disk:          mapDisk(disk),
	}

	// Snapshot this read as the baseline for the next delta. cur.PerCPU aliases
	// the reader's reused buffer, so copy it.
	c.prev.Total = cur.Total
	c.prev.PerCPU = append(c.prev.PerCPU[:0], cur.PerCPU...)
	c.prevValid = true
	return m, nil
}

func (c *Collector) cpu(cur procfs.Stat) protocol.CPUMetrics {
	out := protocol.CPUMetrics{PerCore: make([]float64, len(cur.PerCPU))}
	if c.prevValid && len(c.prev.PerCPU) == len(cur.PerCPU) {
		out.Usage = usageDelta(c.prev.Total, cur.Total)
		for i := range cur.PerCPU {
			out.PerCore[i] = usageDelta(c.prev.PerCPU[i], cur.PerCPU[i])
		}
		return out
	}
	// No usable baseline: report usage averaged since boot.
	out.Usage = usageSinceBoot(cur.Total)
	for i := range cur.PerCPU {
		out.PerCore[i] = usageSinceBoot(cur.PerCPU[i])
	}
	return out
}

// usageDelta is the busy fraction over the interval between prev and cur.
func usageDelta(prev, cur procfs.CPUTimes) float64 {
	pt, ct := prev.Total(), cur.Total()
	if ct <= pt { // counter reset or no elapsed time
		return 0
	}
	totalD := ct - pt
	idleD := cur.Idleness() - prev.Idleness()
	if idleD > totalD { // clock skew / partial read
		return 0
	}
	return clamp01(1 - float64(idleD)/float64(totalD))
}

// usageSinceBoot is the busy fraction over the whole time since boot.
func usageSinceBoot(c procfs.CPUTimes) float64 {
	total := c.Total()
	if total == 0 {
		return 0
	}
	return clamp01(1 - float64(c.Idleness())/float64(total))
}

func clamp01(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

func mapMemory(m procfs.MemInfo) protocol.MemoryMetrics {
	return protocol.MemoryMetrics{
		Total:     m.Total,
		Used:      satSub(m.Total, m.Available),
		Free:      m.Free,
		Available: m.Available,
		Buffers:   m.Buffers,
		Cached:    m.Cached,
		SwapTotal: m.SwapTotal,
		SwapUsed:  satSub(m.SwapTotal, m.SwapFree),
	}
}

func mapNet(in map[string]procfs.NetDev) map[string]protocol.NetDevMetrics {
	out := make(map[string]protocol.NetDevMetrics, len(in))
	for name, d := range in {
		out[name] = protocol.NetDevMetrics{
			RxBytes:   d.RxBytes,
			TxBytes:   d.TxBytes,
			RxPackets: d.RxPackets,
			TxPackets: d.TxPackets,
		}
	}
	return out
}

func mapDisk(in map[string]procfs.DiskStat) map[string]protocol.DiskMetrics {
	out := make(map[string]protocol.DiskMetrics, len(in))
	for name, d := range in {
		out[name] = protocol.DiskMetrics{
			Reads:        d.Reads,
			Writes:       d.Writes,
			ReadSectors:  d.ReadSectors,
			WriteSectors: d.WriteSectors,
		}
	}
	return out
}

// satSub is a saturating subtraction that never underflows the uint.
func satSub(a, b uint64) uint64 {
	if b > a {
		return 0
	}
	return a - b
}
