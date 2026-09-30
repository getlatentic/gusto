package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"
)

// clientFor is the client a call to this credential's model goes out with. An address somebody
// chose for their organisation is reached only at a public address, and everything else with the
// client every credential has had.
func (p *Proxy) clientFor(credential Credential) *http.Client {
	if !credential.Public {
		return p.client
	}
	return p.public
}

// publicClient is a client that connects only to a public address. A name is resolved once, here, and
// the connection is made to an address from that answer, so what was checked is what is connected to and
// a name whose record changes between the two is no way in.
func publicClient(resolve Resolver) *http.Client {
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		var candidates []netip.Addr
		if literal, err := netip.ParseAddr(host); err == nil {
			candidates = []netip.Addr{literal}
		} else if candidates, err = resolve(ctx, host); err != nil {
			return nil, err
		}
		last := errors.New("that name resolves to no address")
		dialer := net.Dialer{Timeout: dialLimit}
		for _, candidate := range candidates {
			candidate = candidate.Unmap()
			if !Public(candidate) {
				last = fmt.Errorf("%s resolves to %s, which is not a public address", host, candidate)
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
	return &http.Client{
		Transport: &http.Transport{
			DialContext:         dial,
			TLSHandshakeTimeout: 10 * time.Second,
			IdleConnTimeout:     90 * time.Second,
			MaxIdleConnsPerHost: 4,
			ForceAttemptHTTP2:   true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// carrierGradeNAT is 100.64.0.0/10, which netip does not count as private.
var carrierGradeNAT = netip.MustParsePrefix("100.64.0.0/10")

// Public says whether an address is one on the open internet: not this machine, not a private
// network's, not link-local, not one that is not an address a host answers on.
func Public(a netip.Addr) bool {
	return a.IsValid() && a.IsGlobalUnicast() && !a.IsPrivate() && !a.IsLoopback() && !a.IsLinkLocalUnicast() &&
		!carrierGradeNAT.Contains(a)
}
