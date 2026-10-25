package web

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode"

	"github.com/getlatentic/gusto/internal/industry"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/preset"
)

// A room's members are shown and changed in two dialogs over its conversation, as Kylon does: who is in it, and adding people and
// agents. An agent added from one of the person's presets keeps that preset's agent type, model and role.

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
}

// roomPreset is one of the person's presets as the add dialog offers it.
type roomPreset struct {
	preset.Preset
	Summary string
}

func (s *Server) roomAgents(held instance.Instance) []roomAgent {
	shown := make([]roomAgent, 0, len(held.Agents))
	for _, agent := range held.Agents {
		shown = append(shown, roomAgent{Agent: agent, Engine: s.engineName(agent.Harness), Brain: modelName(agent.Model)})
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
	var presets []roomPreset
	for _, saved := range s.savedBy(r) {
		presets = append(presets, roomPreset{Preset: saved, Summary: s.presetSummary(saved)})
	}
	return map[string]any{
		"People":  people,
		"Agents":  s.roomAgents(held),
		"Runs":    s.runsRoom(r, held),
		"Presets": presets,
		"Picks":   s.picksFor(r),
		"Offered": s.modelOptionsOf(s.offeredTo(r)),
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
	for _, id := range r.Form["preset"] {
		if err := s.addPresetAgent(r, held.ID, id); err != nil {
			s.turnAway(w, r, s.membersPath(held), err.Error())
			return
		}
	}
	http.Redirect(w, r, s.membersPath(held), http.StatusSeeOther)
}

func (s *Server) addPresetAgent(r *http.Request, id, presetID string) error {
	if s.presets == nil {
		return fmt.Errorf("presets are not kept here")
	}
	saved, found := s.presets.Get(s.keeper.who(r), presetID)
	if !found {
		return fmt.Errorf("that preset is not one of yours")
	}
	held, _ := s.instances.Get(id)
	job := ""
	if role, found := industry.Get(saved.Role); found {
		job = role.Title
	}
	agent := instance.Agent{Name: freeName(held, mentionName(saved.Name)), Harness: saved.Harness, Model: saved.Model, Job: job}
	_, err := s.instances.AddAgent(id, agent)
	return err
}

// unmentionable is what cannot be part of a name somebody types after an @.
var unmentionable = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// mentionName is a preset's name as an agent can be mentioned by: its words run together, starting with a letter.
func mentionName(name string) string {
	var joined strings.Builder
	for _, word := range strings.Fields(name) {
		word = unmentionable.ReplaceAllString(word, "")
		if word == "" {
			continue
		}
		joined.WriteString(strings.ToUpper(word[:1]) + word[1:])
	}
	made := joined.String()
	if made == "" || !unicode.IsLetter(rune(made[0])) {
		made = "Agent" + made
	}
	return made[:min(len(made), 28)]
}

// freeName is a name no agent in the room has yet, numbering it when it is taken.
func freeName(held instance.Instance, name string) string {
	taken := func(each string) bool { _, found := held.AgentCalled(each); return found }
	if !taken(name) {
		return name
	}
	for n := 2; ; n++ {
		if numbered := fmt.Sprintf("%s%d", name, n); !taken(numbered) {
			return numbered
		}
	}
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
