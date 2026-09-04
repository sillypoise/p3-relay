package netguard

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"
)

const maximum_resolved_addresses = 16

type Dialer struct {
	resolver      *net.Resolver
	network       *net.Dialer
	allow_private bool
}

func NewDialer(allow_private bool, timeout time.Duration) *Dialer {
	if timeout <= 0 {
		panic("positive dial timeout is required")
	}
	return &Dialer{
		resolver: net.DefaultResolver,
		network: &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second},
		allow_private: allow_private,
	}
}

func (dialer *Dialer) DialContext(context_value context.Context, network string, address string) (net.Conn, error) {
	host, port, error_value := net.SplitHostPort(address)
	if error_value != nil {
		return nil, fmt.Errorf("split destination address: %w", error_value)
	}
	addresses, error_value := dialer.resolver.LookupNetIP(context_value, "ip", host)
	if error_value != nil {
		return nil, fmt.Errorf("resolve destination: %w", error_value)
	}
	if len(addresses) == 0 || len(addresses) > maximum_resolved_addresses {
		return nil, errors.New("destination resolved to an invalid address count")
	}
	for _, address_value := range addresses {
		if !dialer.allowed(address_value) {
			return nil, errors.New("destination resolved to a prohibited address")
		}
	}

	var dial_errors []error
	for _, address_value := range addresses {
		connection, dial_error := dialer.network.DialContext(
			context_value,
			network,
			net.JoinHostPort(address_value.String(), port),
		)
		if dial_error == nil {
			return connection, nil
		}
		dial_errors = append(dial_errors, dial_error)
	}
	return nil, errors.Join(dial_errors...)
}

func (dialer *Dialer) allowed(address netip.Addr) bool {
	if dialer.allow_private {
		return true
	}
	if !address.IsValid() || !address.IsGlobalUnicast() {
		return false
	}
	if address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() {
		return false
	}
	carrier_nat := netip.MustParsePrefix("100.64.0.0/10")
	if carrier_nat.Contains(address) {
		return false
	}
	benchmark := netip.MustParsePrefix("198.18.0.0/15")
	return !benchmark.Contains(address)
}
