package config

import (
	"fmt"
	"net"
	"net/netip"
)

// The bind guard resolves the configured interface to an address and refuses
// to start if that address is unspecified, loopback, or outside the configured
// subnet. It is much harder to retrofit than to start with, so it goes in
// first (CLAUDE.md §4.1, docs/ROADMAP.md M1). The interface resolver is
// injected so the guard is exhaustively testable with a fake in a sandbox.

// InterfaceAddrs resolves an interface name to its addresses.
type InterfaceAddrs func(name string) ([]net.Addr, error)

// DefaultInterfaceAddrs resolves a real interface via the net package.
func DefaultInterfaceAddrs(name string) ([]net.Addr, error) {
	ifi, err := net.InterfaceByName(name)
	if err != nil {
		return nil, err
	}
	return ifi.Addrs()
}

// ResolveBindAddr returns the address the agent must bind to, or an error that
// explains exactly why the guard tripped. On error the agent must refuse to
// start — never fall back to a broader bind.
func ResolveBindAddr(c *Config, resolve InterfaceAddrs) (netip.Addr, error) {
	addrs, err := resolve(c.Interface)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("bind guard: cannot resolve interface %q: %w", c.Interface, err)
	}

	// Pick the interface address that lives inside the configured subnet.
	var candidate netip.Addr
	found := false
	for _, a := range addrs {
		ip := addrToNetip(a)
		if !ip.IsValid() {
			continue
		}
		if c.subnet.Contains(ip) {
			candidate = ip
			found = true
			break
		}
	}
	if !found {
		return netip.Addr{}, fmt.Errorf(
			"bind guard: interface %q has no address inside subnet %s — refusing to start",
			c.Interface, c.subnet)
	}

	// Belt and suspenders: an in-subnet address should never be unspecified or
	// loopback, but a pathological subnet (0.0.0.0/0, 127.0.0.0/8) could make
	// it so. Refuse regardless — 0.0.0.0 is the one bind we never do.
	if candidate.IsUnspecified() {
		return netip.Addr{}, fmt.Errorf(
			"bind guard: interface %q resolves to the unspecified address — refusing to bind to 0.0.0.0",
			c.Interface)
	}
	if candidate.IsLoopback() {
		return netip.Addr{}, fmt.Errorf(
			"bind guard: interface %q resolves to loopback address %s — refusing to start",
			c.Interface, candidate)
	}
	return candidate, nil
}

// addrToNetip extracts the IP from an interface address (typically *net.IPNet)
// as a netip.Addr, unmapping any IPv4-in-IPv6 form.
func addrToNetip(a net.Addr) netip.Addr {
	var ip net.IP
	switch v := a.(type) {
	case *net.IPNet:
		ip = v.IP
	case *net.IPAddr:
		ip = v.IP
	default:
		return netip.Addr{}
	}
	na, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}
	}
	return na.Unmap()
}
