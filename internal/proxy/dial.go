package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"time"
)

// Resolver is how a name becomes addresses; a test gives its own.
type Resolver func(ctx context.Context, host string) ([]netip.Addr, error)

func systemResolver(ctx context.Context, host string) ([]netip.Addr, error) {
	return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
}

// dial connects a job to a host its allowlist names, and to no address the
// allowlist could not have meant. A name is resolved once, here, and the
// connection is made to an address from that answer, so what was checked is
// what is connected to.
//
// A name that resolves to this machine, or to a link-local address such as a
// cloud's metadata service at 169.254.169.254, is refused: an allowed domain
// whose records point inward, or a name whose record changes between the check
// and the connection, must not turn an allowlist into a way into the host. An
// address written as a literal is what the operator named, so a literal
// loopback address is theirs to allow; a link-local one never is.
func (p *Proxy) dial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	var candidates []netip.Addr
	if literal, err := netip.ParseAddr(host); err == nil {
		candidates = []netip.Addr{literal.Unmap()}
	} else {
		found, err := p.lookup(ctx, host)
		if err != nil {
			return nil, err
		}
		candidates = found
	}
	_, isLiteral := parsedLiteral(host)
	dialer := net.Dialer{Timeout: dialLimit}
	var last error = errors.New("that name resolves to no address")
	for _, candidate := range candidates {
		candidate = candidate.Unmap()
		if refused := unreachable(candidate, isLiteral); refused != "" {
			last = fmt.Errorf("%s resolves to %s, %s", host, candidate, refused)
			continue
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(candidate.String(), port))
		if err == nil {
			return conn, nil
		}
		last = err
	}
	return nil, last
}

func (p *Proxy) lookup(ctx context.Context, host string) ([]netip.Addr, error) {
	resolve := p.resolve
	if resolve == nil {
		resolve = systemResolver
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return resolve(ctx, host)
}

func parsedLiteral(host string) (netip.Addr, bool) {
	addr, err := netip.ParseAddr(host)
	return addr, err == nil
}

// unreachable says why a job may not connect to address, or nothing.
func unreachable(a netip.Addr, named bool) string {
	switch {
	case a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast():
		return "a link-local address, where a cloud's metadata service lives"
	case a.IsMulticast() || a.IsUnspecified():
		return "not an address a host answers on"
	case a.IsLoopback() && !named:
		return "this machine"
	}
	return ""
}
