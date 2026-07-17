// Package config loads and strictly validates the agent's TOML configuration,
// and hosts the bind guard — the single most important line of code in the
// project (CLAUDE.md §4.1). Unknown keys are errors: a typo in a security
// setting must fail loudly, not be silently ignored.
package config

import (
	"fmt"
	"net/netip"
	"path"
	"time"

	"github.com/BurntSushi/toml"
)

// Defaults applied when a field is omitted.
const (
	DefaultListenPort     = 8443
	DefaultSampleInterval = time.Second
	DefaultDockerSocket   = "/var/run/docker.sock"
)

// Config is the agent's validated configuration. The exported fields are
// decoded from TOML; the unexported ones are derived and validated by Load.
type Config struct {
	// Interface is the WireGuard interface the agent binds to, e.g. "wg0".
	Interface string `toml:"interface"`
	// SubnetCIDR is the WireGuard subnet in CIDR form, e.g. "10.0.0.0/24". The
	// bind guard refuses any address outside it. Access the parsed form via
	// the Subnet method.
	SubnetCIDR string `toml:"subnet"`
	// ListenPort is the TCP port served on the interface address.
	ListenPort int `toml:"listen_port"`
	// Peers is the source-IP allowlist. Only these IPs are served.
	Peers []string `toml:"peers"`
	// UnitAllowlist bounds write actions (start/stop/restart) by glob.
	UnitAllowlist []string `toml:"unit_allowlist"`
	// ReadAllowlist bounds read scope by glob. Empty means "read anything".
	ReadAllowlist []string `toml:"read_allowlist"`
	// SampleInterval is the aggregate metric tick.
	SampleInterval Duration `toml:"sample_interval"`
	// DockerSocket is the path to the Docker unix socket (M3).
	DockerSocket string `toml:"docker_socket"`

	subnet netip.Prefix
	peers  map[netip.Addr]struct{}
}

// Load reads and validates the TOML file at path. It rejects unknown keys and
// every semantic error it can catch before the agent opens a socket.
func Load(path string) (*Config, error) {
	var c Config
	md, err := toml.DecodeFile(path, &c)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return nil, fmt.Errorf("config: unknown keys: %v", undecoded)
	}
	if err := c.finalize(); err != nil {
		return nil, err
	}
	return &c, nil
}

// finalize applies defaults, parses derived values, and validates.
func (c *Config) finalize() error {
	if c.ListenPort == 0 {
		c.ListenPort = DefaultListenPort
	}
	if c.SampleInterval == 0 {
		c.SampleInterval = Duration(DefaultSampleInterval)
	}
	if c.DockerSocket == "" {
		c.DockerSocket = DefaultDockerSocket
	}

	if c.Interface == "" {
		return fmt.Errorf("config: interface is required")
	}
	if c.ListenPort < 1 || c.ListenPort > 65535 {
		return fmt.Errorf("config: listen_port %d out of range", c.ListenPort)
	}
	if c.SampleInterval.Std() <= 0 {
		return fmt.Errorf("config: sample_interval must be positive")
	}

	if c.SubnetCIDR == "" {
		return fmt.Errorf("config: subnet is required")
	}
	prefix, err := netip.ParsePrefix(c.SubnetCIDR)
	if err != nil {
		return fmt.Errorf("config: invalid subnet %q: %w", c.SubnetCIDR, err)
	}
	c.subnet = prefix.Masked()

	if len(c.Peers) == 0 {
		return fmt.Errorf("config: at least one peer is required")
	}
	c.peers = make(map[netip.Addr]struct{}, len(c.Peers))
	for _, p := range c.Peers {
		addr, err := netip.ParseAddr(p)
		if err != nil {
			return fmt.Errorf("config: invalid peer %q: %w", p, err)
		}
		c.peers[addr] = struct{}{}
	}

	for _, g := range c.UnitAllowlist {
		if _, err := path.Match(g, ""); err != nil {
			return fmt.Errorf("config: invalid unit_allowlist glob %q: %w", g, err)
		}
	}
	for _, g := range c.ReadAllowlist {
		if _, err := path.Match(g, ""); err != nil {
			return fmt.Errorf("config: invalid read_allowlist glob %q: %w", g, err)
		}
	}
	return nil
}

// Subnet returns the masked WireGuard subnet.
func (c *Config) Subnet() netip.Prefix { return c.subnet }

// PeerAllowed reports whether addr is in the peer allowlist.
func (c *Config) PeerAllowed(addr netip.Addr) bool {
	_, ok := c.peers[addr.Unmap()]
	return ok
}

// UnitWritable reports whether write actions may target unit. Default deny:
// only units matching a unit_allowlist glob are writable.
func (c *Config) UnitWritable(unit string) bool {
	return matchAny(c.UnitAllowlist, unit)
}

// UnitReadable reports whether unit may be read. An empty read_allowlist means
// "read anything" — read scope may be wider than write scope (CLAUDE.md §4.3).
func (c *Config) UnitReadable(unit string) bool {
	if len(c.ReadAllowlist) == 0 {
		return true
	}
	return matchAny(c.ReadAllowlist, unit)
}

func matchAny(globs []string, s string) bool {
	for _, g := range globs {
		if ok, _ := path.Match(g, s); ok {
			return true
		}
	}
	return false
}
