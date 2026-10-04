package web

import (
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/getlatentic/haas/internal/instance"
	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/repoaccess"
)

// setUp is the step between picking a harness and having one: what it thinks
// with, and what it starts out working on.
func (s *Server) setUp(w http.ResponseWriter, r *http.Request) {
	wanted := r.URL.Query().Get("harness")
	for _, entry := range s.entries() {
		if entry.Name == wanted && entry.Ready {
			s.render(w, "setup.html", map[string]any{
				"Title":   productName + " — " + entry.Name,
				"Nav":     "harnesses",
				"Harness": entry,
				"Models":  s.offeredTo(r).For(entry.Name),
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
		Title:   strings.TrimSpace(r.FormValue("title")),
		Who:     s.keeper.who(r),
	}
	if wanted.Repo != "" {
		if err := repoaccess.Acceptable(wanted.Repo, s.localRepos); err != nil {
			s.turnAway(w, r, "/instances/new?harness="+wanted.Harness, err.Error())
			return
		}
	}
	if wanted.Harness == "" {
		wanted.Harness = s.firstReady()
	}
	if wanted.Harness == "" {
		s.turnAway(w, r, "/new", "this deployment cannot start an agent yet")
		return
	}
	if why := s.unfit(wanted); why != "" {
		s.turnAway(w, r, "/instances/new?harness="+wanted.Harness, why)
		return
	}
	if why := s.unusable(r, wanted.Harness, wanted.Model); why != "" {
		s.turnAway(w, r, "/instances/new?harness="+wanted.Harness, why)
		return
	}
	// Starting from the catalog names no model, so it gets the first this
	// deployment offers *that harness* — not the first it offers at all, which
	// may be one this harness cannot use.
	if wanted.Model == "" {
		wanted.Model = s.offeredTo(r).First(wanted.Harness)
	}
	name := wanted.Harness
	if wanted.Repo != "" {
		name = filepath.Base(strings.TrimSuffix(wanted.Repo, ".git"))
	}
	started, err := s.instances.Start(jobs.Name(name, time.Now()), wanted)
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
	held, found := s.mine(w, r)
	if !found {
		return
	}
	threads := s.namedThreads(held.ID)
	said := make([]map[string]any, 0, len(threads))
	for i := len(threads) - 1; i >= 0; i-- {
		said = append(said, map[string]any{"Thread": threads[i], "Latest": s.latestTurn(threads[i].Thread)})
	}
	s.render(w, "instance.html", map[string]any{
		"Title":     productName + " — " + s.agentOf(held).Name,
		"Nav":       "harnesses",
		"Instance":  held,
		"Agent":     s.agentOf(held),
		"Threads":   said,
		"Workspace": s.instances.Workspace(held.ID),
		"Models":    s.offeredTo(r).For(held.Harness),
		"Sharing":   s.apps != nil,
		"Apps":      s.appsOf(held.ID),
		"Refresh":   startingApp(s.appsOf(held.ID)),
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
	held, found := s.mine(w, r)
	if !found {
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

// firstReady is the engine a blank agent gets when nobody chooses: the
// first this deployment can run, in the catalog's order.
func (s *Server) firstReady() string {
	for _, entry := range s.entries() {
		if entry.Ready {
			return entry.Name
		}
	}
	return ""
}

// unusable says why a harness cannot be started on a model, or is empty when it can. An
// agent is never started on a pair that cannot work, however the request was made.
func (s *Server) unusable(r *http.Request, harness, model string) string {
	offered := s.offeredTo(r)
	if s.limitedAway(r, harness) {
		return "you may not use any model that works with " + harness
	}
	if !offered.Serves(harness) {
		return "no model this deployment offers works with " + harness
	}
	if model == "" {
		return ""
	}
	if !offered.Offers(harness, model) {
		return harness + " cannot use " + modelName(model) + choose(offered.For(harness))
	}
	return ""
}

// choose is what to say a person may use instead, when there is something.
func choose(offered []string) string {
	if len(offered) == 0 {
		return ""
	}
	names := make([]string, len(offered))
	for i, each := range offered {
		names[i] = modelName(each)
	}
	return ": choose " + strings.Join(names, " or ")
}

// changeModel points an agent at another model its harness can use, from the next message
// on. What was said so far stays as it was, and each turn's record names the model it ran on.
func (s *Server) changeModel(w http.ResponseWriter, r *http.Request) {
	held, found := s.mine(w, r)
	if !found {
		return
	}
	back := "/instances/" + held.ID
	model := strings.TrimSpace(r.FormValue("model"))
	if why := s.unusable(r, held.Harness, model); why != "" || model == "" {
		if why == "" {
			why = "choose a model"
		}
		s.turnAway(w, r, back, why)
		return
	}
	if err := s.instances.SetModel(held.ID, model); err != nil {
		s.turnAway(w, r, back, err.Error())
		return
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}
