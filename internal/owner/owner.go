// Package owner is the page the owner of a hosted haas manages it from: who asked for a demo, which demos are running,
// whether new ones are being accepted, and which hosts are up. It is its own program, listening only on this machine's
// loopback, so that nothing the front door serves reaches it; the owner reaches it through a tunnel they are allowed to
// open.
package owner

import (
	"embed"
	"encoding/csv"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/getlatentic/haas/internal/cell"
	"github.com/getlatentic/haas/internal/cloud"
	"github.com/getlatentic/haas/internal/web"
)

//go:embed templates/*.html
var files embed.FS

// Hosts lists the hosts that run organisations' servers.
type Hosts interface {
	Running() ([]cell.Instance, error)
}

// App is the owner's pages over the front door's own records.
type App struct {
	Store *cloud.Store
	// Hosts is nil where hosts are not started on demand.
	Hosts Hosts
	Now   func() time.Time
	pages *template.Template
}

// New parses the pages.
func New(store *cloud.Store, hosts Hosts) (*App, error) {
	pages, err := template.New("").Funcs(template.FuncMap{"asset": web.Asset, "ago": ago, "left": left}).ParseFS(files, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &App{Store: store, Hosts: hosts, Now: time.Now, pages: pages}, nil
}

// Handler serves the pages to requests that came to this machine's own loopback address, and to no others.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.dashboard)
	mux.HandleFunc("POST /demos/pause", a.pause)
	mux.HandleFunc("GET /leads.csv", a.leadsCSV)
	mux.Handle("GET /static/", web.Static())
	return local(mux)
}

// local refuses a request that did not come straight to the loopback: one carrying a forwarding header went through
// a proxy, and one for another host name is a page of another site asking a browser to reach this one.
func local(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		if r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("Forwarded") != "" || !slices.Contains([]string{"localhost", "127.0.0.1", "[::1]", "::1"}, host) {
			http.Error(w, "this is not served here", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method == http.MethodPost && r.Header.Get("Sec-Fetch-Site") != "same-origin" && r.Header.Get("Sec-Fetch-Site") != "" {
			http.Error(w, "that form was sent from another site", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type running struct {
	Name, Company, Slug string
	Left                time.Duration
	Ready               bool
}

type hostRow struct {
	ID       string
	Launched time.Time
}

func (a *App) dashboard(w http.ResponseWriter, r *http.Request) {
	leads, err := a.Store.Leads()
	if err != nil {
		slog.Error("the leads could not be read", "error", err)
		http.Error(w, "the leads could not be read", http.StatusInternalServerError)
		return
	}
	page := map[string]any{"Title": "haas owner", "Leads": leads, "Paused": a.Store.DemosPaused(), "Now": a.Now(), "Notice": r.URL.Query().Get("notice")}
	page["Running"] = a.running()
	if a.Hosts != nil {
		if hosts, err := a.Hosts.Running(); err != nil {
			page["HostsError"] = err.Error()
		} else {
			page["Hosts"] = hosts
			page["HostsKnown"] = true
		}
	}
	if err := a.pages.ExecuteTemplate(w, "owner.html", page); err != nil {
		slog.Error("the owner page was not drawn", "error", err)
	}
}

// running is the demos that have not ended, soonest to end first.
func (a *App) running() []running {
	orgs, err := a.Store.Orgs()
	if err != nil {
		return nil
	}
	var rows []running
	for _, org := range orgs {
		if demo, time, found := a.Store.DemoLeft(org.Slug); found {
			rows = append(rows, running{Name: demo.Name, Company: demo.Company, Slug: org.Slug, Left: time, Ready: !demo.Ready.IsZero()})
		}
	}
	slices.SortFunc(rows, func(x, y running) int { return int(x.Left - y.Left) })
	return rows
}

func (a *App) pause(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	paused := r.PostForm.Get("paused") == "on"
	notice := "New demos are being accepted."
	if paused {
		notice = "New demos are paused. Demos that are running go on."
	}
	if err := a.Store.SetDemosPaused(paused); err != nil {
		slog.Error("demos were not paused or resumed", "error", err)
		notice = "That did not change: " + err.Error()
	}
	http.Redirect(w, r, "/?notice="+strings.ReplaceAll(notice, " ", "+"), http.StatusSeeOther)
}

// leadsCSV is every lead for a spreadsheet. A cell a spreadsheet would read as a formula is written as text, since
// the visitor typed it.
func (a *App) leadsCSV(w http.ResponseWriter, r *http.Request) {
	leads, err := a.Store.Leads()
	if err != nil {
		http.Error(w, "the leads could not be read", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="haas-leads.csv"`)
	out := csv.NewWriter(w)
	_ = out.Write([]string{"requested", "name", "email", "company", "ready", "ended", "extended", "why it ended"})
	for _, lead := range leads {
		_ = out.Write([]string{when(lead.Requested), text(lead.Name), text(lead.Email), text(lead.Company), when(lead.Ready), when(lead.Ended), yes(lead.Extended), text(lead.Reason)})
	}
	out.Flush()
}

func when(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func yes(b bool) string {
	if b {
		return "yes"
	}
	return ""
}

func text(s string) string {
	if s != "" && strings.ContainsAny(s[:1], "=+-@\t\r") {
		return "'" + s
	}
	return s
}

func ago(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d/time.Minute))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h ago", int(d/time.Hour))
	}
	return t.Format("2 Jan")
}

// left is a countdown as a clock shows it.
func left(d time.Duration) string {
	seconds := int(d / time.Second)
	return fmt.Sprintf("%d:%02d", seconds/60, seconds%60)
}
