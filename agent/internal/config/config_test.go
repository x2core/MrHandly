package config

import (
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "agent.toml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

const validConfig = `
interface = "wg0"
subnet = "10.44.0.0/24"
listen_port = 8443
peers = ["10.44.0.1", "10.44.0.2"]
unit_allowlist = ["nginx.service", "docker.service"]
read_allowlist = ["*.service"]
sample_interval = "2s"
`

func TestLoadValid(t *testing.T) {
	c, err := Load(writeConfig(t, validConfig))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Interface != "wg0" {
		t.Errorf("Interface = %q", c.Interface)
	}
	if c.SampleInterval.Std() != 2*time.Second {
		t.Errorf("SampleInterval = %v", c.SampleInterval)
	}
	if c.Subnet().String() != "10.44.0.0/24" {
		t.Errorf("Subnet = %s", c.Subnet())
	}
	if !c.PeerAllowed(netip.MustParseAddr("10.44.0.1")) {
		t.Error("10.44.0.1 should be allowed")
	}
	if c.PeerAllowed(netip.MustParseAddr("10.44.0.9")) {
		t.Error("10.44.0.9 should not be allowed")
	}
}

func TestLoadDefaults(t *testing.T) {
	c, err := Load(writeConfig(t, `
interface = "wg0"
subnet = "10.44.0.0/24"
peers = ["10.44.0.1"]
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.ListenPort != DefaultListenPort {
		t.Errorf("ListenPort = %d, want default %d", c.ListenPort, DefaultListenPort)
	}
	if c.SampleInterval.Std() != DefaultSampleInterval {
		t.Errorf("SampleInterval = %v, want default", c.SampleInterval)
	}
	if c.DockerSocket != DefaultDockerSocket {
		t.Errorf("DockerSocket = %q", c.DockerSocket)
	}
}

func TestLoadRejects(t *testing.T) {
	cases := map[string]string{
		"unknown key": `
interface = "wg0"
subnet = "10.44.0.0/24"
peers = ["10.44.0.1"]
listne_port = 9000
`,
		"missing interface": `
subnet = "10.44.0.0/24"
peers = ["10.44.0.1"]
`,
		"missing subnet": `
interface = "wg0"
peers = ["10.44.0.1"]
`,
		"bad subnet": `
interface = "wg0"
subnet = "not-a-cidr"
peers = ["10.44.0.1"]
`,
		"no peers": `
interface = "wg0"
subnet = "10.44.0.0/24"
peers = []
`,
		"bad peer": `
interface = "wg0"
subnet = "10.44.0.0/24"
peers = ["not-an-ip"]
`,
		"bad sample interval": `
interface = "wg0"
subnet = "10.44.0.0/24"
peers = ["10.44.0.1"]
sample_interval = "nonsense"
`,
		"bad glob": `
interface = "wg0"
subnet = "10.44.0.0/24"
peers = ["10.44.0.1"]
unit_allowlist = ["a[b"]
`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, body)); err == nil {
				t.Errorf("expected error for %q, got nil", name)
			}
		})
	}
}

func TestUnitAllowlist(t *testing.T) {
	c, err := Load(writeConfig(t, validConfig))
	if err != nil {
		t.Fatal(err)
	}
	if !c.UnitWritable("nginx.service") {
		t.Error("nginx.service should be writable")
	}
	if c.UnitWritable("sshd.service") {
		t.Error("sshd.service should not be writable")
	}
	// read_allowlist = ["*.service"]
	if !c.UnitReadable("sshd.service") {
		t.Error("sshd.service should be readable")
	}
	if c.UnitReadable("cron.timer") {
		t.Error("cron.timer should not be readable")
	}
}

func TestUnitReadableDefaultOpen(t *testing.T) {
	c, err := Load(writeConfig(t, `
interface = "wg0"
subnet = "10.44.0.0/24"
peers = ["10.44.0.1"]
`))
	if err != nil {
		t.Fatal(err)
	}
	// Empty read_allowlist => read anything.
	if !c.UnitReadable("anything.service") {
		t.Error("empty read_allowlist should allow all reads")
	}
	// But write remains default-deny.
	if c.UnitWritable("anything.service") {
		t.Error("empty unit_allowlist should deny all writes")
	}
}
