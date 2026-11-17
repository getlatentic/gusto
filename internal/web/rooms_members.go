package web

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"unicode"

	"github.com/getlatentic/gusto/internal/instance"
)

// A room's members are shown and changed in two dialogs over its conversation, as Kylon does: who is in it, and adding people and
// the team's agents.

// memberView is a person of a room as its members dialog lists them.
type memberView struct {
	Who, Name, Role string
	Me              bool
}

// memberAgentView is an agent of a room as its members dialog lists it. Engine is its type, Brain its model and Job what it is
// for; Owner is whose agent it is in the dialog's words, and nothing for the viewer's own, which Mine marks.
type memberAgentView struct {
	ID, Name, Harness, Mark, MarkDark string
	Engine, Brain, Job                string
	Owner                             string
	Pending                           bool
	// Allowance is what the agent may spend in the room a month, in words, and AllowanceField the same as its field holds it;
	// both are empty for no limit.
	Allowance, AllowanceField string
	Mine                      bool
}

// addableView is an agent of the team the add dialog offers.
type addableView struct {
	ID, Name, Harness, Mark, MarkDark string
	Engine, Specialty                 string
}

// membersView is what a room's two members dialogs show: who is in it, and who can be added. RoomAgents is how many agents the
// room has, pending ones included.
type membersView struct {
	People     []memberView
	Agents     []memberAgentView
	Runs       bool
	Addable    []addableView
	RoomAgents int
}

// roomMembersOf is what the members dialogs show, or nothing outside a room.
func (s *Server) roomMembersOf(r *http.Request, held instance.Instance) *membersView {
	if !held.Room {
		return nil
	}
	you := s.askerName(r)
	shown := &membersView{People: []memberView{}, Agents: s.memberAgents(held, you), Runs: s.runsRoom(r, held), Addable: []addableView{},
		RoomAgents: len(held.Agents)}
	for _, member := range held.Members {
		shown.People = append(shown.People, memberView{Who: member.Who, Name: s.personName(member.Who), Role: member.Role, Me: strings.EqualFold(member.Who, you)})
	}
	for _, member := range s.crewFor(you, held.Team) {
		in := slices.ContainsFunc(held.Agents, func(agent instance.Agent) bool { return agent.Member == member.ID })
		// A member proposes only an agent of their own.
		if !in && (shown.Runs || strings.EqualFold(member.Owner, you)) {
			shown.Addable = append(shown.Addable, addableView{ID: member.ID, Name: member.Name, Harness: member.Harness,
				Mark: logo(member.Harness), MarkDark: logoDark(member.Harness), Engine: s.engineName(member.Harness), Specialty: member.Specialty})
		}
	}
	return shown
}

func (s *Server) memberAgents(held instance.Instance, you string) []memberAgentView {
	shown := make([]memberAgentView, 0, len(held.Agents))
	for _, agent := range held.Agents {
		owner := s.ownerOf(agent)
		mine := owner != "" && strings.EqualFold(owner, you)
		view := memberAgentView{ID: agent.ID, Name: agent.Name, Harness: agent.Harness, Mark: logo(agent.Harness), MarkDark: logoDark(agent.Harness),
			Engine: s.engineName(agent.Harness), Brain: modelName(agent.Model), Job: agent.Job, Pending: agent.Pending,
			Allowance: allowanceWords(agent.AllowanceUSD), AllowanceField: allowanceField(agent.AllowanceUSD), Mine: mine}
		if !mine {
			view.Owner = s.ownerLabel(owner, you)
		}
		shown = append(shown, view)
	}
	return shown
}

// allowanceWords is a monthly allowance in words, or nothing for no limit.
func allowanceWords(allowance *float64) string {
	if allowance == nil {
		return ""
	}
	return fmt.Sprintf("$%.2f a month here", *allowance)
}

// allowanceField is a monthly allowance as its field holds it, or nothing for no limit.
func allowanceField(allowance *float64) string {
	if allowance == nil {
		return ""
	}
	return fmt.Sprintf("%.2f", *allowance)
}

