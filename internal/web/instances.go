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
	for _, entry := range s.catalog {
		if entry.Name == wanted && entry.Ready {
			s.render(w, "setup.html", map[string]any{
				"Title":   productName + " — " + entry.Name,
				"Nav":     "harnesses",
				"Harness": entry,
				"Models":  s.offeredTo(r),
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
	started, back, why := s.beginInstance(r, wanted)
	if why != "" {
		s.turnAway(w, r, back, why)
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

// beginInstance starts an agent for the person asking, when this deployment can run what they asked and they may use it.
// It says why not when it cannot, and which page to send them back to: the page and the API both start an agent this way,
// so what a person may start does not depend on how they asked.
func (s *Server) beginInstance(r *http.Request, wanted instance.Instance) (started instance.Instance, back, why string) {
	if wanted.Repo != "" {
		if err := repoaccess.Acceptable(wanted.Repo, s.localRepos); err != nil {
			return started, "/instances/new?harness=" + wanted.Harness, err.Error()
		}
	}
	if wanted.Harness == "" {
		wanted.Harness = s.firstReady()
	}
	if wanted.Harness == "" {
		return started, "/new", "this deployment cannot start an agent yet"
	}
	if why := s.runnable(wanted.Harness); why != "" {
		return started, "/new", why
	}
	if why := s.unfit(wanted); why != "" {
		return started, "/instances/new?harness=" + wanted.Harness, why
	}
	if why := s.unusable(r, wanted.Model); why != "" {
		return started, "/instances/new?harness=" + wanted.Harness, why
	}
	// Starting from the catalog names no model, so it gets the first this
	// deployment offers *that harness* — not the first it offers at all, which
	// may be one this harness cannot use.
	if wanted.Model == "" {
		wanted.Model = first(s.offeredTo(r))
	}
	name := wanted.Harness
	if wanted.Repo != "" {
		name = filepath.Base(strings.TrimSuffix(wanted.Repo, ".git"))
	}
	started, err := s.instances.Start(jobs.Name(name, time.Now()), wanted)
	if err != nil {
		return started, "/new", err.Error()
	}
	return started, "", ""
}

// runnable says why an agent cannot be started here: there is none by that name, or this machine cannot run it now.
func (s *Server) runnable(harness string) string {
	for _, entry := range s.catalog {
		if entry.Name != harness {
			continue
		}
		if !entry.Ready {
			return harness + " cannot run here: " + entry.Why
		}
		return ""
	}
	return "there is no agent called " + harness + " here"
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
		"Models":    s.offeredTo(r),
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
	for _, entry := range s.catalog {
		if entry.Ready {
			return entry.Name
		}
	}
	return ""
}

// unusable says why an agent cannot be started on a model, or is empty when it can: the model has to be one this
// person is offered, however the request was made.
func (s *Server) unusable(r *http.Request, model string) string {
	offered := s.offeredTo(r)
	if s.limitedAway(r) {
		return limitedWords
	}
	if model == "" || offered.Offers(model) {
		return ""
	}
	return modelName(model) + " is not offered here" + choose(offered)
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
	if from := r.FormValue("back"); strings.HasPrefix(from, back+"/threads/") && !strings.ContainsAny(from, "?#\\") {
		back = from
	}
	model := strings.TrimSpace(r.FormValue("model"))
	if why := s.unusable(r, model); why != "" || model == "" {
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
