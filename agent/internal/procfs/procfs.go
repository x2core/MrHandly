// Package procfs reads and parses the handful of /proc and /sys files the
// agent projects as host metrics. The filesystem root is injectable so the
// whole package is testable against fixtures with no root, systemd or Docker
// (CLAUDE.md §5 "Design for fixtures first").
//
// A Reader owns a reusable buffer and is NOT safe for concurrent use: the
// sampler gives each source goroutine its own Reader, and one-shot handlers
// construct a throwaway one. This is what keeps the hot path allocation-free.
package procfs

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
)

// Reader reads procfs files under a fixed root. Construct one per goroutine.
type Reader struct {
	statPath   string
	memPath    string
	netPath    string
	loadPath   string
	uptimePath string
	diskPath   string

	buf []byte // reused across reads to keep the hot path allocation-free
}

// New returns a Reader rooted at root. In production root is "/"; in tests it
// points at a fixture tree (e.g. "testdata/host-a").
func New(root string) *Reader {
	return &Reader{
		statPath:   filepath.Join(root, "proc", "stat"),
		memPath:    filepath.Join(root, "proc", "meminfo"),
		netPath:    filepath.Join(root, "proc", "net", "dev"),
		loadPath:   filepath.Join(root, "proc", "loadavg"),
		uptimePath: filepath.Join(root, "proc", "uptime"),
		diskPath:   filepath.Join(root, "proc", "diskstats"),
		buf:        make([]byte, 0, 4096),
	}
}

// slurp reads the whole file at path into the Reader's reusable buffer.
// procfs files report size 0, so this reads incrementally rather than trusting
// stat(2), and reuses the buffer between calls.
func (r *Reader) slurp(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r.buf = r.buf[:0]
	for {
		if len(r.buf) == cap(r.buf) {
			grown := make([]byte, len(r.buf), 2*cap(r.buf)+1)
			copy(grown, r.buf)
			r.buf = grown
		}
		n, err := f.Read(r.buf[len(r.buf):cap(r.buf)])
		r.buf = r.buf[:len(r.buf)+n]
		if err != nil {
			if err == io.EOF {
				return r.buf, nil
			}
			return nil, err
		}
	}
}

// ---------------------------------------------------------------------------
// /proc/stat
// ---------------------------------------------------------------------------

// CPUTimes holds the cumulative jiffy counters for one CPU line of /proc/stat.
type CPUTimes struct {
	User, Nice, System, Idle, Iowait, IRQ, SoftIRQ, Steal uint64
}

// Total is the sum of all counters (busy + idle).
func (c CPUTimes) Total() uint64 {
	return c.User + c.Nice + c.System + c.Idle + c.Iowait + c.IRQ + c.SoftIRQ + c.Steal
}

// Idleness is the portion of Total spent idle or waiting on I/O.
func (c CPUTimes) Idleness() uint64 { return c.Idle + c.Iowait }

// Stat is the parsed CPU section of /proc/stat.
type Stat struct {
	Total  CPUTimes   // the aggregate "cpu" line
	PerCPU []CPUTimes // the "cpu0", "cpu1", … lines, in order
}

// Stat reads and parses the CPU lines of /proc/stat into dst, reusing dst's
// PerCPU backing array. This is the hot path (1 Hz) and allocates nothing in
// steady state.
func (r *Reader) Stat(dst *Stat) error {
	b, err := r.slurp(r.statPath)
	if err != nil {
		return err
	}
	parseStat(b, dst)
	return nil
}

var cpuPrefix = []byte("cpu")

func parseStat(buf []byte, dst *Stat) {
	dst.PerCPU = dst.PerCPU[:0]
	for len(buf) > 0 {
		var line []byte
		line, buf = nextLine(buf)
		if !bytes.HasPrefix(line, cpuPrefix) {
			// CPU lines are contiguous at the top of /proc/stat; the first
			// non-cpu line ("intr", "ctxt", …) ends the section.
			break
		}
		name, rest := nextField(line)
		var t CPUTimes
		parseCPUTimes(rest, &t)
		if len(name) == len(cpuPrefix) { // exactly "cpu": the aggregate line
			dst.Total = t
		} else { // "cpu0", "cpu1", …
			dst.PerCPU = append(dst.PerCPU, t)
		}
	}
}

func parseCPUTimes(fields []byte, t *CPUTimes) {
	slots := [8]*uint64{
		&t.User, &t.Nice, &t.System, &t.Idle,
		&t.Iowait, &t.IRQ, &t.SoftIRQ, &t.Steal,
	}
	rest := fields
	for i := 0; i < len(slots); i++ {
		var f []byte
		f, rest = nextField(rest)
		if len(f) == 0 {
			break
		}
		if v, ok := atou(f); ok {
			*slots[i] = v
		}
	}
}

var btimePrefix = []byte("btime")

// BootTime returns the host boot time as a Unix timestamp, read from the
// "btime" line of /proc/stat. It returns 0 if the file or line is unavailable.
func (r *Reader) BootTime() int64 {
	b, err := r.slurp(r.statPath)
	if err != nil {
		return 0
	}
	for len(b) > 0 {
		var line []byte
		line, b = nextLine(b)
		if !bytes.HasPrefix(line, btimePrefix) {
			continue
		}
		_, rest := nextField(line) // drop "btime"
		val, _ := nextField(rest)
		v, _ := atou(val)
		return int64(v)
	}
	return 0
}

// ---------------------------------------------------------------------------
// /proc/meminfo
// ---------------------------------------------------------------------------

// MemInfo holds the fields of /proc/meminfo the agent reports, in bytes.
type MemInfo struct {
	Total     uint64
	Free      uint64
	Available uint64
	Buffers   uint64
	Cached    uint64
	SwapTotal uint64
	SwapFree  uint64
}

