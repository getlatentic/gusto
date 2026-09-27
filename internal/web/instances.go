package web

import (
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/getlatentic/haas/internal/instance"
	"github.com/getlatentic/haas/internal/jobs"
)

// setUp is the step between picking a harness and having one: what it thinks
// with, and what it starts out working on.
func (s *Server) setUp(w http.ResponseWriter, r *http.Request) {
	wanted := r.URL.Query().Get("harness")
	for _, entry := range s.catalog {
		if entry.Name == wanted && entry.Ready {
			s.render(w, "setup.html", map[string]any{
				"Title":   "haas — " + entry.Name,
				"Nav":     "harnesses",
				"Harness": entry,
				"Models":  s.models.For(entry.Name),
				"Setups":  s.fitting(entry),
				"Chosen":  r.URL.Query().Get("setup"),
				"Then":    r.URL.Query().Get("then"),
				"Refused": r.URL.Query().Get("refused"),
			})
			return
		}
	}
	s.turnAway(w, r, "/new", "this deployment cannot start "+wanted)
}

// startInstance gives a harness a computer to work in.
func (s *Server) startInstance(w http.ResponseWriter, r *http.Request) {
	wanted := instance.Instance{
		Harness: r.FormValue("harness"),
		Model:   strings.TrimSpace(r.FormValue("model")),
		Repo:    strings.TrimSpace(r.FormValue("repo")),
		Setup:   strings.TrimSpace(r.FormValue("setup")),
		Who:     s.keeper.who(r),
	}
	if wanted.Harness == "" {
		s.turnAway(w, r, "/new", "pick a harness to start")
		return
	}
	if why := s.unfit(wanted); why != "" {
		s.turnAway(w, r, "/instances/new?harness="+wanted.Harness, why)
		return
	}
	// Starting from the catalog names no model, so it gets the first this
	// deployment offers *that harness* — not the first it offers at all, which
	// may be one this harness cannot use.
	if wanted.Model == "" {
		wanted.Model = s.models.First(wanted.Harness)
	}
	name := wanted.Harness
	if wanted.Repo != "" {
		name = filepath.Base(strings.TrimSuffix(wanted.Repo, ".git"))
	}
	started, err := s.instances.Start(s.index.Free(name, time.Now()), wanted)
	if err != nil {
		s.turnAway(w, r, "/new", err.Error())
		return
	}
	// A one-off against a repository is an instance started for a task, and
	// whoever asked for it wants the task form, not a conversation.
	if r.FormValue("then") == "task" {
		http.Redirect(w, r, "/instances/"+started.ID+"#task", http.StatusSeeOther)
		return
	}
	// Somebody who started a harness wants to be talking to it, not reading a
	// page about it, and starting it opened the first conversation.
	http.Redirect(w, r, threadPath(s.instances.Latest(started.ID)), http.StatusSeeOther)
}

// instancePage is one computer: what it runs, what it works in, and every
// conversation being held with it.
func (s *Server) instancePage(w http.ResponseWriter, r *http.Request) {
	held, found := s.instances.Get(r.PathValue("id"))
	if !found {
		http.NotFound(w, r)
		return
	}
	threads := s.namedThreads(held.ID)
	said := make([]map[string]any, 0, len(threads))
	for i := len(threads) - 1; i >= 0; i-- {
		said = append(said, map[string]any{"Thread": threads[i], "Latest": s.latestTurn(threads[i].Thread)})
	}
	s.render(w, "instance.html", map[string]any{
		"Title":     "haas — " + held.ID,
		"Nav":       "harnesses",
		"Instance":  held,
		"Threads":   said,
		"Workspace": s.instances.Workspace(held.ID),
		"Tasks":     s.tasks(held.ID),
		"Refused":   r.URL.Query().Get("refused"),
	})
}

// latestTurn is the last thing said in a thread, which is what a list of
// threads needs to tell them apart.
func (s *Server) latestTurn(thread instance.Thread) (record jobs.Record) {
	if len(thread.Turns) == 0 {
		return record
	}
	record, _ = s.index.Get(thread.Turns[len(thread.Turns)-1])
	return record
}

// openThread starts another conversation on an instance that is already there.
func (s *Server) openThread(w http.ResponseWriter, r *http.Request) {
	held, found := s.instances.Get(r.PathValue("id"))
	if !found {
		http.NotFound(w, r)
		return
	}
	thread, err := s.instances.NewThread(held.ID, strings.TrimSpace(r.FormValue("title")))
	if err != nil {
		s.turnAway(w, r, "/instances/"+held.ID, err.Error())
		return
	}
	http.Redirect(w, r, threadPath(thread), http.StatusSeeOther)
}

func threadPath(thread instance.Thread) string {
	return "/instances/" + thread.Instance + "/threads/" + thread.ID
}
