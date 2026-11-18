package web

import (
	"net/http"
	"strings"
)

// cookieBase is the path a request reached this server's pages under, which the cookies set for it are kept to: the server's
// base (UseAddress), and none for an app's own host name, which is served at the root of that host.
func (s *Server) cookieBase(r *http.Request) string {
	if r.Header.Get(AppHeader) != "" {
		return ""
	}
	return s.base
}

// cookiePath is one of this server's paths as a cookie is kept to it, under base.
func cookiePath(base, own string) string {
	if base == "" {
		return own
	}
	return strings.TrimSuffix(base+own, "/")
}

// lockedName is what a cookie of this server's is called over TLS, where a browser takes it only from this server: a __Host-
// cookie at the root of a host, which no sibling host name can set, and a __Secure- one under a path, since a browser takes a
// __Host- cookie only for the whole host.
func lockedName(r *http.Request, base, name string) string {
	switch {
	case !overTLS(r):
		return name
	case base == "":
		return "__Host-" + name
	default:
		return "__Secure-" + name
	}
}