// MemInfo reads and parses /proc/meminfo.
func (r *Reader) MemInfo() (MemInfo, error) {
	b, err := r.slurp(r.memPath)
	if err != nil {
		return MemInfo{}, err
	}
	var m MemInfo
	parseMemInfo(b, &m)
	return m, nil
}

func parseMemInfo(buf []byte, m *MemInfo) {
	for len(buf) > 0 {
		var line []byte
		line, buf = nextLine(buf)
		colon := bytes.IndexByte(line, ':')
		if colon < 0 {
			continue
		}
		key := line[:colon]
		val, _ := nextField(line[colon+1:]) // first token is the value in kB
		kb, ok := atou(val)
		if !ok {
			continue
		}
		b := kb * 1024
		switch string(key) { // no allocation: compiler special-cases string([]byte) in switch
		case "MemTotal":
			m.Total = b
		case "MemFree":
			m.Free = b
		case "MemAvailable":
			m.Available = b
		case "Buffers":
			m.Buffers = b
		case "Cached":
			m.Cached = b
		case "SwapTotal":
			m.SwapTotal = b
		case "SwapFree":
			m.SwapFree = b
		}
	}
}

// ---------------------------------------------------------------------------
// /proc/net/dev
// ---------------------------------------------------------------------------

// NetDev holds cumulative counters for one interface.
type NetDev struct {
	RxBytes, RxPackets uint64
	TxBytes, TxPackets uint64
}

// NetDev reads and parses /proc/net/dev, keyed by interface name. The loopback
// interface is excluded — it is never interesting on the fleet view.
func (r *Reader) NetDev() (map[string]NetDev, error) {
	b, err := r.slurp(r.netPath)
	if err != nil {
		return nil, err
	}
	out := make(map[string]NetDev)
	parseNetDev(b, out)
	return out, nil
}

func parseNetDev(buf []byte, out map[string]NetDev) {
	_, rest := nextLine(buf)  // "Inter-|   Receive …" header
	_, rest = nextLine(rest)  // " face |bytes packets …" header
	buf = rest
	for len(buf) > 0 {
		var l []byte
		l, buf = nextLine(buf)
		colon := bytes.IndexByte(l, ':')
		if colon < 0 {
			continue
		}
		name, _ := nextField(l[:colon])
		if len(name) == 0 || string(name) == "lo" {
			continue
		}
		// Fields after the colon: rx bytes packets errs drop fifo frame
		// compressed multicast | tx bytes packets ...
		f := l[colon+1:]
		var d NetDev
		var field []byte
		for i := 0; i < 10; i++ {
			field, f = nextField(f)
			v, _ := atou(field)
			switch i {
			case 0:
				d.RxBytes = v
			case 1:
				d.RxPackets = v
			case 8:
				d.TxBytes = v
			case 9:
				d.TxPackets = v
			}
		}
		out[string(name)] = d
	}
}

// ---------------------------------------------------------------------------
// /proc/loadavg
// ---------------------------------------------------------------------------

// LoadAvg holds the 1/5/15-minute load averages.
type LoadAvg struct {
	One, Five, Fifteen float64
}

// LoadAvg reads and parses /proc/loadavg.
func (r *Reader) LoadAvg() (LoadAvg, error) {
	b, err := r.slurp(r.loadPath)
	if err != nil {
		return LoadAvg{}, err
	}
	var la LoadAvg
	f1, rest := nextField(b)
	f2, rest := nextField(rest)
	f3, _ := nextField(rest)
	la.One, _ = atof(f1)
	la.Five, _ = atof(f2)
	la.Fifteen, _ = atof(f3)
	return la, nil
}

// ---------------------------------------------------------------------------
// /proc/uptime
// ---------------------------------------------------------------------------

// Uptime reads /proc/uptime and returns seconds since boot.
func (r *Reader) Uptime() (float64, error) {
	b, err := r.slurp(r.uptimePath)
	if err != nil {
		return 0, err
	}
	f, _ := nextField(b)
	up, _ := atof(f)
	return up, nil
}

// ---------------------------------------------------------------------------
// /proc/diskstats
// ---------------------------------------------------------------------------

// DiskStat holds cumulative I/O counters for one block device. Sectors are
// 512 bytes.
type DiskStat struct {
	Reads, Writes             uint64
	ReadSectors, WriteSectors uint64
}

// DiskStats reads and parses /proc/diskstats, keyed by device name. Virtual
// loop and ram devices are excluded.
func (r *Reader) DiskStats() (map[string]DiskStat, error) {
	b, err := r.slurp(r.diskPath)
	if err != nil {
		return nil, err
	}
	out := make(map[string]DiskStat)
	parseDiskStats(b, out)
	return out, nil
}

func parseDiskStats(buf []byte, out map[string]DiskStat) {
	for len(buf) > 0 {
		var l []byte
		l, buf = nextLine(buf)
		// Fields: major minor name reads rmerged sread ms writes wmerged
		// swritten ...
		var fields [10][]byte
		rest := l
		n := 0
		for n < len(fields) {
			var f []byte
			f, rest = nextField(rest)
			if len(f) == 0 {
				break
			}
			fields[n] = f
			n++
		}
		if n < 10 {
			continue
		}
		name := fields[2]
		if bytes.HasPrefix(name, []byte("loop")) || bytes.HasPrefix(name, []byte("ram")) {
			continue
		}
		var d DiskStat
		d.Reads, _ = atou(fields[3])
		d.ReadSectors, _ = atou(fields[5])
		d.Writes, _ = atou(fields[7])
		d.WriteSectors, _ = atou(fields[9])
		out[string(name)] = d
	}
}
