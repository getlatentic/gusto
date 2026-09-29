package cloud

import (
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/getlatentic/haas/internal/entry"
)

// provision starts an organisation's server, without making the person wait for it.
func (s *Server) provision(slug string) {
	if s.Controller == nil {
		return
	}
	s.making.Add(1)
	go func() {
		defer s.making.Done()
		if err := s.Controller.Provision(slug); err != nil {
			slog.Error("an organisation's server could not be started", "org", slug, "error", err)
		}
	}()
}

// orgSlug is the organisation a host name is, when it is <slug>.<domain>. The front
// door's own name is the domain itself, and is not one.
func (s *Server) orgSlug(host string) (string, bool) {
	if s.Controller == nil {
		return "", false
	}
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	slug, found := strings.CutSuffix(strings.ToLower(host), "."+s.Domain)
	if !found || strings.Contains(slug, ".") || ValidSlug(slug) != nil {
		return "", false
	}
	return slug, true
}

// orgURL is the address of an organisation's server as a browser reaches it.
func (s *Server) orgURL(slug string) string {
	host := slug + "." + s.Domain
	if port := s.URL.Port(); port != "" {
		host += ":" + port
	}
	return s.URL.Scheme + "://" + host
}

// route sends a request to the organisation's server, or says why it cannot.
func (s *Server) route(w http.ResponseWriter, r *http.Request, slug string) {
	address, state := s.Controller.Route(slug)
	if state == "" {
		if _, found, _ := s.Store.Org(slug); !found {
			http.NotFound(w, r)
			return
		}
		state = Provisioning
	}
	if address == "" {
		w.Header().Set("Retry-After", "3")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`<!doctype html><meta charset="utf-8"><meta http-equiv="refresh" content="3"><title>haas</title><p>` + stateWords(state) + `</p>`))
		return
	}
	(&httputil.ReverseProxy{
		Rewrite:       s.rewrite(address),
		FlushInterval: -1,
		Transport:     &http.Transport{ResponseHeaderTimeout: 0, IdleConnTimeout: 90 * time.Second, MaxIdleConnsPerHost: 8},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Warn("an organisation's server did not answer", "org", slug, "error", err)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte(`<!doctype html><meta charset="utf-8"><meta http-equiv="refresh" content="3"><title>haas</title><p>This organisation's server is not answering. It is being started again.</p>`))
		},
	}).ServeHTTP(w, r)
}

// rewrite sends a request on to the address as the front door saw it: the client
// is who the front door says, and none of the front door's own cookies go. What the
// client claimed about itself is not believed, since the organisation's server
// writes down where a person signed in from.
func (s *Server) rewrite(address string) func(*httputil.ProxyRequest) {
	target := &url.URL{Scheme: "http", Host: address}
	scheme := "http"
	if s.Sessions.secure {
		scheme = "https"
	}
	return func(pr *httputil.ProxyRequest) {
		pr.SetURL(target)
		pr.Out.Host = pr.In.Host
		pr.Out.Header.Set("X-Forwarded-For", s.source(pr.In))
		pr.Out.Header.Set("X-Forwarded-Proto", scheme)
		pr.Out.Header.Set("X-Forwarded-Host", pr.In.Host)
		pr.Out.Header.Del("Cookie")
		for _, cookie := range pr.In.Cookies() {
			if cookie.Name != "haas_cloud" && cookie.Name != "__Host-haas_cloud" {
				pr.Out.AddCookie(cookie)
			}
		}
	}
}

// open signs a person in to their organisation's server, with a token that server
// checks, and sends their browser there.
func (s *Server) open(w http.ResponseWriter, r *http.Request, person Person) {
	if s.Controller == nil {
		http.NotFound(w, r)
		return
	}
	if _, state := s.Controller.Route(person.Org); state != Ready {
		http.Redirect(w, r, "/org", http.StatusSeeOther)
		return
	}
	key, err := s.Controller.EntryKey(person.Org)
	if err != nil {
		slog.Error("an organisation's entry key could not be read", "org", person.Org, "error", err)
		http.Redirect(w, r, "/org", http.StatusSeeOther)
		return
	}
	token, err := entry.Mint(key, person.Org, person.Email, string(person.Role), time.Now())
	if err != nil {
		http.Redirect(w, r, "/org", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, s.orgURL(person.Org)+"/auth/entry?t="+url.QueryEscape(token), http.StatusSeeOther)
}

// stateWords is what a person is told of a server that is not ready.
func stateWords(state State) string {
	switch state {
	case Failed:
		return "This organisation's server could not be started. It is being tried again."
	case Starting:
		return "This organisation's server is starting."
	}
	return "This organisation is being set up."
}
