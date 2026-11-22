// SPDX-License-Identifier: GPL-3.0-or-later
package tunnel

import (
	"strings"
	"testing"
)

func TestUnknownTransportIsRejected(t *testing.T) {
	_, err := New(Config{Transport: "quic"})
	if err == nil || !strings.Contains(err.Error(), "unknown transport") {
		t.Fatalf("New(quic) error = %v, want unknown transport", err)
	}
}

func TestUDPTransportDefaultsToTheWireGuardPort(t *testing.T) {
	cfg := Config{Transport: TransportUDP}
	if cfg.Port != 0 {
		t.Fatal("test setup: port should start unset")
	}
	// New() fills the port before it touches the keys, so the validation error is
	// about the missing server key while Port is already 51820.
	if _, err := New(cfg); err == nil || !strings.Contains(err.Error(), "server public key") {
		t.Fatalf("New(udp) error = %v, want a key error after the port default", err)
	}
}

func TestIPv6DefaultsMatchProton(t *testing.T) {
	if DefaultIPv6 != "2a07:b944::2:2" || DefaultDNS6 != "2a07:b944::2:1" {
		t.Fatalf("IPv6 defaults = %s/%s, want Proton's pair", DefaultIPv6, DefaultDNS6)
	}
}
