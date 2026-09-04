package netguard

import (
	"net/netip"
	"testing"
	"time"
)

func TestAllowedRejectsNonPublicAddresses(t *testing.T) {
	dialer := NewDialer(false, time.Second)
	prohibited := []string{
		"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1",
		"198.18.0.1", "::1", "fe80::1", "fc00::1",
	}
	for _, raw := range prohibited {
		if dialer.allowed(netip.MustParseAddr(raw)) {
			t.Fatalf("prohibited address %s was allowed", raw)
		}
	}
	if !dialer.allowed(netip.MustParseAddr("1.1.1.1")) {
		t.Fatal("public address was rejected")
	}
}

func TestAllowedPrivateOverrideIsExplicit(t *testing.T) {
	dialer := NewDialer(true, time.Second)
	if !dialer.allowed(netip.MustParseAddr("127.0.0.1")) {
		t.Fatal("development private-address override was not honored")
	}
}
