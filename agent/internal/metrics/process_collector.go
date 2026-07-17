package metrics

import (
	"sort"

	"github.com/x2core/mrhandly/agent/internal/procfs"
	"github.com/x2core/mrhandly/agent/internal/protocol"
)

// ProcessCollector projects the process table, computing per-process CPU as a
// fraction of total machine capacity over the interval between two reads. It is
// stateful (holds the previous per-pid jiffies and total CPU) and not safe for
// concurrent use — the sampler drives one from its single goroutine.
//
// This is the expensive read (~300 pids), so it runs only while a client has
// the Processes view open (CLAUDE.md §8) — the sampler guarantees that.
type ProcessCollector struct {
	r         *procfs.Reader
	prev      map[int]uint64 // pid -> cumulative jiffies
	prevTotal uint64         // aggregate /proc/stat total jiffies
	prevValid bool
}

// NewProcessCollector returns a collector rooted at root.
func NewProcessCollector(root string) *ProcessCollector {
	return &ProcessCollector{r: procfs.New(root), prev: map[int]uint64{}}
}

// Reset drops the CPU baseline so the first frame after an idle gap does not
// report a delta spanning it.
func (c *ProcessCollector) Reset() { c.prevValid = false }

// Collect reads the process table and returns it sorted by CPU descending.
func (c *ProcessCollector) Collect() ([]protocol.Process, error) {
	var st procfs.Stat
	if err := c.r.Stat(&st); err != nil {
		return nil, err
	}
	total := st.Total.Total()

	procs, err := c.r.Processes()
	if err != nil {
		return nil, err
	}

	totalDelta := total - c.prevTotal
	usable := c.prevValid && total > c.prevTotal

	next := make(map[int]uint64, len(procs))
	out := make([]protocol.Process, 0, len(procs))
	for _, p := range procs {
		next[p.PID] = p.Jiffies
		cpu := 0.0
		if usable {
			if prevJ, ok := c.prev[p.PID]; ok && p.Jiffies >= prevJ && totalDelta > 0 {
				cpu = clamp01(float64(p.Jiffies-prevJ) / float64(totalDelta))
			}
		}
		out = append(out, protocol.Process{
			PID:     p.PID,
			PPID:    p.PPID,
			Name:    displayName(p),
			State:   p.State,
			CPU:     cpu,
			RSS:     p.RSS,
			Threads: p.Threads,
		})
	}

	c.prev = next
	c.prevTotal = total
	c.prevValid = true

	sort.Slice(out, func(i, j int) bool {
		if out[i].CPU != out[j].CPU {
			return out[i].CPU > out[j].CPU
		}
		return out[i].RSS > out[j].RSS
	})
	return out, nil
}

func displayName(p procfs.ProcStat) string {
	if p.Cmdline != "" {
		return p.Cmdline
	}
	return p.Comm
}
