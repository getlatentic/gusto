package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/getlatentic/haas/internal/apps"
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
	switch {
	case command == "" || len(command) > 500 || strings.ContainsAny(command, "\r\n\x00"):
		s.turnAway(w, r, back, "give the one line that starts the app, such as node server.js")
		return
	case err != nil || port < 1024 || port > 65535:
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

// app is a shared app, asked for by somebody signed in to this organisation's server. The request
// goes to the gateway as this server, with none of the person's credentials, and what comes back is
// stopped from being treated as part of these pages: a page the agent's code drew is not one this
// server drew, so it runs with no origin, no cookies of the person's, and nothing to do to their session.
func (s *Server) app(w http.ResponseWriter, r *http.Request) {
	if s.apps == nil {
		http.NotFound(w, r)
		return
	}
	app, err := s.apps.Get(r.PathValue("id"))
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
	prefix := "/apps/" + app.ID
	if r.URL.Path == prefix {
		http.Redirect(w, r, prefix+"/", http.StatusSeeOther)
		return
	}
	// What the pages set on every response would stop the app's own scripts and styles.
	w.Header().Del("Content-Security-Policy")
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.Out.URL.Scheme = "http"
			pr.Out.URL.Host = app.Target
			pr.Out.Host = app.Host
			path := strings.TrimPrefix(r.URL.Path, prefix)
			raw := ""
			if r.URL.RawPath != "" {
				raw = strings.TrimPrefix(r.URL.RawPath, prefix)
			}
			pr.Out.URL.Path, pr.Out.URL.RawPath = path, raw
			for _, header := range []string{"Cookie", "Authorization", "Proxy-Authorization", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "Forwarded", "X-Real-Ip"} {
				pr.Out.Header.Del(header)
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			for _, header := range []string{"Set-Cookie", "Strict-Transport-Security", "Clear-Site-Data", "Content-Security-Policy-Report-Only"} {
				resp.Header.Del(header)
			}
			resp.Header.Add("Content-Security-Policy", "sandbox allow-scripts allow-forms allow-popups allow-modals")
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
			if !errors.Is(err, context.Canceled) {
				s.apps.Change(app.ID, func(a *apps.App) { a.State, a.Note = apps.Stopped, "it stopped answering" })
			}
			http.Error(w, "this app is not answering", http.StatusBadGateway)
		},
	}
	proxy.ServeHTTP(w, r)
}

// Wait returns when every app started so far has answered or failed. A test uses it.
func (s *Server) Wait() { s.publishing.Wait() }
