package web

import (
	"cmp"
	"net/http"
	"slices"
	"strings"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
)

// roomTurn is what a room's conversation shows of a turn on top of what any conversation does: who said it, which agent
// answered, and which agent handed it on.
type roomTurn struct {
	Who    string
	By     string
	Engine string
	From   string
	// Note is something said that no agent was asked to answer, and Echo a turn whose message the turn before it already shows.
	Note bool
	Echo bool
}

// roomTimeline names who spoke in each of a room's turns and puts what was said to nobody in among them, in the order it was
// said.
func (s *Server) roomTimeline(held instance.Instance, thread instance.Thread, turns []turn) []turn {
	for i := range turns {
		turns[i].roomTurn = s.roomTurnOf(held, turns[i].Record)
	}
	// Notes go in last first, so each lands after the turns it followed without moving the places of the ones before it.
	for _, note := range slices.Backward(thread.Notes) {
		shown := turn{Record: jobs.Record{ID: note.ID, Queued: note.At}, Said: note.Text, roomTurn: roomTurn{Who: note.Who, Note: true}}
		turns = slices.Insert(turns, min(note.After, len(turns)), shown)
	}
	return turns
}

func (s *Server) roomTurnOf(held instance.Instance, record jobs.Record) roomTurn {
	shown := roomTurn{Who: record.Asked, By: record.Agent, Engine: s.engineName(record.Harness), Echo: record.Answers != ""}
	if agent, found := held.AgentByID(record.Agent); found {
		shown.By = agent.Name
	}
	if record.HandedBy != "" {
		shown.From = record.HandedBy
		if agent, found := held.AgentByID(record.HandedBy); found {
			shown.From = agent.Name
		}
	}
	return shown
}

// roomWaiting is the first of a room's turns that has not finished, since several agents can be working at once.
func roomWaiting(turns []turn) string {
	for _, each := range turns {
		if !each.Note && !each.State.Done() {
			return each.ID
		}
	}
	return ""
}

// composerHint is what a room's message box suggests, naming its agents so people know whom they can mention.
func composerHint(held instance.Instance) string {
	switch len(held.Agents) {
	case 0:
		return "Message the room"
	case 1:
		return "Message " + held.Agents[0].Name
	}
	names := make([]string, 0, len(held.Agents))
	for _, agent := range held.Agents {
		names = append(names, "@"+agent.Name)
	}
	return "Message the room, mention " + strings.Join(names, " or ")
}

// roomsFor is each room the person asking is in, the one spoken in last first, as the side list shows them.
func (s *Server) roomsFor(r *http.Request) []railChat {
	who := s.askerName(r)
	var rooms []railChat
	for _, held := range s.instances.List() {
		if !held.Room || held.RoleOf(who) == "" {
			continue
		}
		used := s.lately(held)
		rooms = append(rooms, railChat{Name: held.Title, Where: threadPath(s.instances.Latest(held.ID)), When: used.Active})
	}
	slices.SortStableFunc(rooms, func(a, b railChat) int { return cmp.Compare(b.When, a.When) })
	return rooms
}
