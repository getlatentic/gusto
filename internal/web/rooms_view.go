package web

import (
	"cmp"
	"html/template"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
)

// roomTurn is what a room's conversation shows of a turn on top of what any conversation does: who said it, which agent
// answered, and which agent handed it on.
type roomTurn struct {
	Who    string
	By     string
	Engine string
	// Role is what the agent does, or its harness when nobody said.
	Role string
	From string
	// Note is something said that no agent was asked to answer, and Echo a turn whose message the turn before it already shows.
	Note bool
	Echo bool
	// SaidJoins and ReplyJoins say the message or reply follows one from the same sender, so it goes under that sender's name.
	SaidJoins  bool
	ReplyJoins bool
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
	groupBySender(turns)
	return turns
}

func (s *Server) roomTurnOf(held instance.Instance, record jobs.Record) roomTurn {
	shown := roomTurn{Who: record.Asked, By: record.Agent, Engine: s.engineName(record.Harness), Echo: record.Answers != ""}
	shown.Role = shown.Engine
	if agent, found := held.AgentByID(record.Agent); found {
		shown.By = agent.Name
		shown.Role = firstOf(agent.Job, shown.Engine)
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

// groupBySender marks each message and reply sent straight after another by the same person or agent.
func groupBySender(turns []turn) {
	last := ""
	for i := range turns {
		each := &turns[i].roomTurn
		if each.From == "" && !each.Echo {
			each.SaidJoins = last == "person:"+each.Who
			last = "person:" + each.Who
		}
		if !each.Note {
			each.ReplyJoins = last == "agent:"+each.By
			last = "agent:" + each.By
		}
	}
}

// mentionMark is an @name in a message, which a room shows set apart.
var mentionMark = regexp.MustCompile(`(^|[^\w@])(@[A-Za-z][A-Za-z0-9_-]{0,31})`)

// mentions is a message as a room shows it, each @name set apart.
func mentions(text string) template.HTML {
	return template.HTML(mentionMark.ReplaceAllString(template.HTMLEscapeString(text), `$1<span class="rounded-xs bg-accent-wash px-0.5 font-medium text-accent">$2</span>`))
}

// personName is what a room calls a person: the part of their address before the @, or Owner for the server's shared sign-in.
func personName(who string) string {
	if who == sharedSignIn {
		return "Owner"
	}
	name, _, _ := strings.Cut(who, "@")
	return name
}

// initials stand for a person where a room has no picture of them. The shared sign-in is nobody in particular, so it has none.
func initials(who string) string {
	if who == sharedSignIn {
		return ""
	}
	words := strings.FieldsFunc(personName(who), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	letters := ""
	for _, word := range words[:min(len(words), 2)] {
		first, _ := utf8.DecodeRuneInString(word)
		letters += string(unicode.ToUpper(first))
	}
	return letters
}
