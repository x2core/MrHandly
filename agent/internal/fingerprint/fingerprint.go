// Package fingerprint detects host identity and capabilities once at agent
// start. Everything it reads is rooted at an injectable filesystem path and
// its capability probes are injectable functions, so it is fully testable
// against fixtures with no systemd, Docker or root (CLAUDE.md §5).
//
// Host identity is cached; the Docker capability is re-probed lazily on each
// Info call, because a Docker socket can appear or disappear while the agent
// runs (docs/ROADMAP.md M1).
package fingerprint

import (
	"bufio"
	"bytes"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/x2core/mrhandly/agent/internal/procfs"
	"github.com/x2core/mrhandly/agent/internal/protocol"
)

// Options configures capability detection.
type Options struct {
	// Root is the filesystem root to read from ("/" in production).
	Root string
	// Arch is the running binary's architecture (runtime.GOARCH).
	Arch string
	// Systemd probes whether systemd is the init system. If nil, a default
	// probe checks for /run/systemd/system under Root.
	Systemd func() bool
	// Docker probes whether a dialable Docker socket is present. If nil, a
	// default probe dials DockerSocket. Re-evaluated on every Info call.
	Docker func() bool
	// DockerSocket is the socket path for the default Docker probe.
	DockerSocket string
}

// Fingerprint is the cached host identity plus live capability probes.
type Fingerprint struct {
	host    protocol.HostInfo
	systemd bool             // detected once at start
	docker  func() bool      // re-probed lazily
}

// Detect reads host identity and evaluates the systemd capability once.
func Detect(opts Options) *Fingerprint {
	root := opts.Root
	if root == "" {
		root = "/"
	}

	pr := procfs.New(root)
	host := protocol.HostInfo{
		Arch:     opts.Arch,
		Hostname: readLine(root, "proc", "sys", "kernel", "hostname"),
		Kernel:   readLine(root, "proc", "sys", "kernel", "osrelease"),
		Distro:   readDistro(root),
		BootTime: pr.BootTime(),
	}
	var st procfs.Stat
	if err := pr.Stat(&st); err == nil {
		host.CPUs = len(st.PerCPU)
	}
	if mi, err := pr.MemInfo(); err == nil {
		host.TotalMemory = mi.Total
	}

	systemd := opts.Systemd
	if systemd == nil {
		systemd = defaultSystemdProbe(root)
	}
	docker := opts.Docker
	if docker == nil {
		docker = defaultDockerProbe(opts.DockerSocket)
	}

	return &Fingerprint{
		host:    host,
		systemd: systemd(),
		docker:  docker,
	}
}

// Host returns the cached host identity.
func (f *Fingerprint) Host() protocol.HostInfo { return f.host }

// Systemd reports the cached systemd capability.
func (f *Fingerprint) Systemd() bool { return f.systemd }

// Docker re-probes and reports the Docker capability.
func (f *Fingerprint) Docker() bool { return f.docker() }

// Info assembles the /v1/info payload, re-probing Docker.
func (f *Fingerprint) Info(version, commit string) protocol.Info {
	return protocol.Info{
		Protocol: protocol.Version,
		Agent:    protocol.AgentInfo{Version: version, Commit: commit},
		Host:     f.host,
		Capabilities: protocol.Capabilities{
			Systemd: f.systemd,
			Docker:  f.docker(),
		},
	}
}

// defaultSystemdProbe reports whether systemd is the init system by checking
// for its runtime directory.
func defaultSystemdProbe(root string) func() bool {
	return func() bool {
		fi, err := os.Stat(filepath.Join(root, "run", "systemd", "system"))
		return err == nil && fi.IsDir()
	}
}

// defaultDockerProbe reports whether the Docker socket is present and dialable.
func defaultDockerProbe(socket string) func() bool {
	return func() bool {
		if socket == "" {
			return false
		}
		c, err := net.DialTimeout("unix", socket, 200*time.Millisecond)
		if err != nil {
			return false
		}
		_ = c.Close()
		return true
	}
}

// readLine reads a single-line file and trims surrounding whitespace.
func readLine(root string, elem ...string) string {
	b, err := os.ReadFile(filepath.Join(append([]string{root}, elem...)...))
	if err != nil {
		return ""
	}
	return string(bytes.TrimSpace(b))
}

// readDistro extracts PRETTY_NAME from /etc/os-release.
func readDistro(root string) string {
	f, err := os.Open(filepath.Join(root, "etc", "os-release"))
	if err != nil {
		return ""
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		v, ok := strings.CutPrefix(sc.Text(), "PRETTY_NAME=")
		if !ok {
			continue
		}
		return strings.Trim(v, `"'`)
	}
	return ""
}
