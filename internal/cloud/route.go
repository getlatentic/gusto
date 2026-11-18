package cloud

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/apps"
	"github.com/getlatentic/gusto/internal/entry"
	"github.com/getlatentic/gusto/internal/team"
	"github.com/getlatentic/gusto/internal/web"
)

// provision starts an organization's server, without making the person wait for it.
func (s *Server) provision(ctx context.Context, slug string) {
	if s.Controller == nil {
		return
	}
	ctx = context.WithoutCancel(ctx)
	s.making.Add(1)
	go func() {
		defer s.making.Done()
		if err := s.Controller.Provision(ctx, slug); err != nil && !errors.Is(err, ErrRemoved) {
			slog.Error("an organization's server could not be started", "org", slug, "error", err)
		}
	}()
}

// orgSlug is the organization a host name is, when it is <slug>.<domain>. The front
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

// appHost is the app a host name is, when it is <slug>--<id>.<domain>: an app a person
// shared from an agent of the organization, at a name of its own so that the browser keeps it
// apart from the organization's pages. An organization's address never has two hyphens in a row,
// so the two are never mistaken.
func (s *Server) appHost(host string) (slug, id string, ok bool) {
	if s.Controller == nil {
		return "", "", false
	}
	if name, _, err := net.SplitHostPort(host); err == nil {
		host = name
	}
	label, found := strings.CutSuffix(strings.ToLower(host), "."+s.AppsDomain)
	if !found || strings.Contains(label, ".") {
		return "", "", false
	}
	slug, id, found = strings.Cut(label, "--")
	if !found || ValidSlug(slug) != nil || !apps.Valid(id) {
		return "", "", false
	}
	return slug, id, true
}

// appURL is the address of a shared app as a browser reaches it.
func (s *Server) appURL(slug, id string) string {
	apps, _ := url.Parse(AppsAddress(s.URL, s.AppsDomain))
	return apps.Scheme + "://" + slug + "--" + id + "." + apps.Host
}

// AppsAddress is where shared apps are, with no app's name in front: the domain they are served under, with gusto cloud's
// own scheme and port.
func AppsAddress(public *url.URL, appsDomain string) string {
	host := appsDomain
	if port := public.Port(); port != "" {
		host += ":" + port
	}
	return public.Scheme + "://" + host
}

// orgURL is the address of an organization's server as a browser reaches it.
func (s *Server) orgURL(slug string) string { return OrgAddress(s.URL, s.Domain, s.OrgPaths, slug) }

// OrgAddress is the address of an organization's server as a browser reaches it through a gusto cloud server at public: at
// /workspace/<slug> on public's own name when organizations are at paths, and at <slug>.<domain> otherwise.
func OrgAddress(public *url.URL, domain string, paths bool, slug string) string {
	if paths {
		return strings.TrimRight(public.String(), "/") + orgPath + slug
	}
	host := slug + "." + domain
	if port := public.Port(); port != "" {
		host += ":" + port
	}
	return public.Scheme + "://" + host
}

// route sends a request to the organization's server, or says why it cannot. prefix is the path the organization is reached
// at on gusto cloud's own name, and "" when it has a host name of its own.
func (s *Server) route(w http.ResponseWriter, r *http.Request, slug, prefix string) {
	s.forward(w, r, slug, "", prefix)
}

// routeApp sends a request for an app's own host name to the organization's server, signed as
// being for that app, which is all the request can then reach there.
func (s *Server) routeApp(w http.ResponseWriter, r *http.Request, slug, id string) {
	s.forward(w, r, slug, id, "")
}

// upstream carries every request to an organization's server, a room's WebSocket among them: the proxy passes an upgrade through
// and then copies bytes both ways, with no timeout of its own. It is shared, so the connections it keeps are reused.
var upstream = &http.Transport{ResponseHeaderTimeout: 0, IdleConnTimeout: 90 * time.Second, MaxIdleConnsPerHost: 8}

// forward sends a request to the organization's server, for one of its apps when an app is named, and fits the answer to the
// path the organization is reached at when there is one.
func (s *Server) forward(w http.ResponseWriter, r *http.Request, slug, app, prefix string) {
	address, state := s.Controller.Route(r.Context(), slug)
	if state == "" {
		if _, found, _ := s.Store.Org(r.Context(), slug); !found {
			if !s.demoEnded(r.Context(), w, slug) {
				http.NotFound(w, r)
			}
			return
		}
		state = Provisioning
	}
	if address == "" {
		w.Header().Set("Retry-After", "3")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`<!doctype html><meta charset="utf-8"><meta http-equiv="refresh" content="3"><title>gusto</title><p>` + stateWords(state) + `</p>`))
		return
	}
	var fitted func(*http.Response) error
	var person string
	if prefix != "" {
		fitted = mountedAnswer(prefix)
		person = s.vouch(r, slug)
	}
	(&httputil.ReverseProxy{
		Rewrite:        s.rewrite(address, s.appSignature(slug, app), person, prefix),
		ModifyResponse: fitted,
		FlushInterval:  -1,
		Transport:      upstream,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Warn("an organization's server did not answer", "org", slug, "error", err)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadGateway)
			w.Write([]byte(`<!doctype html><meta charset="utf-8"><meta http-equiv="refresh" content="3"><title>gusto</title><p>This organization's server is not answering. It is being started again.</p>`))
		},
	}).ServeHTTP(w, r)
}

