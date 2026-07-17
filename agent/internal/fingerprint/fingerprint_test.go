package fingerprint

import "testing"

const fixtureRoot = "../../testdata/host-a"

func TestDetectHost(t *testing.T) {
	fp := Detect(Options{
		Root:    fixtureRoot,
		Arch:    "amd64",
		Systemd: func() bool { return true },
		Docker:  func() bool { return false },
	})
	h := fp.Host()
	if h.Hostname != "lab-02" {
		t.Errorf("Hostname = %q, want lab-02", h.Hostname)
	}
	if h.Kernel != "6.1.0-18-amd64" {
		t.Errorf("Kernel = %q", h.Kernel)
	}
	if h.Distro != "Debian GNU/Linux 12 (bookworm)" {
		t.Errorf("Distro = %q", h.Distro)
	}
	if h.Arch != "amd64" {
		t.Errorf("Arch = %q", h.Arch)
	}
	if h.CPUs != 2 {
		t.Errorf("CPUs = %d, want 2", h.CPUs)
	}
	if h.TotalMemory != 16384000*1024 {
		t.Errorf("TotalMemory = %d", h.TotalMemory)
	}
	if h.BootTime != 1700000000 {
		t.Errorf("BootTime = %d, want 1700000000", h.BootTime)
	}
}

func TestInfoReprobesDocker(t *testing.T) {
	calls := 0
	present := false
	fp := Detect(Options{
		Root:    fixtureRoot,
		Arch:    "arm64",
		Systemd: func() bool { return true },
		Docker: func() bool {
			calls++
			return present
		},
	})
	// Docker is lazy: Detect must not probe it (the socket may appear later).
	if calls != 0 {
		t.Fatalf("docker probe called %d times during Detect, want 0 (lazy)", calls)
	}

	present = true // socket appears while the agent runs
	info := fp.Info("v1.2.3", "abc123")
	if calls != 1 {
		t.Fatalf("docker probe called %d times, want 1 per Info", calls)
	}
	if info.Protocol != 1 {
		t.Errorf("Protocol = %d, want 1", info.Protocol)
	}
	if info.Agent.Version != "v1.2.3" || info.Agent.Commit != "abc123" {
		t.Errorf("Agent = %+v", info.Agent)
	}
	if !info.Capabilities.Systemd {
		t.Error("systemd should be true")
	}
	// Docker is re-probed by Info; second probe returns true.
	if !info.Capabilities.Docker {
		t.Error("docker should be re-probed as true on Info call")
	}
}

func TestDefaultSystemdProbe(t *testing.T) {
	// The fixture has proc/... but a real systemd runtime dir only under
	// run/systemd/system.
	if !defaultSystemdProbe(fixtureRoot)() {
		t.Error("expected systemd probe to detect run/systemd/system fixture")
	}
	if defaultSystemdProbe("/no/such/root")() {
		t.Error("expected systemd probe to be false for missing root")
	}
}

func TestDefaultDockerProbeAbsent(t *testing.T) {
	if defaultDockerProbe("")() {
		t.Error("empty socket path must probe false")
	}
	if defaultDockerProbe("/no/such/docker.sock")() {
		t.Error("missing socket must probe false")
	}
}
