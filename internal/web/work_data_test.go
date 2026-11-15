package web

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

// The work pages' data, as web/src/pages/work/types.ts names it, decoded into the views the server builds it from.

// jobsJSON is Activity's data.
type jobsJSON struct{ Board boardView }

// activity is Activity's data at the query given.
func (s *site) activity(t *testing.T, query string) boardView {
	t.Helper()
	var page jobsJSON
	s.data(t, "/jobs"+query, &page)
	return page.Board
}

// runs is every run the board lists, in the order it lists them.
func (b boardView) runs() []runView {
	var all []runView
	for _, day := range b.Days {
		all = append(all, day.Runs...)
	}
	return all
}

func (b boardView) ids() []string {
	var ids []string
	for _, each := range b.runs() {
		ids = append(ids, each.ID)
	}
	return ids
}

// run is the board's line for a task, by its id.
func (b boardView) run(t *testing.T, id string) runView {
	t.Helper()
	all := b.runs()
	at := slices.IndexFunc(all, func(each runView) bool { return each.ID == id })
	if at < 0 {
		t.Fatalf("Activity does not list %s: %v", id, b.ids())
	}
	return all[at]
}

func (b boardView) figure(t *testing.T, label string) figure {
	t.Helper()
	at := slices.IndexFunc(b.Figures, func(each figure) bool { return each.Label == label })
	if at < 0 {
		t.Fatalf("Activity has no %s figure: %+v", label, b.Figures)
	}
	return b.Figures[at]
}

// taskJSON is a task page's data.
type taskJSON struct {
	MovedTo      string
	Job          jobView
	Outcome      string
	Took         string
	Waiting      string
	Brain        string
	Build        string
	ThreadPath   string
	On           origin
	Admin, Live  bool
	Cost         *costView
	Agent        *taskAgentView
	Talk         *talkView
	Steps        []step
	Calls        *callsView
	Files        []hashedFile
	Proof        *proofView
	StoppedByCap *capStopView
	Home         *fullHome
	Patch        *patchFilesView
	Transcript   *transcriptView
	Check        *checkPageView
}

// task is the data of a task's page.
func (s *site) task(t *testing.T, id string) taskJSON {
	t.Helper()
	var page taskJSON
	s.data(t, "/jobs/"+id, &page)
	return page
}

// patchFile is one file of the task's patch, by its name.
func (page taskJSON) patchFile(t *testing.T, name string) diffFile {
	t.Helper()
	if page.Patch == nil {
		t.Fatal("the task's page has no patch")
	}
	at := slices.IndexFunc(page.Patch.Files, func(each diffFile) bool { return each.Name == name })
	if at < 0 {
		t.Fatalf("the patch does not name %s", name)
	}
	return page.Patch.Files[at]
}

// hashOf is the fingerprint the page gives a file of the record, or nothing when it gives none.
func (page taskJSON) hashOf(name string) string {
	for _, each := range page.Files {
		if each.Name == name {
			return each.SHA256
		}
	}
	return ""
}

// projectJSON is a project page's data.
type projectJSON struct {
	Instance      projectView
	Agent         projectAgentView
	Conversations []conversationRow
	Models        []option
	Sharing, Runs bool
	Apps          []projectAppView
	Refresh       bool
	Tasks         []checkedTaskView
	Routines      []routineView
	Unread        string
	Refused       string
}

// project is the data of a project's page.
func (s *site) project(t *testing.T, id string) projectJSON {
	t.Helper()
	var page projectJSON
	s.data(t, "/instances/"+id, &page)
	return page
}

// compareJSON is the data of the page that starts a comparison.
type compareJSON struct {
	Refused  string
	Lineups  []lineupRow
	Template *compareTemplateView
	Engines  []engineChoice
	Models   []string
}

// lineupJSON is one comparison's data.
type lineupJSON struct {
	Lineup  lineupView
	Running bool
	Passed  int
	Reading *struct{ Headline string }
	Sides   []sideView
}

// enginesJSON is the data of how each agent type did.
type enginesJSON struct{ Measured measuredView }

// isTheApp says the page at path is the app's, which draws it from its data.
func isTheApp(t *testing.T, s *site, path string) {
	t.Helper()
	got := s.ask(t, "GET", path, nil)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `<div id="root">`) {
		t.Errorf("%s answered %d without the app", path, got.Code)
	}
}