// rewrite sends a request on to the address as gusto cloud saw it: the client
// is who gusto cloud says, and none of gusto cloud's own cookies go. What the
// client claimed about itself is not believed, since the organization's server
// writes down where a person signed in from. person is the token that says whose
// request it is, for an organization at a path, and "" for any other.
func (s *Server) rewrite(address, appSignature, person, prefix string) func(*httputil.ProxyRequest) {
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
		pr.Out.Header.Del("X-Forwarded-Prefix")
		if prefix != "" {
			pr.Out.Header.Set("X-Forwarded-Prefix", prefix)
		}
		// Which app a request is for is said by gusto cloud alone: whatever the client sent is
		// dropped, and a request that is not for an app carries none.
		pr.Out.Header.Del(web.AppHeader)
		if appSignature != "" {
			pr.Out.Header.Set(web.AppHeader, appSignature)
		}
		pr.Out.Header.Del(web.PersonHeader)
		if person != "" {
			pr.Out.Header.Set(web.PersonHeader, person)
		}
		pr.Out.Header.Del("Cookie")
		for _, cookie := range pr.In.Cookies() {
			if !ownCookie(cookie.Name) {
				pr.Out.AddCookie(cookie)
			}
		}
	}
}

// appSignature is the signed header value for an app of an organization, or nothing when no
// app is named or the organization's key cannot be read.
func (s *Server) appSignature(slug, id string) string {
	if id == "" {
		return ""
	}
	key, err := s.Controller.EntryKey(slug)
	if err != nil {
		slog.Error("an organization's entry key could not be read", "org", slug, "error", err)
		return ""
	}
	return web.SignApp(key, id)
}

// open takes a person into their organization's server and sends their browser there. At a path of gusto cloud's own name
// the organization shares gusto cloud's sign-in, so the browser needs nothing more. At a host name of the organization's
// own, it carries a token that server checks and signs the person in with. An app is always at a host name of its own, a
// different origin from the organization's.
func (s *Server) open(w http.ResponseWriter, r *http.Request, person Person) {
	if s.Controller == nil {
		http.NotFound(w, r)
		return
	}
	if _, state := s.Controller.Route(r.Context(), person.Org); state != Ready {
		s.starting(w, r, person, state)
		return
	}
	if id := r.URL.Query().Get("app"); id != "" {
		s.openApp(w, r, person, id)
		return
	}
	into := url.Values{}
	if !s.OrgPaths {
		token, ok := s.entryToken(w, r, person)
		if !ok {
			return
		}
		into.Set("t", token)
	}
	if err := s.Store.RecordSignIn(r.Context(), person.Email, person.Org); err != nil {
		slog.Error("a sign-in to an organization was not recorded", "org", person.Org, "error", err)
	}
	if space := r.URL.Query().Get("team"); team.Valid(space) == nil {
		into.Set("team", space)
	}
	http.Redirect(w, r, s.orgURL(person.Org)+"/auth/entry?"+into.Encode(), http.StatusSeeOther)
}

// openApp sends a person to an app of their organization's, signed in on its own host name.
func (s *Server) openApp(w http.ResponseWriter, r *http.Request, person Person, id string) {
	if !apps.Valid(id) {
		http.NotFound(w, r)
		return
	}
	token, ok := s.entryToken(w, r, person)
	if !ok {
		return
	}
	http.Redirect(w, r, s.appURL(person.Org, id)+"/auth/entry?t="+url.QueryEscape(token), http.StatusSeeOther)
}

// entryToken is a token that signs a person in to their organization's server once. When none can be made, the person is
// sent to their organization's page here.
func (s *Server) entryToken(w http.ResponseWriter, r *http.Request, person Person) (string, bool) {
	key, err := s.Controller.EntryKey(person.Org)
	if err == nil {
		var token string
		if token, err = entry.Mint(key, person.Org, person.Email, string(person.Role), time.Now()); err == nil {
			return token, true
		}
	}
	slog.Error("a token to enter an organization with could not be made", "org", person.Org, "error", err)
	http.Redirect(w, r, "/org", http.StatusSeeOther)
	return "", false
}

// vouch is the token that says whose a request to an organization's pages at a path is: the person signed in to gusto
// cloud, whose sign-in an organization at a path shares. What they may do there is the organization's server's to decide.
// It is "" for a request with no sign-in, which the organization's server sends to sign in.
func (s *Server) vouch(r *http.Request, slug string) string {
	email, ok := s.Sessions.Who(r)
	if !ok {
		return ""
	}
	key, err := s.Controller.EntryKey(slug)
	if err != nil {
		slog.Error("an organization's entry key could not be read", "org", slug, "error", err)
		return ""
	}
	token, err := entry.Vouch(key, slug, email, time.Now())
	if err != nil {
		slog.Error("a request could not be vouched for", "org", slug, "error", err)
		return ""
	}
	return token
}

// starting is what a person going in sees while their organization's server is not ready: what it is doing, on a
// page that looks again until it can let them in. A demo has its own page for this.
func (s *Server) starting(w http.ResponseWriter, r *http.Request, person Person, state State) {
	if _, _, found := s.Store.DemoLeft(r.Context(), person.Org); found {
		http.Redirect(w, r, "/demo/wait", http.StatusSeeOther)
		return
	}
	org, _, err := s.Store.Org(r.Context(), person.Org)
	if err != nil {
		slog.Error("an organization's record could not be read", "org", person.Org, "error", err)
	}
	watched := &watch{State: state}
	if state == "" {
		state = Provisioning
	}
	s.render(w, http.StatusOK, "starting", view{Title: cmp.Or(org.Name, person.Org), Org: org, Watch: watched, Notice: stateWords(state)})
}

// stateWords is what a person is told of a server that is not ready.
func stateWords(state State) string {
	switch state {
	case Failed:
		return "This organization's server could not be started. It is being tried again."
	case Starting:
		return "This organization's server is starting."
	case Waiting:
		return "A machine is starting for this organization. That takes about a minute when none is running."
	}
	return "This organization is being set up."
}
