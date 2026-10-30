package web

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"unicode"

	"github.com/getlatentic/gusto/internal/instance"
)

// A room's members are shown and changed in two dialogs over its conversation, as Kylon does: who is in it, and adding people and
// the workspace's agents.

// roomPerson is a member as the members dialog lists them.
type roomPerson struct {
	instance.Member
	You bool
}

// roomAgent is an agent as the members dialog and a conversation show it.
type roomAgent struct {
	instance.Agent
	Engine string
	Brain  string
	Owner  string
	// Yours says the person looking owns it, and may take it out of the room.
	Yours bool
}

// Allowance is what the agent may spend in the room a month, in words, or nothing for no limit.
func (a roomAgent) Allowance() string {
	if a.AllowanceUSD == nil {
		return ""
	}
	return fmt.Sprintf("$%.2f a month here", *a.AllowanceUSD)
}

func (s *Server) roomAgents(held instance.Instance, you string) []roomAgent {
	shown := make([]roomAgent, 0, len(held.Agents))
	for _, agent := range held.Agents {
		owner := s.ownerOf(agent)
		shown = append(shown, roomAgent{Agent: agent, Engine: s.engineName(agent.Harness), Brain: modelName(agent.Model),
			Owner: s.ownerLabel(owner, you), Yours: owner != "" && strings.EqualFold(owner, you)})
	}
	return shown
}

// roomMembers is what a room's two members dialogs show, for the conversation page.
func (s *Server) roomMembers(r *http.Request, held instance.Instance) map[string]any {
	you := s.askerName(r)
	people := make([]roomPerson, 0, len(held.Members))
	for _, member := range held.Members {
		people = append(people, roomPerson{Member: member, You: strings.EqualFold(member.Who, you)})
	}
	runs := s.runsRoom(r, held)
	var addable []listedAgent
	for _, member := range s.crewFor(you) {
		in := slices.ContainsFunc(held.Agents, func(agent instance.Agent) bool { return agent.Member == member.ID })
		// A member proposes only an agent of their own.
		if !in && (runs || strings.EqualFold(member.Owner, you)) {
			addable = append(addable, s.listedAgent(r, member))
		}
	}
	return map[string]any{
		"People":  people,
		"Agents":  s.roomAgents(held, you),
		"Runs":    runs,
		"Addable": addable,
	}
}

// roomMembersOf is what the members dialogs show, or nothing outside a room.
func (s *Server) roomMembersOf(r *http.Request, held instance.Instance) map[string]any {
	if !held.Room {
		return nil
	}
	return s.roomMembers(r, held)
}

// roomMembersPage opens the room with its members dialog showing.
func (s *Server) roomMembersPage(w http.ResponseWriter, r *http.Request) {
	held, found := s.roomOf(w, r)
	if !found {
		return
	}
	http.Redirect(w, r, s.membersPath(held), http.StatusSeeOther)
}

// membersPath is the room with its members dialog open, where a change to its members comes back to.
func (s *Server) membersPath(held instance.Instance) string {
	return threadPath(s.instances.Latest(held.ID)) + "?members=1"
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
	for _, who := range people {
		// Adding somebody who is in the room already would set their role, and so take an admin's away.
		if inRoom(held, who) {
			why := s.personName(who) + " is in this room already"
			if strings.EqualFold(who, s.askerName(r)) {
				why = "you are in this room already"
			}
			s.turnAway(w, r, s.membersPath(held), why)
			return
		}
		if err := s.instances.SetMember(held.ID, who, instance.RoleMember); err != nil {
			s.turnAway(w, r, s.membersPath(held), err.Error())
			return
		}
	}
	for _, id := range r.Form["agent"] {
		if err := s.addCrewAgent(r, held, id); err != nil {
			s.turnAway(w, r, s.membersPath(held), err.Error())
			return
		}
	}
	http.Redirect(w, r, s.membersPath(held), http.StatusSeeOther)
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
	if role == "remove" {
		if err = s.instances.RemoveMember(held.ID, who); err == nil {
			s.takeOutAgentsOf(held.ID, who, s.askerName(r))
		}
	} else {
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
