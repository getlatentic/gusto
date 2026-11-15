package web

import (
	"net/http"

	"github.com/getlatentic/gusto/internal/apps"
)

// A project's page data, in the shape web/src/pages/work/types.ts names: one agent's computer, what it runs and works in, and
// every conversation, routine, app and checked task it has.

const projectRoute = "/instances/{id}"

type projectView struct {
	ID, Model, Sensitivity, Setup, Repo, Started string
}

// projectAgentView is the agent as its page names it: Field is the kind of work it was set up for, drawn when its type has
// no mark.
type projectAgentView struct {
	Name, Harness, Field, Engine, Brain, Mark, MarkDark string
}

// conversationRow is one conversation of the project, newest first, with the last thing said in it.
type conversationRow struct {
	Path, Name   string
	Latest       string
	LatestQueued int64
}

type routineView struct {
	ID, Title, Prompt, Thread, When, Last string
	Paused                                bool
}

type projectAppView struct {
	ID, State, Link, Folder, Command, Note string
	Port                                   int
}

type checkedTaskView struct {
	ID, Brief, State string
	Queued           int64
}

// hasProject is whether the address names a project the person asking may change.
func (s *Server) hasProject(r *http.Request) bool {
	_, found := s.owned(r)
	return found
}

// projectData is a project's page. Somebody else's agent gets the answer a missing one does, so a guess cannot tell the two
// apart. Refresh says an app is still starting, so the page reads itself again until it answers.
func (s *Server) projectData(r *http.Request) (map[string]any, error) {
	held, found := s.owned(r)
	if !found {
		return nil, failed{http.StatusNotFound, "there is no such agent"}
	}
	named := s.agentOf(held)
	routines, unread := s.routineRows(held.ID)
	unreadWords := ""
	if unread != nil {
		unreadWords = unread.Error()
	}
	running := s.appsOf(held.ID)
	offered := s.offeredTo(r)
	return map[string]any{
		"Instance": projectView{ID: held.ID, Model: held.Model, Sensitivity: held.Sensitivity, Setup: held.Setup, Repo: held.Repo,
			Started: whenOf(held.Started)},
		"Agent": projectAgentView{Name: named.Name, Harness: held.Harness, Field: named.Field, Engine: named.Engine, Brain: named.Brain,
			Mark: logo(held.Harness), MarkDark: logoDark(held.Harness)},
		"Conversations": s.conversationRows(held.ID),
		"Models":        s.modelChoices(offered),
		"Sharing":       s.apps != nil,
		"Runs":          s.runner != nil,
		"Apps":          s.projectApps(running),
		"Refresh":       startingApp(running),
		"Tasks":         s.checkedTasks(held.ID),
		"Routines":      routineViews(routines),
		"Unread":        unreadWords,
		"Refused":       s.refusal(r),
	}, nil
}

func (s *Server) conversationRows(id string) []conversationRow {
	threads := s.namedThreads(id)
	rows := make([]conversationRow, 0, len(threads))
	for i := len(threads) - 1; i >= 0; i-- {
		latest := s.latestTurn(threads[i].Thread)
		rows = append(rows, conversationRow{Path: threadPath(threads[i].Thread), Name: threads[i].Name, Latest: latest.Brief,
			LatestQueued: latest.Queued})
	}
	return rows
}

func routineViews(rows []routineRow) []routineView {
	views := make([]routineView, 0, len(rows))
	for _, row := range rows {
		views = append(views, routineView{ID: row.ID, Title: row.Title, Prompt: row.Prompt, Thread: row.Thread, When: row.When,
			Last: row.Last, Paused: row.Paused})
	}
	return views
}

func (s *Server) projectApps(running []apps.App) []projectAppView {
	views := make([]projectAppView, 0, len(running))
	for _, app := range running {
		views = append(views, projectAppView{ID: app.ID, State: string(app.State), Link: s.appLink(app), Folder: app.Folder,
			Command: app.Command, Note: app.Note, Port: app.Port})
	}
	return views
}

func (s *Server) checkedTasks(id string) []checkedTaskView {
	done := s.tasks(id)
	views := make([]checkedTaskView, 0, len(done))
	for _, record := range done {
		views = append(views, checkedTaskView{ID: record.ID, Brief: record.Brief, State: string(record.State), Queued: record.Queued})
	}
	return views
}

// modelChoices are the models the agent may be pointed at, each with what it costs as the person choosing reads it.
func (s *Server) modelChoices(models []string) []option {
	choices := make([]option, 0, len(models))
	for _, model := range models {
		label := modelName(model)
		if cost := s.costOf(model); cost != "" {
			label += " · " + cost
		}
		choices = append(choices, option{Value: model, Label: label})
	}
	return choices
}
