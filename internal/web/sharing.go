package web

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/apps"
)

// AppRunner runs the web apps that agents built, where the agents are. Start runs the app's
// command in the agent's computer and returns the app with where it can be asked, once it
// answers, or says why it did not. A deployment whose agents do not run in computers has none,
// and offers no sharing.
type AppRunner interface {
	Start(ctx context.Context, app apps.App) (apps.App, error)
	Stop(ctx context.Context, app apps.App) error
}

// UseApps lets people share the web apps their agents built.
func (s *Server) UseApps(store *apps.Store, runner AppRunner) {
	s.apps, s.runner = store, runner
}

const (
	// appStart is how long an app has to answer.
	appStart = 90 * time.Second
	// appsPerAgent is how many apps an agent may have at once.
	appsPerAgent = 3
)

// appsOf is an agent's apps, or none where nothing is shared.
func (s *Server) appsOf(instance string) []apps.App {
	if s.apps == nil {
		return nil
	}
	found, _ := s.apps.Of(instance)
	return found
}

// startingApp is whether any of the apps is not yet answering, which is why a page draws itself again.
func startingApp(found []apps.App) bool {
	for _, app := range found {
		if app.State == apps.Starting {
			return true
		}
	}
	return false
}

// publishApp starts the command a person names in their agent's computer, and shares what it
// serves once it answers. It returns at once, and the page shows the app starting.
func (s *Server) publishApp(w http.ResponseWriter, r *http.Request) {
	held, found := s.mine(w, r)
	if !found {
		return
	}
	if s.apps == nil {
		http.NotFound(w, r)
		return
	}
	back := "/instances/" + held.ID
	command := strings.TrimSpace(r.FormValue("command"))
	port, err := strconv.Atoi(strings.TrimSpace(r.FormValue("port")))
	folder := r.FormValue("preview")
	if folder != "" {
		// A page an agent built is served from its folder by a server of ours, so nobody has to write one. The person
		// is sent back to the conversation they asked from.
		back = s.backTo(held.ID, r.FormValue("back"), back)
		if command, port = previewCommand(folder), previewPort(s.appsOf(held.ID)); command == "" || port == 0 {
			s.turnAway(w, r, back, "that is not a folder of this agent's that can be previewed")
			return
		}
		err = nil
	}
	switch {
	// What a person types is one short line; the server of ours for a preview is longer and is not typed.
	case folder == "" && (command == "" || len(command) > 500 || strings.ContainsAny(command, "\r\n\x00")):
		s.turnAway(w, r, back, "give the one line that starts the app, such as node server.js")
		return
	case folder == "" && (err != nil || port < 1024 || port > 65535):
		s.turnAway(w, r, back, "the port is a number from 1024 to 65535, the one the app listens on")
		return
	}
	running := 0
	for _, each := range s.appsOf(held.ID) {
		if each.State == apps.Starting || each.State == apps.Live {
			running++
		}
	}
	if running >= appsPerAgent {
		s.turnAway(w, r, back, "this agent already shares "+strconv.Itoa(appsPerAgent)+" apps; stop one first")
		return
	}
	app, err := s.apps.New(held.ID, held.Who, command, port)
	if err != nil {
		s.turnAway(w, r, back, err.Error())
		return
	}
	s.publishing.Add(1)
	go func() {
		defer s.publishing.Done()
		s.startApp(app)
	}()
	http.Redirect(w, r, back+"#apps", http.StatusSeeOther)
}

// backTo is where to send a person after they started an app: the conversation they asked from when it is one of this
// agent's, and the agent's own page otherwise.
func (s *Server) backTo(instance, asked, otherwise string) string {
	prefix := "/instances/" + instance + "/threads/"
	rest, found := strings.CutPrefix(asked, prefix)
	if !found || rest == "" || strings.ContainsAny(rest, "/?#\\ ") {
		return otherwise
	}
	return asked
}

// startApp waits for the runner to have the app answering, and records how that went.
func (s *Server) startApp(app apps.App) {
	ctx, cancel := context.WithTimeout(context.Background(), appStart)
	defer cancel()
	started, err := s.runner.Start(ctx, app)
	if err != nil {
		note := err.Error()
		if len(note) > 1500 {
			note = note[len(note)-1500:]
		}
		s.apps.Change(app.ID, func(a *apps.App) { a.State, a.Note = apps.Failed, note })
		return
	}
	s.apps.Change(app.ID, func(a *apps.App) {
		a.State, a.Sandbox, a.Host, a.Target, a.Note = apps.Live, started.Sandbox, started.Host, started.Target, ""
	})
}

