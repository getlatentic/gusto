// Package web is the pages a person uses: submit a job, watch it happen, read
// what it left behind.
//
// Server-rendered, with the templates compiled into the binary, because a
// customer installs one artifact and a second origin with its own build step is
// not one artifact.
package web

import (
	"embed"
	"html/template"
	"net/http"

	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/spend"
	"github.com/getlatentic/haas/internal/talk"
)

//go:embed templates/*.html
var pages embed.FS

type Server struct {
	index     *jobs.Index
	sessions  *talk.Sessions
	queue     *jobs.Queue
	keeper    *keeper
	templates *template.Template
	harnesses []string
	ledger    *spend.Ledger
}

func New(index *jobs.Index, sessions *talk.Sessions, queue *jobs.Queue, password string, harnesses []string) (*Server, error) {
	keeper, err := newKeeper(password)
	if err != nil {
		return nil, err
	}
	templates, err := template.New("").Funcs(helpers).ParseFS(pages, "templates/*.html")
	if err != nil {
		return nil, err
	}
	return &Server{index: index, sessions: sessions, queue: queue, keeper: keeper, templates: templates, harnesses: harnesses}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", s.loginPage)
	mux.HandleFunc("POST /login", s.signIn)
	mux.HandleFunc("POST /logout", s.signOut)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })

	mux.HandleFunc("GET /{$}", s.guard(s.list))
	mux.HandleFunc("POST /jobs", s.guard(s.submit))
	mux.HandleFunc("POST /api/jobs", s.sent(s.accept))
	mux.HandleFunc("GET /jobs/{id}", s.guard(s.job))
	mux.HandleFunc("GET /jobs/{id}/files/{name}", s.guard(s.file))
	mux.HandleFunc("GET /jobs/{id}/bundle", s.guard(s.bundle))
	mux.HandleFunc("GET /watch/{id}", s.guard(s.watch))
	mux.HandleFunc("GET /spend", s.guard(s.spendPage))
	mux.HandleFunc("GET /api/spend", s.sent(s.spendJSON))
	mux.HandleFunc("GET /talk", s.guard(s.talking))
	mux.HandleFunc("POST /talk", s.guard(s.begin))
	mux.HandleFunc("GET /talk/{id}", s.guard(s.conversation))
	mux.HandleFunc("POST /talk/{id}/say", s.guard(s.say))
	return mux
}

func (s *Server) render(w http.ResponseWriter, page string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, page, data); err != nil {
		http.Error(w, "the page could not be drawn", http.StatusInternalServerError)
	}
}
