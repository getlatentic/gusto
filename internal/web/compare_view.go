package web

import (
	"net/http"
	"strings"

	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/compare"
)

// The data of how each agent type did, of comparing agents, and of one comparison, in the shapes web/src/pages/work/types.ts
// names.

const lineupRoute = "/compare/{id}"

// enginesData is how each agent type did on the benchmark here.
func (s *Server) enginesData(*http.Request) (map[string]any, error) {
	return map[string]any{"Measured": measuredOf(s.engines())}, nil
}

// compareTemplateView is the sample project a comparison runs on: Starter is the task it suggests, and Check the command that
// decides, as a shell line.
type compareTemplateView struct{ Title, Starter, Check string }

type engineChoice struct{ Name, Title, Mark, MarkDark string }

type lineupRow struct {
	ID, Brief, Model string
	Agents           int
	At               int64
	When             string
}

// compareData is the page that starts a comparison, with the person's earlier ones.
func (s *Server) compareData(r *http.Request) (map[string]any, error) {
	page := map[string]any{"Refused": s.refusal(r), "Lineups": []lineupRow{}}
	if s.lineups != nil {
		page["Lineups"] = lineupRows(s.ownLineups(r, s.lineups.List()))
	}
	if fit := comparable(); len(fit) > 0 {
		held := fit[0]
		starter := ""
		if len(held.Starters) > 0 {
			starter = held.Starters[0]
		}
		page["Template"] = compareTemplateView{Title: held.Title, Starter: starter, Check: strings.Join(held.Check, " ")}
		page["Engines"] = engineChoices(s.runnersOf(held))
		page["Models"] = s.sharedModels(held, s.offeredTo(r))
	}
	return page, nil
}

func lineupRows(held []compare.Lineup) []lineupRow {
	rows := make([]lineupRow, 0, len(held))
	for _, each := range held {
		rows = append(rows, lineupRow{ID: each.ID, Brief: each.Brief, Model: each.Model, Agents: len(each.Runs), At: each.At, When: whenOf(each.At)})
	}
	return rows
}

func engineChoices(entries []catalog.Entry) []engineChoice {
	choices := make([]engineChoice, 0, len(entries))
	for _, entry := range entries {
		choices = append(choices, engineChoice{Name: entry.Name, Title: entry.Title, Mark: logo(entry.Name), MarkDark: logoDark(entry.Name)})
	}
	return choices
}

// sideView is one agent's run in a comparison. Seconds is its time, the box starting included, and Money and Calls what the
// proxy counted when it counted.
type sideView struct {
	Engine, Harness, Mark, MarkDark, Model string
	Task, Holding                          string
	Done, Checked, Passed                  bool
	Seconds                                int64
	Metered                                bool
	Money                                  string
	Calls                                  int
	Files, Added, Removed                  int
	Fastest, Cheapest                      bool
}

type lineupView struct{ ID, Brief, Model string }

// hasLineup is whether the address names a comparison the person asking may read.
func (s *Server) hasLineup(r *http.Request) bool {
	_, found := s.lineupOf(r)
	return found
}

func (s *Server) lineupOf(r *http.Request) (compare.Lineup, bool) {
	if s.lineups == nil {
		return compare.Lineup{}, false
	}
	lineup, found := s.lineups.Get(r.PathValue("id"))
	if !found || !s.inTeam(r, lineup.Team) || !s.mayUse(r, lineup.Who) {
		return compare.Lineup{}, false
	}
	return lineup, true
}

// lineupData is one comparison: each agent's run side by side, and, once every run is over, which of them stands clear. A
// verdict on runs that are still working would name a winner that a run still to finish could overturn.
func (s *Server) lineupData(r *http.Request) (map[string]any, error) {
	lineup, found := s.lineupOf(r)
	if !found {
		return nil, failed{http.StatusNotFound, "there is no such comparison"}
	}
	ledger, _ := s.spending()
	sides, running := s.sidesOf(lineup, ledger)
	page := map[string]any{"Lineup": lineupView{ID: lineup.ID, Brief: lineup.Brief, Model: lineup.Model}, "Running": running,
		"Passed": passedCount(sides), "Reading": nil}
	if !running && len(sides) > 1 {
		reading := weigh(sides)
		page["Reading"] = &reading
	}
	page["Sides"] = sideViews(sides)
	return page, nil
}

func sideViews(sides []side) []sideView {
	views := make([]sideView, 0, len(sides))
	for _, one := range sides {
		view := sideView{Engine: one.Engine, Harness: one.Harness, Mark: logo(one.Harness), MarkDark: logoDark(one.Harness),
			Model: one.Model, Task: one.Record.ID, Holding: one.Holding, Done: one.Record.State.Done(), Checked: one.Checked,
			Passed: one.Passed, Seconds: one.Seconds(), Metered: one.Metered, Files: one.Files, Added: one.Added,
			Removed: one.Removed, Fastest: one.Fastest, Cheapest: one.Cheapest}
		if one.Metered {
			view.Money, view.Calls = one.Spent.Money(), one.Spent.Calls
		}
		views = append(views, view)
	}
	return views
}
