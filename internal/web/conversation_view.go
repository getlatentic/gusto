package web

import (
	"cmp"

	"github.com/getlatentic/gusto/internal/apps"
	"github.com/getlatentic/gusto/internal/instance"
)

// The conversation page's data and its turns, in the shapes web/src/pages/conversation/types.ts names. A turn is the same for
// every viewer, since a live frame carries it to everybody in a room: whose agent answered is an address, and the page says
// "your agent" itself.

// turnView is one turn of a conversation, or one note or notice of a room, in its place among the others.
type turnView struct {
	ID string
	// Kind is "turn", "note" or "notice".
	Kind    string
	Divider string

	Who         string
	WhoName     string
	WhoInitials string
	Said        string
	SaidHTML    string
	ShowSaid    bool
	SaidJoins   bool
	Queued      int64

	By         string
	Role       string
	Harness    string
	Mark       string
	MarkDark   string
	Owner      string
	OwnerName  string
	From       string
	ReplyJoins bool
	Done       bool
	Holding    string
	Started    int64
	Ended      int64

	Took      string
	Money     string
	Steps     []step
	Apps      string
	ReplyHTML string
	Capped    *cappedView
	StoppedBy string
	Stopped   string
	Home      *fullHome
	Blocked   int
	Refused   []blocked
	Previews  []string
	Files     *turnFiles
	File      string
}

// cappedView is the spending limit that stopped a reply: Credit says it was gusto's free credit, used up.
type cappedView struct {
	Credit             bool
	SpentUSD, LimitUSD float64
	Words              string
}

// Turn kinds, as the page tells them apart.
const (
	kindTurnView   = "turn"
	kindNoteView   = "note"
	kindNoticeView = "notice"
)

// turnScene is what a turn is drawn with from the conversation it is in.
type turnScene struct {
	held  instance.Instance
	path  string
	dir   string
	names []string
	// agent answers every turn of a conversation that is not a room's.
	agent agent
}

func (s *Server) sceneOf(held instance.Instance, thread instance.Thread) turnScene {
	return turnScene{held: held, path: threadPath(thread), dir: thread.Dir, names: s.roomNames(held), agent: s.agentOf(held)}
}

// turnViews is each turn as the page draws it.
func (s *Server) turnViews(turns []turn, scene turnScene) []turnView {
	views := make([]turnView, 0, len(turns))
	for _, each := range turns {
		views = append(views, s.turnViewOf(each, scene))
	}
	return views
}

func (s *Server) turnViewOf(shown turn, scene turnScene) turnView {
	who := shown.Who
	if !scene.held.Room {
		who = shown.Asked
	}
	view := turnView{
		ID: shown.ID, Kind: turnKindOf(shown), Divider: shown.Divider,
		Who: who, WhoName: s.personName(who), WhoInitials: s.initials(who),
		Said: shown.Said, SaidHTML: mentionsIn(shown.Said, scene.names), ShowSaid: shown.From == "" && !shown.Echo,
		SaidJoins: shown.SaidJoins, Queued: shown.Queued, File: shown.File,
	}
	if shown.Note {
		// A note has no reply to wait for.
		view.Done = true
		return view
	}
	s.replyOf(&view, shown, scene)
	if view.Done {
		s.resultOf(&view, shown, scene)
	}
	return view
}

func turnKindOf(shown turn) string {
	switch {
	case shown.Notice:
		return kindNoticeView
	case shown.Note:
		return kindNoteView
	}
	return kindTurnView
}

// replyOf fills in who answers a turn and where its answer stands.
func (s *Server) replyOf(view *turnView, shown turn, scene turnScene) {
	view.By, view.Role, view.Harness = shown.By, shown.Role, shown.Harness
	if !scene.held.Room {
		view.By, view.Role, view.Harness = scene.agent.Name, scene.agent.Engine, scene.held.Harness
	}
	view.Mark, view.MarkDark = logo(view.Harness), logoDark(view.Harness)
	view.Owner, view.From = shown.Owner, shown.From
	if view.Owner != "" {
		view.OwnerName = s.personName(view.Owner)
	}
	view.ReplyJoins, view.Holding = shown.ReplyJoins, shown.Holding
	view.Done, view.Started, view.Ended, view.StoppedBy = shown.State.Done(), shown.Started, shown.Ended, shown.StoppedBy
}

// resultOf fills in what a finished turn came to.
func (s *Server) resultOf(view *turnView, shown turn, scene turnScene) {
	view.Took = took(shown.Started, shown.Ended)
	if shown.Metered && shown.Spent.Unmeasured == 0 && shown.Spent.USD > 0 {
		view.Money = shown.Spent.Money()
	}
	view.Steps, view.Apps = shown.Steps, shown.Apps
	if shown.Replied != "" {
		view.ReplyHTML = string(s.replyIn(shown.Replied, scene.held.ID, scene.dir, scene.path))
	}
	stoppedOf(view, shown)
	view.Blocked, view.Refused = shown.Blocked, shown.Refused
	// Only the owner is offered a preview; the page knows whether that is who is looking.
	if s.apps != nil {
		view.Previews = shown.Previews
	}
	view.Files = shown.Files
}

// stoppedOf fills in what stopped a turn, told once: who stopped it, else a full home, else a spending limit, else the agent's
// last words. A spending limit is told beside a full home, as the page always told it.
func stoppedOf(view *turnView, shown turn) {
	if capped := shown.Capped; capped != nil {
		view.Capped = &cappedView{Credit: capped.Cap == "credit" && capped.SpentUSD >= capped.LimitUSD,
			SpentUSD: capped.SpentUSD, LimitUSD: capped.LimitUSD, Words: shown.Limit}
	}
	if view.StoppedBy != "" {
		return
	}
	view.Home = shown.Home
	if view.Home == nil && view.Capped == nil {
		view.Stopped = shown.Stopped
	}
}

// The page's own parts.

type instanceView struct {
	ID, Title          string
	Room, Direct       bool
	Sensitivity, Model string
}

type threadView struct{ ID, Dir string }

type agentView struct{ Name, Harness, Brain, Mark, MarkDark string }

// directAgentView is the one agent of a person's own room with one agent, which the page is named after.
type directAgentView struct{ Name, Harness, Mark, MarkDark string }

type appLinkView struct{ Name, Link string }

func instanceViewOf(held instance.Instance) instanceView {
	return instanceView{ID: held.ID, Title: held.Title, Room: held.Room, Direct: held.Direct, Sensitivity: held.Sensitivity, Model: held.Model}
}

func agentViewOf(shown agent) agentView {
	return agentView{Name: shown.Name, Harness: shown.Harness, Brain: shown.Brain, Mark: logo(shown.Harness), MarkDark: logoDark(shown.Harness)}
}

func directAgentOf(held instance.Instance) *directAgentView {
	if !held.Direct || len(held.Agents) == 0 {
		return nil
	}
	first := held.Agents[0]
	return &directAgentView{Name: first.Name, Harness: first.Harness, Mark: logo(first.Harness), MarkDark: logoDark(first.Harness)}
}

// appLinks is the apps running from the conversation's folder, each with where it is opened.
func (s *Server) appLinks(running []apps.App) []appLinkView {
	links := make([]appLinkView, 0, len(running))
	for _, app := range running {
		links = append(links, appLinkView{Name: cmp.Or(app.Folder, app.Command, app.ID), Link: s.appLink(app)})
	}
	return links
}

// hintOf is what the message box suggests: a room names whom it can mention, and a conversation its agent.
func hintOf(held instance.Instance, answers agent) string {
	if !held.Room {
		return "Message " + answers.Name
	}
	return composerHint(held)
}
