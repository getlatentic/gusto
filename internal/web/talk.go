package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getlatentic/haas/internal/catalog"
	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/talk"
)

// talking lists the conversations, and offers to start one.
func (s *Server) talking(w http.ResponseWriter, r *http.Request) {
	s.render(w, "talking.html", map[string]any{
		"Title":     "haas — talking",
		"Subtitle":  "a conversation with a harness",
		"Sessions":  s.sessions.List(),
		"Harnesses": catalog.Ready(s.catalog),
		"Catalog":   s.catalog,
		"Models":    s.models,
		"Refused":   r.URL.Query().Get("refused"),
	})
}

// begin starts a conversation. A repository is optional: with one the harness
// works in a checkout, without one it works in an empty directory.
func (s *Server) begin(w http.ResponseWriter, r *http.Request) {
	wanted := talk.Session{
		Harness: r.FormValue("harness"),
		Model:   strings.TrimSpace(r.FormValue("model")),
		Repo:    strings.TrimSpace(r.FormValue("repo")),
	}
	if wanted.Harness == "" {
		http.Redirect(w, r, "/talk?refused=a+conversation+needs+a+harness", http.StatusSeeOther)
		return
	}
	name := "talk"
	if wanted.Repo != "" {
		name = filepath.Base(strings.TrimSuffix(wanted.Repo, ".git"))
	}
	started, err := s.sessions.Start(s.index.Free(name, time.Now()), wanted)
	if err != nil {
		http.Redirect(w, r, "/talk?refused="+http.StatusText(http.StatusBadRequest)+": "+err.Error(), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/talk/"+started.ID, http.StatusSeeOther)
}

// conversation shows what has been said, and offers somewhere to say more.
func (s *Server) conversation(w http.ResponseWriter, r *http.Request) {
	session, found := s.sessions.Get(r.PathValue("id"))
	if !found {
		http.NotFound(w, r)
		return
	}
	type turn struct {
		jobs.Record
		Said    string
		Replied string
	}
	var turns []turn
	for _, id := range session.Turns {
		record, found := s.index.Get(id)
		if !found {
			continue
		}
		turns = append(turns, turn{
			Record:  record,
			Said:    record.Brief,
			Replied: said(record.Dir(s.index.Root())),
		})
	}
	running := ""
	if len(turns) > 0 && !turns[len(turns)-1].State.Done() {
		running = turns[len(turns)-1].ID
	}
	s.render(w, "conversation.html", map[string]any{
		"Title":    "haas — " + session.ID,
		"Subtitle": session.ID,
		"Session":  session,
		"Turns":    turns,
		"Waiting":  running != "",
		"Latest":   running,
	})
}

// said is what the harness replied, read from the turn's own transcript. It is
// bounded because a page is not the place to read a megabyte.
func said(dir job.Dir) string {
	text, err := os.ReadFile(dir.OutFile("transcript.log"))
	if err != nil {
		return ""
	}
	const most = 40 * 1024
	if len(text) > most {
		return "…" + string(text[len(text)-most:])
	}
	return string(text)
}

// say is one turn: an ordinary job that mounts the conversation.
func (s *Server) say(w http.ResponseWriter, r *http.Request) {
	session, found := s.sessions.Get(r.PathValue("id"))
	if !found {
		http.NotFound(w, r)
		return
	}
	said := strings.TrimSpace(r.FormValue("said"))
	if said == "" {
		http.Redirect(w, r, "/talk/"+session.ID, http.StatusSeeOther)
		return
	}
	spec := job.Spec{
		Brief:      said,
		Harness:    session.Harness,
		Model:      session.Model,
		Session:    session.ID,
		Remembered: session.Remembered,
		Repo:       session.Repo,
	}
	id := s.index.Free(said, time.Now())
	dir := job.Dir(filepath.Join(s.index.Root(), id))
	if err := dir.Make(); err != nil {
		s.refuse(w, r, err)
		return
	}
	if err := spec.Write(dir); err != nil {
		s.refuse(w, r, err)
		return
	}
	if err := harness.Keep(dir, spec.Harness); err != nil {
		s.refuse(w, r, err)
		return
	}
	record := jobs.Record{
		ID: id, State: jobs.Queued, Brief: said, Repo: session.Repo,
		Harness: session.Harness, Model: session.Model,
		Asked: "the shared sign-in", Queued: jobs.Now(),
	}
	if err := s.index.Add(record); err != nil {
		s.refuse(w, r, err)
		return
	}
	if err := s.sessions.Said(session.ID, id, ""); err != nil {
		s.refuse(w, r, err)
		return
	}
	s.queue.Wake()
	http.Redirect(w, r, "/talk/"+session.ID+"#"+id, http.StatusSeeOther)
}