// removeApp stops an app and takes it off the agent's page.
func (s *Server) removeApp(w http.ResponseWriter, r *http.Request) {
	held, found := s.mine(w, r)
	if !found {
		return
	}
	if s.apps == nil {
		http.NotFound(w, r)
		return
	}
	app, err := s.apps.Get(r.PathValue("app"))
	if err != nil || app.Instance != held.ID {
		http.NotFound(w, r)
		return
	}
	s.stopApp(app)
	http.Redirect(w, r, "/instances/"+held.ID+"#apps", http.StatusSeeOther)
}

// stopApp stops an app's processes, gives its address back, and forgets it. An app that would not stop is
// still forgotten: the computer it ran in is what limits it.
func (s *Server) stopApp(app apps.App) {
	if app.State == apps.Live || app.State == apps.Starting {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.runner.Stop(ctx, app)
	}
	s.apps.Delete(app.ID)
}

// AppHeader says which app a request is for. The front door sets it, for a request to an
// app's own host name, and signs it with the key it shares with this server, so a client
// cannot write it and the request it arrives on can reach nothing but that app.
const AppHeader = "X-Gusto-App"

// SignApp is the header's value for an app: its id, and a signature only the front door and this server can make.
func SignApp(key []byte, id string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("app:" + id))
	return id + "." + hex.EncodeToString(mac.Sum(nil))
}

// verifiedApp is the app id in a header the front door signed, or nothing.
func (s *Server) verifiedApp(header string) (string, bool) {
	if s.front == nil {
		return "", false
	}
	id, _, found := strings.Cut(header, ".")
	if !found || !apps.Valid(id) || !hmac.Equal([]byte(header), []byte(SignApp(s.front.Key, id))) {
		return "", false
	}
	return id, true
}

// byApp keeps a request for an app's own host name away from everything else on this
// server: it is signed in with the token the front door made, and passed to the app, and
// nothing of the organisation's pages can be reached on that name.
func (s *Server) byApp(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get(AppHeader)
		if header == "" {
			next.ServeHTTP(w, r)
			return
		}
		id, ok := s.verifiedApp(header)
		if !ok {
			http.Error(w, "that is not an app's address", http.StatusBadRequest)
			return
		}
		s.appOrigin(w, r, id)
	})
}

// appOrigin serves an app at the root of its own host name, so that what the app wrote works
// as it wrote it, absolute paths included, and the browser keeps it apart from the organisation's pages.
func (s *Server) appOrigin(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method == http.MethodGet && r.URL.Path == "/auth/entry" {
		s.enter(w, r)
		return
	}
	if !s.keeper.knows(r) {
		http.Redirect(w, r, s.front.URL+"/open?app="+id, http.StatusSeeOther)
		return
	}
	s.serveApp(w, r, id, "")
}

// appLink is where an app is opened: its own host name behind a front door, and a path on this
// server's own where there is none.
func (s *Server) appLink(app apps.App) string {
	if s.front == nil {
		return "/apps/" + app.ID + "/"
	}
	parsed, err := url.Parse(s.front.URL)
	if err != nil {
		return "/apps/" + app.ID + "/"
	}
	return parsed.Scheme + "://" + s.front.Org + "--" + app.ID + "." + parsed.Host + "/"
}

// app is the path an app is opened at on a server with no front door. Behind one, an app has a host
// name of its own, and this sends a link somebody kept there.
func (s *Server) app(w http.ResponseWriter, r *http.Request) {
	if s.apps == nil {
		http.NotFound(w, r)
		return
	}
	id := r.PathValue("id")
	if s.front != nil {
		if app, err := s.apps.Get(id); err == nil {
			http.Redirect(w, r, s.appLink(app), http.StatusSeeOther)
			return
		}
		http.NotFound(w, r)
		return
	}
	prefix := "/apps/" + id
	if r.URL.Path == prefix {
		http.Redirect(w, r, prefix+"/", http.StatusSeeOther)
		return
	}
	s.serveApp(w, r, id, prefix)
}

