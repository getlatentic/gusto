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
}

func (s *Server) roomAgents(held instance.Instance, you string) []roomAgent {
	shown := make([]roomAgent, 0, len(held.Agents))
	for _, agent := range held.Agents {
		owner := ownerLabel(s.ownerOf(agent), you)
		shown = append(shown, roomAgent{Agent: agent, Engine: s.engineName(agent.Harness), Brain: modelName(agent.Model), Owner: owner})
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
	var addable []listedAgent
	for _, member := range s.crewFor(you) {
		if !slices.ContainsFunc(held.Agents, func(agent instance.Agent) bool { return agent.Member == member.ID }) {
			addable = append(addable, s.listedAgent(r, member))
		}
	}
	return map[string]any{
		"People":  people,
		"Agents":  s.roomAgents(held, you),
		"Runs":    s.runsRoom(r, held),
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
	if !s.runsRoom(r, held) {
		http.Error(w, "only a room's admin adds to it", http.StatusForbidden)
		return
	}
	for _, who := range strings.FieldsFunc(r.FormValue("who"), func(c rune) bool { return c == ',' || unicode.IsSpace(c) }) {
		if err := s.instances.SetMember(held.ID, who, instance.RoleMember); err != nil {
			s.turnAway(w, r, s.membersPath(held), err.Error())
			return
		}
	}
	for _, id := range r.Form["agent"] {
		if err := s.addCrewAgent(r, held.ID, id); err != nil {
			s.turnAway(w, r, s.membersPath(held), err.Error())
			return
		}
	}
	http.Redirect(w, r, s.membersPath(held), http.StatusSeeOther)
}

// addCrewAgent puts a workspace agent the person may work with into a room.
func (s *Server) addCrewAgent(r *http.Request, id, memberID string) error {
	if s.crew == nil {
		return fmt.Errorf("this server keeps no agents")
	}
	member, found := s.crew.Get(memberID)
	if !found || !member.MayUse(s.askerName(r)) {
		return fmt.Errorf("that agent is not one you may work with")
	}
	_, err := s.instances.AddAgent(id, instance.Agent{Name: member.Name, Harness: member.Harness, Model: member.Model, Member: member.ID})
	return err
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
		err = s.instances.RemoveMember(held.ID, who)
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
