package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/getlatentic/haas/internal/catalog"
	"github.com/getlatentic/haas/internal/compare"
	"github.com/getlatentic/haas/internal/industry"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/rules"
)

// UseBoard gives the pages somewhere to keep comparisons. Without one, a
// comparison starts only on the command line.
func (s *Server) UseBoard(board *compare.Board) { s.lineups = board }

// comparable is a template whose sample work has a check, which is what lets
// the same task be given to several engines and decided the same way.
func comparable() []industry.Template {
	var fit []industry.Template
	for _, held := range industry.All() {
		if len(held.Check) > 0 && !held.Restricted() {
			fit = append(fit, held)
		}
	}
	return fit
}

func (s *Server) comparePage(w http.ResponseWriter, r *http.Request) {
	page := map[string]any{"Title": productName + " — compare agents", "Nav": "compare", "Refused": r.URL.Query().Get("refused")}
	if s.lineups != nil {
		page["Lineups"] = s.ownLineups(r, s.lineups.List())
	}
	if fit := comparable(); len(fit) > 0 {
		held := fit[0]
		page["Template"] = held
		page["Engines"] = s.runnersOf(held)
		page["Models"] = s.sharedModels(held, s.offeredTo(r))
	}
	s.render(w, "compare.html", page)
}

// sharedModels is each model, by the name a person reads, that the template's engines can be compared on: every model
// offered, when at least two engines can take the template.
func (s *Server) sharedModels(held industry.Template, models Models) []string {
	if len(s.runnersOf(held)) < 2 {
		return nil
	}
	var named []string
	for _, model := range models {
		if !slices.Contains(named, modelOf(model)) {
			named = append(named, modelOf(model))
		}
	}
	return named
}

// modelFor is the model an engine is pointed at for a plain model name: the first offered that goes by that name.
func modelFor(models Models, named string) (string, bool) {
	for _, model := range models {
		if modelOf(model) == named {
			return model, true
		}
	}
	return "", false
}

func (s *Server) startComparison(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id, err := s.lineUp(r, r.PostForm.Get("brief"), r.PostForm.Get("model"), r.PostForm["harness"])
	if err != nil {
		http.Redirect(w, r, "/compare?refused="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/compare/"+id, http.StatusSeeOther)
}

// lineUp queues one run for each engine: the same brief, on the same commit
// of the template's sample, with the same model, skills and check.
func (s *Server) lineUp(r *http.Request, brief, named string, harnesses []string) (string, error) {
	fit := comparable()
	if s.lineups == nil || len(fit) == 0 {
		return "", errors.New("this server cannot compare agents")
	}
	held := fit[0]
	brief = strings.TrimSpace(brief)
	if brief == "" {
		return "", errors.New("say what the agents should do")
	}
	if len(harnesses) < 2 {
		return "", errors.New("choose at least two agents to compare")
	}
	if err := s.install(held); err != nil {
		return "", err
	}
	repo, commit, err := s.lineups.Sample(held.Name, held.Files)
	if err != nil {
		return "", err
	}
	id, err := s.lineups.Reserve(jobs.Name("compare "+brief, time.Now()))
	if err != nil {
		return "", err
	}
	lineup := compare.Lineup{ID: id, Brief: brief, Template: held.Name,
		Model: named, Commit: commit, Check: held.Check, At: time.Now().Unix(), Who: s.keeper.who(r)}
	models := s.offeredTo(r)
	for _, harness := range harnesses {
		spec, err := s.sideSpec(models, held, harness, named, brief, repo, commit)
		if err != nil {
			return "", err
		}
		run, err := s.queued(spec, nil, s.keeper.who(r), "")
		if err != nil {
			return "", err
		}
		lineup.Runs = append(lineup.Runs, compare.Entry{Harness: harness, Job: run})
	}
	return lineup.ID, s.lineups.Save(lineup)
}

// sideSpec is one engine's run in a lineup, refused here rather than in the
// queue when the engine is not offered or cannot reach the model.
func (s *Server) sideSpec(models Models, held industry.Template, harness, named, brief, repo, commit string) (job.Spec, error) {
	if !slices.ContainsFunc(s.runnersOf(held), func(entry catalog.Entry) bool { return entry.Name == harness }) {
		return job.Spec{}, fmt.Errorf("%s is not offered here", harness)
	}
	model, found := modelFor(models, named)
	if !found {
		return job.Spec{}, fmt.Errorf("%s is not offered here", named)
	}
	spec := job.Spec{Brief: brief, Repo: repo, Ref: commit, Gate: held.Check, Harness: harness, Model: model,
		Sensitivity: held.Sensitivity, Setup: &job.Setup{Name: held.Name}}
	adapter, err := runnable(spec, nil)
	if err != nil {
		return job.Spec{}, err
	}
	return spec, rules.Allows(spec, adapter.EndpointOf())
}