// serveApp passes a request to a shared app as this server, with the person's credentials taken out
// of it. With a prefix the app is served under a path of this server's own origin, so what comes back
// is stopped from being treated as one of these pages: it runs sandboxed, with no origin, no cookies and
// nothing to do to the person's session. With none the app has an origin of its own, and needs no such
// fence, only that it cannot set what belongs to this server's.
func (s *Server) serveApp(w http.ResponseWriter, r *http.Request, id, prefix string) {
	if s.apps == nil {
		http.NotFound(w, r)
		return
	}
	app, err := s.apps.Get(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch app.State {
	case apps.Live:
	case apps.Starting:
		http.Error(w, "this app is still starting", http.StatusServiceUnavailable)
		return
	default:
		http.NotFound(w, r)
		return
	}
	// What the pages set on every response would stop the app's own scripts and styles, and
	// would be said twice beside the ones set below.
	for _, header := range []string{"Content-Security-Policy", "Referrer-Policy", "Cache-Control", "X-Content-Type-Options", "Strict-Transport-Security"} {
		w.Header().Del(header)
	}
	proxy := &httputil.ReverseProxy{
		Transport: s.appTransport(app),
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = "http"
			pr.Out.URL.Host = app.Target
			pr.Out.Host = app.Host
			pr.Out.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
			pr.Out.URL.RawPath = ""
			if r.URL.RawPath != "" {
				pr.Out.URL.RawPath = strings.TrimPrefix(r.URL.RawPath, prefix)
			}
			for _, header := range []string{"Authorization", "Proxy-Authorization", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "Forwarded", "X-Real-Ip", AppHeader} {
				pr.Out.Header.Del(header)
			}
			pr.Out.Header.Del("Cookie")
			// The gateway refuses a request whose Origin is not its own, as a defence for the apps behind
			// it, and a browser sends the address it drew the page at. This server stands in for the
			// gateway's own address, so a request that names an origin names that.
			pr.Out.Header.Del("Referer")
			if r.Header.Get("Origin") != "" || (r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions) {
				pr.Out.Header.Set("Origin", "http://"+app.Host)
			}
			if prefix == "" {
				for _, cookie := range r.Cookies() {
					if cookie.Name != cookieName && cookie.Name != secureCookieName {
						pr.Out.AddCookie(cookie)
					}
				}
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			for _, header := range []string{"Strict-Transport-Security", "Clear-Site-Data", "Content-Security-Policy-Report-Only"} {
				resp.Header.Del(header)
			}
			cookies := resp.Header.Values("Set-Cookie")
			resp.Header.Del("Set-Cookie")
			if prefix == "" {
				keepAppCookies(resp, cookies)
			} else {
				resp.Header.Add("Content-Security-Policy", "sandbox allow-scripts allow-forms allow-popups allow-modals")
			}
			resp.Header.Set("X-Content-Type-Options", "nosniff")
			resp.Header.Set("Referrer-Policy", "no-referrer")
			resp.Header.Set("Cache-Control", "no-store")
			// A redirect to a path of the app's own is to a path of the app.
			if location := resp.Header.Get("Location"); strings.HasPrefix(location, "/") && !strings.HasPrefix(location, "//") {
				resp.Header.Set("Location", prefix+location)
			} else if parsed, err := url.Parse(location); err == nil && parsed.Host == app.Host {
				resp.Header.Set("Location", prefix+parsed.RequestURI())
			}
			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			// An app that is slow, or that a person gave up on, has not stopped; one that refuses a connection has.
			var timeout net.Error
			if !errors.Is(err, context.Canceled) && !(errors.As(err, &timeout) && timeout.Timeout()) {
				s.apps.Change(app.ID, func(a *apps.App) { a.State, a.Note = apps.Stopped, "it stopped answering" })
			}
			http.Error(w, "this app is not answering", http.StatusBadGateway)
		},
	}
	proxy.ServeHTTP(w, r)
}

// keepAppCookies gives back the cookies an app set for itself. They are the app's own, host only, and
// none is one of this server's: a cookie with a Domain would be sent to the organisation's pages as well,
// and one with this server's session name would sign a person out of, or in as somebody, on this host.
func keepAppCookies(resp *http.Response, lines []string) {
	for _, line := range lines {
		cookie, err := http.ParseSetCookie(line)
		if err != nil || cookie.Name == cookieName || cookie.Name == secureCookieName || strings.HasPrefix(cookie.Name, "__Host-") || strings.HasPrefix(cookie.Name, "__Secure-") {
			continue
		}
		cookie.Domain = ""
		resp.Header.Add("Set-Cookie", cookie.String())
	}
}

// Wait returns when every app started so far has answered or failed. A test uses it.
func (s *Server) Wait() { s.publishing.Wait() }
