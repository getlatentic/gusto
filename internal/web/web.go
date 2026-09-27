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
	"net/http"
	"strings"

	"github.com/getlatentic/haas/internal/catalog"
	"github.com/getlatentic/haas/internal/instance"
	"github.com/getlatentic/haas/internal/jobs"
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
	models    []string
	ledger    *spend.Ledger
}

func New(index *jobs.Index, instances *instance.Store, queue *jobs.Queue, password string, identity *Identity, offered []catalog.Entry, models []string) (*Server, error) {
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
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })

	mux.HandleFunc("GET /{$}", s.guard(s.catalogPage))
	mux.HandleFunc("GET /instances/new", s.guard(s.setUp))
	mux.HandleFunc("POST /instances", s.guard(s.startInstance))
	mux.HandleFunc("GET /instances/{id}", s.guard(s.instancePage))
	mux.HandleFunc("POST /instances/{id}/tasks", s.guard(s.runTask))
	mux.HandleFunc("POST /instances/{id}/threads", s.guard(s.openThread))
	mux.HandleFunc("GET /instances/{id}/threads/{thread}", s.guard(s.conversation))
	mux.HandleFunc("GET /instances/{id}/threads/{thread}/turns", s.guard(s.turns))
	mux.HandleFunc("POST /instances/{id}/threads/{thread}/say", s.guard(s.say))
	mux.HandleFunc("GET /jobs", s.guard(s.list))
	mux.HandleFunc("POST /api/jobs", s.sent(s.accept))
	mux.HandleFunc("GET /jobs/{id}", s.guard(s.job))
	mux.HandleFunc("GET /jobs/{id}/files/{name}", s.guard(s.file))
	mux.HandleFunc("GET /jobs/{id}/bundle", s.guard(s.bundle))
	mux.HandleFunc("GET /watch/{id}", s.guard(s.watch))
	mux.HandleFunc("GET /spend", s.guard(s.spendPage))
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
		http.Error(w, "the page could not be drawn", http.StatusInternalServerError)
	}
}
