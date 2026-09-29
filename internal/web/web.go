// Package web is the pages a person uses: start a harness, run work on it,
// watch it happen, and read what every run left behind.
//
// Server-rendered, with the templates compiled into the binary, because a
// customer installs one artifact and a second origin with its own build step is
// not one artifact.
package web

import (
	"embed"
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/getlatentic/haas/internal/catalog"
	"github.com/getlatentic/haas/internal/changelog"
	"github.com/getlatentic/haas/internal/compare"
	"github.com/getlatentic/haas/internal/instance"
	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/setup"
	"github.com/getlatentic/haas/internal/signing"
	"github.com/getlatentic/haas/internal/spend"
)

//go:embed templates/*.html
var pages embed.FS

type Server struct {
	index     *jobs.Index
	instances *instance.Store
	queue     *jobs.Queue
	keeper    *keeper
	templates *template.Template
	identity  *Identity
	catalog   []catalog.Entry
	models    Models
	ledger    *spend.Ledger
	keyring   signing.Keyring
	admins    map[string]bool
	// changes is the record of what changed on the platform, and turnedAway
	// counts the sign-ins written to it.
	changes    *changelog.Log
	turnedAway refusals
	lineups    *compare.Board
	setups     *setup.Store
	servers    setup.Servers
	// inBuilding is whether a model inside the deployment's own network is
	// reachable, without which restricted work cannot start at all.
	inBuilding bool
}

func New(index *jobs.Index, instances *instance.Store, queue *jobs.Queue, password string, identity *Identity, offered []catalog.Entry, models Models) (*Server, error) {
	keeper, err := newKeeper(password)
	if err != nil {
		return nil, err
	}
	templates, err := template.New("").Funcs(helpers).ParseFS(pages, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{index: index, instances: instances, queue: queue, keeper: keeper, templates: templates, identity: identity, catalog: offered, models: models}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", s.loginPage)
	mux.HandleFunc("POST /login", s.signIn)
	mux.HandleFunc("GET /auth/start", s.start)
	mux.HandleFunc("GET /auth/callback", s.back)
	mux.HandleFunc("POST /logout", s.signOut)
	mux.Handle("GET /static/", serveStatic())
	mux.HandleFunc("GET /keys/{file}", s.key)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })

	mux.HandleFunc("GET /{$}", s.guard(s.home))
	mux.HandleFunc("GET /new", s.guard(s.catalogPage))
	mux.HandleFunc("GET /new/blank", s.guard(s.blankPage))
	mux.HandleFunc("GET /engines", s.guard(s.enginesPage))
	mux.HandleFunc("GET /compare", s.guard(s.comparePage))
	mux.HandleFunc("POST /compare", s.guard(s.startComparison))
	mux.HandleFunc("GET /compare/{id}", s.guard(s.lineupPage))
	mux.HandleFunc("GET /setups", s.guard(s.setupsPage))
	mux.HandleFunc("GET /instances/new", s.guard(s.setUp))
	mux.HandleFunc("POST /instances", s.guard(s.startInstance))
	mux.HandleFunc("GET /instances/{id}", s.guard(s.instancePage))
	mux.HandleFunc("POST /instances/{id}/remove", s.guard(s.removeInstance))
	mux.HandleFunc("GET /instances/{id}/file", s.guard(s.folderFile))
	mux.HandleFunc("GET /instances/{id}/files", s.guard(s.folderPage))
	mux.HandleFunc("POST /instances/{id}/tasks", s.guard(s.runTask))
	mux.HandleFunc("POST /instances/{id}/threads", s.guard(s.openThread))
	mux.HandleFunc("GET /instances/{id}/threads/{thread}", s.guard(s.conversation))
	mux.HandleFunc("GET /instances/{id}/threads/{thread}/turns", s.guard(s.turns))
	mux.HandleFunc("POST /instances/{id}/threads/{thread}/say", s.guard(s.say))
	mux.HandleFunc("GET /templates/{name}", s.guard(s.templatePage))
	mux.HandleFunc("POST /templates/{name}", s.guard(s.startTemplate))
	mux.HandleFunc("GET /setups/{name}", s.guard(s.setupPage))
	mux.HandleFunc("POST /setups", s.guard(s.adminOnly(s.saveSetup)))
	mux.HandleFunc("POST /setups/{name}/skills", s.guard(s.adminOnly(s.writeSkill)))
	mux.HandleFunc("POST /setups/{name}/skills/{skill}/drop", s.guard(s.adminOnly(s.dropSkill)))
	mux.HandleFunc("GET /jobs", s.guard(s.list))
	mux.HandleFunc("POST /api/jobs", s.sent(s.accept))
	mux.HandleFunc("GET /jobs/{id}", s.guard(s.job))
	mux.HandleFunc("GET /jobs/{id}/files/{name}", s.guard(s.file))
	mux.HandleFunc("GET /jobs/{id}/bundle", s.guard(s.bundle))
	mux.HandleFunc("GET /watch/{id}", s.guard(s.watch))
	mux.HandleFunc("GET /spend", s.guard(s.spendPage))
	mux.HandleFunc("GET /changes", s.guard(s.adminPage(s.changesPage)))
	mux.HandleFunc("GET /changes.jsonl", s.guard(s.adminPage(s.changesFile)))
	mux.HandleFunc("GET /api/changes", s.sent(s.changesScript))
	mux.HandleFunc("GET /api/spend", s.sent(s.spendJSON))
	// Conversations were held at /talk before an instance could hold more than
	// one. A link somebody kept still finds its instance.
	mux.HandleFunc("GET /talk", moved("/"))
	mux.HandleFunc("GET /talk/{id}", moved("/instances/{id}"))
	return mux
}

// moved redirects an address that used to mean something here to where it
// means it now, filling {id} from the request.
func moved(to string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, strings.ReplaceAll(to, "{id}", r.PathValue("id")), http.StatusMovedPermanently)
	}
}

func (s *Server) render(w http.ResponseWriter, page string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, page, data); err != nil {
		slog.Error("a page could not be drawn", "page", page, "error", err)
		http.Error(w, "the page could not be drawn", http.StatusInternalServerError)
	}
}
