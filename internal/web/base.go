package web

import (
	"html"
	"net/url"
	"strings"
)

// UseAddress tells the server the address people reach it at. Its path is where the server's pages are, which is the root of a
// host unless a proxy in front serves it under a path, as gusto cloud serves each organization at /workspace/<slug>. The proxy
// asks the server for the page below that path and puts the path in front of the server's redirects; the server puts it in the
// addresses it writes into pages, and keeps its cookies to it.
func (s *Server) UseAddress(public string) error {
	if public == "" {
		return nil
	}
	parsed, err := url.Parse(public)
	if err != nil {
		return err
	}
	s.base = strings.TrimRight(parsed.EscapedPath(), "/")
	return nil
}

// baseHead tells the app and the scripts of a page the path the server's pages are under, when it is not the root.
func baseHead(base string) string {
	if base == "" {
		return ""
	}
	return `<meta name="gusto-base" content="` + html.EscapeString(base) + `">`
}
