package accesslog

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
)

// forwardedTrusted says the server is reached only through a proxy of its own, so the address the proxy forwards is believed
// whatever connected. It is set when haas is told it is behind one (HAAS_BEHIND_PROXY): in a container the proxy never connects
// from the loopback, so everybody would otherwise be one source to the sign-in lock and the record.
var forwardedTrusted atomic.Bool

// TrustForwarded sets whether the address a proxy forwards is believed from any peer.
func TrustForwarded(on bool) { forwardedTrusted.Store(on) }

// Remote is the address of whoever made the request: the one place that says, for the log, the sign-in lock and the record.
//
// Behind nginx the direct peer is always loopback, and the client is in the
// headers nginx sets. From anywhere else those headers are whatever the caller
// chose to write, so they are believed only when the peer is loopback, or when haas was told it is only reached through a proxy.
func Remote(r *http.Request) string { return remote(r) }

func remote(r *http.Request) string {
	peer := peerAddress(r.RemoteAddr)
	if !peer.IsValid() {
		return r.RemoteAddr // a unix socket, say: whatever net/http was given
	}
	if forwarded, told := forwardedFor(r.Header); told && (peer.IsLoopback() || forwardedTrusted.Load()) {
		return forwarded.String()
	}
	return peer.String()
}

func peerAddress(remoteAddr string) netip.Addr {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return address.Unmap()
}

// forwardedFor prefers X-Real-IP, which nginx sets outright to the peer it saw.
// X-Forwarded-For is appended to, so a client can put anything at its start;
// only the last entry is the one nginx added.
func forwardedFor(header http.Header) (netip.Addr, bool) {
	if real, err := netip.ParseAddr(strings.TrimSpace(header.Get("X-Real-IP"))); err == nil {
		return real, true
	}
	chain := header.Values("X-Forwarded-For")
	if len(chain) == 0 {
		return netip.Addr{}, false
	}
	hops := strings.Split(chain[len(chain)-1], ",")
	last, err := netip.ParseAddr(strings.TrimSpace(hops[len(hops)-1]))
	return last, err == nil
}
