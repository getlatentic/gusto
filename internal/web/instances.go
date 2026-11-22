package web

import (
	"cmp"
	"net/http"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/repoaccess"
	"github.com/getlatentic/gusto/internal/setup"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/internal/team"
)

// setUp is the step between picking a harness and having one: what it thinks with, and what it starts out working on. A harness
// this deployment cannot start sends the person back to the presets, saying so.
func (s *Server) setUp(w http.ResponseWriter, r *http.Request) {
	if _, found := s.harnessToSetUp(r); !found {
		s.turnAway(w, r, "/presets", "this deployment cannot start "+r.URL.Query().Get("harness"))
		return
	}
	s.appPage(w, r)
}

// harnessToSetUp is the harness the address names, when this deployment can start it.
func (s *Server) harnessToSetUp(r *http.Request) (catalog.Entry, bool) {
	wanted := r.URL.Query().Get("harness")
	for _, entry := range s.engines() {
		if entry.Name == wanted && entry.Ready {
			return entry, true
		}
	}
	return catalog.Entry{}, false
}

// setUpHarness is the harness being set up, with its mark when it has one.
type setUpHarness struct{ Name, Title, What, Mark string }

// setUpData is what the set-up page offers: the models the person may use, and the skill sets the harness can take. Chosen is the skill set the link named, and Then
// what the person wants once it has started.
func (s *Server) setUpData(r *http.Request) (map[string]any, error) {
	entry, found := s.harnessToSetUp(r)
	if !found {
		return nil, failed{http.StatusNotFound, "this deployment cannot start " + r.URL.Query().Get("harness")}
	}
	models := s.modelOptionsOf(s.offeredTo(r))
	setups := []option{}
	for _, held := range s.fitting(entry) {
		setups = append(setups, option{Value: held.Name, Label: setupLabel(held)})
	}
	return map[string]any{
		"Harness": setUpHarness{Name: entry.Name, Title: entry.Title, What: entry.What, Mark: logo(entry.Name)},
		"Models":  models,
		"Setups":  setups,
		"Chosen":  r.URL.Query().Get("setup"),
		"Then":    r.URL.Query().Get("then"),
		"Refused": s.refusal(r),
	}, nil
}

// setupLabel is a skill set as the set-up page's menu says it: "reviewer · 2 skills, 1 MCP".
func setupLabel(held setup.Setup) string {
	label := held.Name + " · " + strconv.Itoa(len(held.Skills)) + " skill"
	if len(held.Skills) != 1 {
		label += "s"
	}
	if len(held.Servers) > 0 {
		label += ", " + strconv.Itoa(len(held.Servers)) + " MCP"
	}
	return label
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
		return started, "/presets", "this deployment cannot start an agent yet"
	}
	if why := s.runnable(wanted.Harness); why != "" {
		return started, "/presets", why
	}
	if why := s.unfit(wanted); why != "" {
		return started, "/instances/new?harness=" + wanted.Harness, why
	}
	if why := s.unusable(r, wanted.Model); why != "" {
		return started, "/instances/new?harness=" + wanted.Harness, why
	}
	// Starting from the catalog names no model, so it gets one this deployment offers that harness and that the harness can
	// finish on, not merely the first offered.
	if wanted.Model == "" {
		wanted.Model = cmp.Or(s.modelToStartOn(r, wanted.Harness), first(s.offeredTo(r)))
	}
	wanted.Team = team.Stored(s.teamOf(r))
	started, err := s.instances.Start(tasks.Name(instanceName(wanted), time.Now()), wanted)
	if err != nil {
		return started, "/presets", err.Error()
	}
	return started, "", ""
}

// instanceName is what a new instance's id begins with: its repository's name, or its agent type's when it has none.
func instanceName(wanted instance.Instance) string {
	if wanted.Repo != "" {
		return filepath.Base(strings.TrimSuffix(wanted.Repo, ".git"))
	}
	return wanted.Harness
}

// runnable says why an agent cannot be started here: there is none by that name, or this machine cannot run it now.
func (s *Server) runnable(harness string) string {
	for _, entry := range s.engines() {
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

// latestTurn is the last thing said in a thread, which is what a list of
// threads needs to tell them apart.
func (s *Server) latestTurn(thread instance.Thread) (record tasks.Record) {
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
	for _, entry := range s.engines() {
		if entry.Ready {
			return entry.Name
		}
	}
	return ""
}

// unusable says why an agent cannot be started on a model, or is empty when it can: the model has to be one this
// person is offered, however the request was made.
func (s *Server) unusable(r *http.Request, model string) string {
	if lacking := s.lacking(); len(lacking) > 0 {
		return lacking[0]
	}
	if s.unconfirmed() {
		return "confirm on the chat page that agents run with no sandbox, once, first"
	}
	offered := s.offeredTo(r)
	if s.limitedAway(r) {
		return limitedWords
	}
	if model == "" || offered.Offers(model) {
		return ""
	}
	// A model offered under the same name through another provider is told apart by its whole name.
	if slices.ContainsFunc(offered, func(each string) bool { return modelName(each) == modelName(model) }) {
		return model + " is not offered here: choose " + strings.Join(offered, " or ")
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