// roomMembersPage opens the room with its members dialog showing.
func (s *Server) roomMembersPage(w http.ResponseWriter, r *http.Request) {
	held, found := s.roomOf(w, r)
	if !found {
		return
	}
	http.Redirect(w, r, s.membersPath(held), http.StatusSeeOther)
}

// addPath is the room with its add dialog open, where what was refused there comes back to.
func (s *Server) addPath(held instance.Instance) string {
	return threadPath(s.instances.Latest(held.ID)) + "?add=1"
}

// membersPath is the room with its members dialog open, where a change to its members comes back to.
func (s *Server) membersPath(held instance.Instance) string {
	return threadPath(s.instances.Latest(held.ID)) + "?members=1"
}

// cannotAdd is why one of the people cannot be added to a room: they are in it already, or not in its team.
func (s *Server) cannotAdd(r *http.Request, held instance.Instance, people []string) string {
	for _, who := range people {
		if inRoom(held, who) {
			return s.alreadyIn(r, who)
		}
		if why := s.outsideTeam(held, who); why != "" {
			return why
		}
	}
	return ""
}

// addRoomMembers adds the people and the presets ticked in the add dialog in one go.
func (s *Server) addRoomMembers(w http.ResponseWriter, r *http.Request) {
	held, found := s.roomOf(w, r)
	if !found {
		return
	}
	people := strings.FieldsFunc(r.FormValue("who"), func(c rune) bool { return c == ',' || unicode.IsSpace(c) })
	if len(people) > 0 && !s.runsRoom(r, held) {
		http.Error(w, "only a room's admin adds people to it", http.StatusForbidden)
		return
	}
	if why := s.cannotAdd(r, held, people); why != "" {
		s.turnAway(w, r, s.addPath(held), why)
		return
	}
	var arrivals []instance.Arrival
	for _, id := range r.Form["agent"] {
		arrival, err := s.crewArrival(r, held, id)
		if err != nil {
			s.turnAway(w, r, s.addPath(held), err.Error())
			return
		}
		arrivals = append(arrivals, arrival)
	}
	if _, err := s.instances.Join(held.ID, people, arrivals); err != nil {
		why := err.Error()
		if already, is := err.(instance.AlreadyInError); is {
			why = s.alreadyIn(r, already.Who)
		}
		s.turnAway(w, r, s.addPath(held), why)
		return
	}
	// Adding finishes the task, so the room comes back with its conversation in view and no dialog over it.
	http.Redirect(w, r, threadPath(s.instances.Latest(held.ID)), http.StatusSeeOther)
}

// alreadyIn is why a person is not added to a room they are in: adding them would set their role, and so take an admin's away.
func (s *Server) alreadyIn(r *http.Request, who string) string {
	if strings.EqualFold(who, s.askerName(r)) {
		return "you are in this room already"
	}
	return s.personName(who) + " is in this room already"
}

// changeRoomRole makes a member an admin or a member, or takes them out of the room.
func (s *Server) changeRoomRole(w http.ResponseWriter, r *http.Request) {
	held, found := s.roomOf(w, r)
	if !found {
		return
	}
	if !s.runsRoom(r, held) {
		http.Error(w, "only a room's admin changes roles in it", http.StatusForbidden)
		return
	}
	who, role := r.FormValue("who"), r.FormValue("role")
	var err error
	switch why := s.outsideTeam(held, who); {
	case role == "remove":
		err = s.takeOutMember(held.ID, who, s.askerName(r))
	case why != "":
		err = errors.New(why)
	default:
		err = s.instances.SetMember(held.ID, who, role)
	}
	if err != nil {
		s.turnAway(w, r, s.membersPath(held), err.Error())
		return
	}
	if role == "remove" && strings.EqualFold(who, s.askerName(r)) && !s.admin(r) {
		http.Redirect(w, r, "/chat", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, s.membersPath(held), http.StatusSeeOther)
}

// inRoom is whether a person is one of the room's people.
func inRoom(held instance.Instance, who string) bool {
	for _, member := range held.Members {
		if strings.EqualFold(member.Who, strings.TrimSpace(who)) {
			return true
		}
	}
	return false
}
