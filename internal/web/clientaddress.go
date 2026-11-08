package web

import (
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/getlatentic/gusto/internal/accesslog"
)

// clientOf is where a request came from, for the change record and the sign-in locks. Behind gusto cloud the front door is the
// peer and says who it saw in X-Forwarded-For, which it sets outright, so that address is believed from a peer on a private
// network, the only kind of caller a server that is reached through its front door has. Anywhere else it is what the access log
// says (accesslog.Remote): a forwarded address is believed from a proxy on this machine, and from any peer when gusto was told it
// is only reached through one.
func (s *Server) clientOf(r *http.Request) string {
	if s.front != nil {
		if client, ok := forwardedByFrontDoor(r); ok {
			return client
		}
	}
	return accesslog.Remote(r)
}

// forwardedByFrontDoor is the address the front door wrote, when the request came from a private address. The last entry is the
// one a proxy added.
func forwardedByFrontDoor(r *http.Request) (string, bool) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return "", false
	}
	peer, err := netip.ParseAddr(host)
	if err != nil || !(peer.IsPrivate() || peer.IsLoopback() || peer.IsLinkLocalUnicast()) {
		return "", false
	}
	chain := r.Header.Values("X-Forwarded-For")
	if len(chain) == 0 {
		return "", false
	}
	hops := strings.Split(chain[len(chain)-1], ",")
	client, err := netip.ParseAddr(strings.TrimSpace(hops[len(hops)-1]))
	if err != nil {
		return "", false
	}
	return client.Unmap().String(), true
}
