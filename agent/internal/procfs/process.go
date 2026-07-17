package procfs

import (
	"os"
	"path/filepath"
)

// pageSize is used to convert RSS (reported in pages) to bytes.
var pageSize = uint64(os.Getpagesize())

// ProcStat is one process's raw reading from /proc/<pid>/stat (+cmdline). CPU
// is left as cumulative jiffies (utime+stime); the collector turns it into a
// usage fraction across two reads.
type ProcStat struct {
	PID     int
	PPID    int
	Comm    string // from stat, truncated to 15 bytes by the kernel
	State   string // R, S, D, Z, …
	Jiffies uint64 // utime + stime
	RSS     uint64 // bytes
	Threads int
	Cmdline string // full command line, or "" (falls back to Comm)
}

// Processes reads the process table. It is the expensive path (~300 pids), so
// it runs only while a client has the Processes view open (CLAUDE.md §8). The
// Reader's buffer is reused across the per-pid stat reads.
func (r *Reader) Processes() ([]ProcStat, error) {
	procRoot := filepath.Dir(r.statPath) // <root>/proc
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return nil, err
	}
	out := make([]ProcStat, 0, len(entries))
	for _, e := range entries {
		pid, ok := allDigits(e.Name())
		if !ok {
			continue
		}
		b, err := r.slurp(filepath.Join(procRoot, e.Name(), "stat"))
		if err != nil {
			continue // process exited between readdir and read — skip
		}
		ps, ok := parseProcStat(b)
		if !ok {
			continue
		}
		ps.PID = pid
		if cmd, err := os.ReadFile(filepath.Join(procRoot, e.Name(), "cmdline")); err == nil {
			ps.Cmdline = cmdline(cmd)
		}
		out = append(out, ps)
	}
	return out, nil
}

func allDigits(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

// parseProcStat parses /proc/<pid>/stat. comm is wrapped in parentheses and may
// contain spaces and parens, so we split on the LAST ')'.
func parseProcStat(b []byte) (ProcStat, bool) {
	open := indexByte(b, '(')
	close := lastIndexByte(b, ')')
	if open < 0 || close < 0 || close < open {
		return ProcStat{}, false
	}
	var ps ProcStat
	ps.Comm = string(b[open+1 : close])

	// Fields after "comm) ": index 0 == field 3 (state).
	rest := b[close+1:]
	// state(3) ppid(4) ... utime(14) stime(15) ... num_threads(20) ... rss(24)
	// zero-based token offsets from state: state=0, ppid=1, utime=11, stime=12,
	// num_threads=17, rss=21.
	var (
		utime, stime uint64
		haveU, haveS bool
	)
	i := 0
	for len(rest) > 0 {
		var f []byte
		f, rest = nextField(rest)
		if len(f) == 0 {
			break
		}
		switch i {
		case 0:
			ps.State = string(f)
		case 1:
			if v, ok := atou(f); ok {
				ps.PPID = int(v)
			}
		case 11:
			utime, haveU = atou(f)
		case 12:
			stime, haveS = atou(f)
		case 17:
			if v, ok := atou(f); ok {
				ps.Threads = int(v)
			}
		case 21:
			if v, ok := atou(f); ok {
				ps.RSS = v * pageSize
			}
		}
		i++
	}
	if haveU {
		ps.Jiffies += utime
	}
	if haveS {
		ps.Jiffies += stime
	}
	return ps, true
}

// cmdline turns the null-separated /proc/<pid>/cmdline into a space-joined
// string, trimming the trailing null.
func cmdline(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	// Trim trailing NULs.
	for len(b) > 0 && b[len(b)-1] == 0 {
		b = b[:len(b)-1]
	}
	out := make([]byte, len(b))
	for i, c := range b {
		if c == 0 {
			out[i] = ' '
		} else {
			out[i] = c
		}
	}
	return string(out)
}

func indexByte(b []byte, c byte) int {
	for i := 0; i < len(b); i++ {
		if b[i] == c {
			return i
		}
	}
	return -1
}

func lastIndexByte(b []byte, c byte) int {
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] == c {
			return i
		}
	}
	return -1
}
