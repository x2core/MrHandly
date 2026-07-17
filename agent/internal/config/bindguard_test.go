package config

import (
	"errors"
	"net"
	"strings"
	"testing"
)

// fakeResolver returns a canned address list (and optional error) for any
// interface, so the bind guard is testable with no real network interfaces.
func fakeResolver(cidrs []string, err error) InterfaceAddrs {
	return func(string) ([]net.Addr, error) {
		if err != nil {
			return nil, err
		}
		addrs := make([]net.Addr, 0, len(cidrs))
		for _, c := range cidrs {
			ip, ipnet, perr := net.ParseCIDR(c)
			if perr != nil {
				panic(perr)
			}
			ipnet.IP = ip
			addrs = append(addrs, ipnet)
		}
		return addrs, nil
	}
}

// cfg builds a minimal validated Config for the given subnet.
func cfg(t *testing.T, subnet string) *Config {
	t.Helper()
	c := &Config{Interface: "wg0", SubnetCIDR: subnet, Peers: []string{"10.0.0.9"}}
	if err := c.finalize(); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	return c
}

func TestResolveBindAddr(t *testing.T) {
	tests := []struct {
		name     string
		subnet   string
		addrs    []string
		resolveErr error
		wantAddr string // empty => expect error
		wantErr  string // substring the error must contain
	}{
		{
			name:     "valid in-subnet address",
			subnet:   "10.44.0.0/24",
			addrs:    []string{"10.44.0.5/24"},
			wantAddr: "10.44.0.5",
		},
		{
			name:    "picks in-subnet from several",
			subnet:  "10.44.0.0/24",
			addrs:   []string{"192.168.1.10/24", "10.44.0.7/24", "172.16.0.1/16"},
			wantAddr: "10.44.0.7",
		},
		{
			name:    "outside subnet refused",
			subnet:  "10.44.0.0/24",
			addrs:   []string{"192.168.1.10/24"},
			wantErr: "no address inside subnet",
		},
		{
			name:    "unspecified refused",
			subnet:  "0.0.0.0/0",
			addrs:   []string{"0.0.0.0/0"},
			wantErr: "unspecified",
		},
		{
			name:    "loopback refused",
			subnet:  "127.0.0.0/8",
			addrs:   []string{"127.0.0.1/8"},
			wantErr: "loopback",
		},
		{
			name:       "interface lookup failure refused",
			subnet:     "10.44.0.0/24",
			resolveErr: errors.New("no such device"),
			wantErr:    "cannot resolve interface",
		},
		{
			name:    "no addresses refused",
			subnet:  "10.44.0.0/24",
			addrs:   nil,
			wantErr: "no address inside subnet",
		},
		{
			name:     "ipv6 in-subnet address",
			subnet:   "fd00::/64",
			addrs:    []string{"fd00::5/64"},
			wantAddr: "fd00::5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := cfg(t, tt.subnet)
			addr, err := ResolveBindAddr(c, fakeResolver(tt.addrs, tt.resolveErr))
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got addr %s", tt.wantErr, addr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %q, want substring %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if addr.String() != tt.wantAddr {
				t.Fatalf("addr = %s, want %s", addr, tt.wantAddr)
			}
		})
	}
}
