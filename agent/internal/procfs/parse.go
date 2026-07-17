package procfs

import "strconv"

// Byte-level parsing helpers for the /proc and /sys hot paths. These avoid the
// allocations that strings.Split / strings.Fields would incur per tick
// (CLAUDE.md §8: parse bytes, not strings).

// nextLine splits the first line off b, returning the line (without its
// trailing '\n') and the remainder.
func nextLine(b []byte) (line, rest []byte) {
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' {
			return b[:i], b[i+1:]
		}
	}
	return b, nil
}

// nextField returns the next whitespace-delimited field of b and the remainder
// following it. Leading spaces and tabs are skipped.
func nextField(b []byte) (field, rest []byte) {
	isSep := func(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }
	i := 0
	for i < len(b) && isSep(b[i]) {
		i++
	}
	b = b[i:]
	j := 0
	for j < len(b) && !isSep(b[j]) {
		j++
	}
	return b[:j], b[j:]
}

// atou parses an unsigned base-10 integer from b without allocating. It
// returns false if b is empty or contains a non-digit.
func atou(b []byte) (uint64, bool) {
	if len(b) == 0 {
		return 0, false
	}
	var n uint64
	for _, c := range b {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + uint64(c-'0')
	}
	return n, true
}

// atof parses a float from b. Floats appear only in cold-ish 1 Hz paths
// (loadavg, uptime) where the tiny conversion cost is irrelevant, so this
// leans on the standard library for correctness.
func atof(b []byte) (float64, bool) {
	f, err := strconv.ParseFloat(string(b), 64)
	if err != nil {
		return 0, false
	}
	return f, true
}
