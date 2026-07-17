package docker

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/x2core/mrhandly/agent/internal/protocol"
)

// LogBacklog is the number of historical lines fetched before following.
const LogBacklog = 100

// ContainerLogs streams a container's output. Docker multiplexes stdout and
// stderr over a single connection with an 8-byte frame header UNLESS the
// container was created with a TTY, in which case the stream is raw. We inspect
// Tty first and demux accordingly (docs/ROADMAP.md M3).
//
// The stream is bound to ctx: cancelling it (a client disconnect) unblocks the
// body read and the goroutine closes the response and the output channel — no
// leaked connection.
func (c *Client) ContainerLogs(ctx context.Context, id string, follow bool) (<-chan protocol.ContainerLog, error) {
	insp, err := c.inspectRaw(ctx, id)
	if err != nil {
		return nil, err
	}
	tty := insp.Config.Tty

	q := url.Values{}
	q.Set("stdout", "1")
	q.Set("stderr", "1")
	q.Set("timestamps", "1")
	q.Set("tail", fmt.Sprintf("%d", LogBacklog))
	if follow {
		q.Set("follow", "1")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/containers/"+id+"/logs?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if resp.StatusCode == http.StatusNotFound {
		drainClose(resp.Body)
		return nil, ErrNotFound
	}
	if resp.StatusCode >= 400 {
		drainClose(resp.Body)
		return nil, fmt.Errorf("docker: logs status %d", resp.StatusCode)
	}

	out := make(chan protocol.ContainerLog, 64)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		demux(resp.Body, tty, func(stream string, payload []byte) bool {
			for _, raw := range splitLines(payload) {
				ts, msg := splitTimestamp(raw)
				select {
				case out <- protocol.ContainerLog{Stream: stream, Timestamp: ts, Message: msg}:
				case <-ctx.Done():
					return false
				}
			}
			return true
		})
	}()
	return out, nil
}

// demux reads the Docker log stream and calls emit for each frame's payload.
// It stops on read error, EOF, or when emit returns false. For a TTY stream
// there is no framing, so each scanned line is emitted as stdout.
func demux(r io.Reader, tty bool, emit func(stream string, payload []byte) bool) {
	if tty {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			if !emit("stdout", sc.Bytes()) {
				return
			}
		}
		return
	}

	header := make([]byte, 8)
	for {
		if _, err := io.ReadFull(r, header); err != nil {
			return
		}
		// header[0]: 0=stdin, 1=stdout, 2=stderr. header[4:8]: big-endian size.
		stream := "stdout"
		if header[0] == 2 {
			stream = "stderr"
		}
		size := binary.BigEndian.Uint32(header[4:8])
		if size == 0 {
			continue
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(r, payload); err != nil {
			return
		}
		if !emit(stream, payload) {
			return
		}
	}
}

// splitLines splits a frame payload into non-empty lines, trimming trailing
// carriage returns.
func splitLines(payload []byte) []string {
	text := strings.TrimRight(string(payload), "\n")
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], "\r")
	}
	return lines
}

// splitTimestamp separates the RFC3339Nano timestamp that Docker prepends when
// timestamps=1 from the message. If the leading token doesn't parse as a time,
// the whole line is the message and the timestamp is 0.
func splitTimestamp(line string) (int64, string) {
	space := strings.IndexByte(line, ' ')
	if space < 0 {
		return 0, line
	}
	t, err := time.Parse(time.RFC3339Nano, line[:space])
	if err != nil {
		return 0, line
	}
	return t.UnixMilli(), line[space+1:]
}

// parseRFC3339Seconds parses a container's Created timestamp (RFC3339Nano) to
// Unix seconds, returning 0 on failure.
func parseRFC3339Seconds(s string) int64 {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return 0
	}
	return t.Unix()
}
