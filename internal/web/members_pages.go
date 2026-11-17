package web

import (
	"net/http"
	"slices"
	"strings"

	"github.com/getlatentic/gusto/internal/crew"
	"github.com/getlatentic/gusto/internal/instance"
)

// The members pages' data, in the shapes web/src/pages/members/types.ts names. /members is the list and its dialogs, which the
// app reads once and keeps drawn; an agent's page and a person's page are only whoever is open, with anything a form sent there
// was refused for.

const (
	agentRoute  = "/members/agents/{id}"
	personRoute = "/members/people/{who}"
)

// newAgentView is what the new agent dialog offers: every agent type, the one chosen first, and the models it may run on.
type newAgentView struct {
	Kinds      []kindView
	FirstReady string
	Offered    []option
}

// kindView is one agent type in the new agent dialog. Measured is how it did on the benchmark here, and Failed that it finished
// on none of the models offered. Installable is one gusto can install on the person's own computer.
type kindView struct {
	Name, Title, What, Why, Mark, MarkDark string
	Measured                               string
	Ready, Failed, Installable             bool
}

// agentPageView is the agent an agent's page is of. Mine is whether the person looking may change it, and Usable whether they
// may work with it.
type agentPageView struct {
	listedAgent
	Brain, Model, Persona, Setup    string
	Owner, OwnerName, OwnerInitials string
	Access                          []accessView
	Created                         int64
	Mine, Usable                    bool
}

// accessView is somebody besides its maker who may work with an agent.
type accessView struct{ Who, Name, Initials string }

// agentSkillsView is the agent's Skills tab: what it can be given, and what it is. Given is whether it has a skill set, Title
// that set's name as the page says it, and Skills what the set holds.
type agentSkillsView struct {
	Choices []skillChoice
	Given   bool
	Title   string
	Skills  []givenSkill
}

// givenSkill is one skill of the set an agent is given.
type givenSkill struct{ Name, Description string }

// roomLink is a room the page links to.
type roomLink struct{ Name, Where string }

// membersData is the members list and the dialogs that add to it, which every members page shows.
func (s *Server) membersData(r *http.Request) (map[string]any, error) {
	invite := s.inviteOffered(r)
	return map[string]any{
		"People":   s.workspacePeople(r),
		"Agents":   s.workspaceAgents(r, ""),
		"Keeps":    s.crew != nil,
		"NewAgent": s.newAgentOf(r),
		"Invite":   invite,
		"Sent":     invite != nil && invitationSent(r),
		"Refused":  s.refusal(r),
	}, nil
}

func (s *Server) newAgentOf(r *http.Request) newAgentView {
	offered := s.offeredTo(r)
	kinds := kindsFor(s.engines(), offered)
	shown := newAgentView{Kinds: make([]kindView, 0, len(kinds)), FirstReady: firstReady(s.picksFor(r)), Offered: s.modelOptionsOf(offered)}
	for _, each := range kinds {
		shown.Kinds = append(shown.Kinds, kindView{Name: each.Name, Title: each.Title, What: each.What, Why: each.Why,
			Mark: logo(each.Name), MarkDark: logoDark(each.Name), Measured: each.Measured, Ready: each.Ready, Failed: each.Failed,
			Installable: !each.Ready && s.installable(each.Name)})
	}
	return shown
}

// firstReady is the agent type a new agent is offered first: the first this person can run.
func firstReady(picks []pick) string {
	if len(picks) == 0 {
		return ""
	}
	return picks[0].Name
}

// hasAgent is whether the address names an agent the person asking may see.
func (s *Server) hasAgent(r *http.Request) bool {
	_, found := s.crewAgent(r)
	return found
}

// agentData is an agent's page: the agent, with its access, its skills, its apps where this server offers them, and the rooms
// of the person asking that it is in. Which tab is open is the page's to say.
func (s *Server) agentData(r *http.Request) (map[string]any, error) {
	member, found := s.crewAgent(r)
	if !found {
		return nil, failed{http.StatusNotFound, "there is no such agent"}
	}
	return map[string]any{
		"Agent":  s.agentPageOf(r, member),
		"Apps":   s.agentAppsFor(r, member.ID),
		"Skills": s.agentSkillsOf(member.Setup),
		"Rooms": s.roomsWith(r, func(held instance.Instance) bool {
			return slices.ContainsFunc(held.Agents, func(agent instance.Agent) bool { return agent.Member == member.ID })
		}),
		"Refused": s.refusal(r),
	}, nil
}

func (s *Server) agentPageOf(r *http.Request, member crew.Agent) agentPageView {
	who := s.askerName(r)
	shown := agentPageView{listedAgent: s.listedAgent(member), Brain: modelName(member.Model), Model: member.Model, Persona: member.Persona,
		Setup: member.Setup, Owner: member.Owner, OwnerName: s.personName(member.Owner), OwnerInitials: s.initials(member.Owner),
		Access: make([]accessView, 0, len(member.Access)), Created: member.Created,
		Mine: strings.EqualFold(member.Owner, who) || s.admin(r), Usable: member.MayUse(who)}
	for _, each := range member.Access {
		shown.Access = append(shown.Access, accessView{Who: each, Name: s.personName(each), Initials: s.initials(each)})
	}
	return shown
}

func (s *Server) agentSkillsOf(name string) agentSkillsView {
	held, given := s.skillsOf(name)
	shown := agentSkillsView{Choices: s.skillChoices(), Given: given, Title: s.skillTitle(name), Skills: make([]givenSkill, 0, len(held.Skills))}
	for _, each := range held.Skills {
		shown.Skills = append(shown.Skills, givenSkill{Name: each.Name, Description: each.Description})
	}
	return shown
}

// hasPerson is whether the address names somebody in the workspace of the person asking.
func (s *Server) hasPerson(r *http.Request) bool {
	_, found := s.personAt(s.workspacePeople(r), r)
	return found
}

// personAt is the person the address names, among the people given.
func (s *Server) personAt(people []listedPerson, r *http.Request) (listedPerson, bool) {
	who := strings.ToLower(r.PathValue("who"))
	at := slices.IndexFunc(people, func(each listedPerson) bool { return each.Who == who })
	if at < 0 {
		return listedPerson{}, false
	}
	return people[at], true
}

// personData is a person's page: who they are, their bio, the rooms the person asking shares with them, and the agents they made.
func (s *Server) personData(r *http.Request) (map[string]any, error) {
	people := s.workspacePeople(r)
	person, found := s.personAt(people, r)
	if !found {
		return nil, failed{http.StatusNotFound, "there is nobody of that address here"}
	}
	return map[string]any{
		"Person":  person,
		"Bio":     s.bioOf(r, person.Who),
		"Rooms":   s.roomsWith(r, func(held instance.Instance) bool { return held.RoleOf(person.Who) != "" }),
		"Made":    s.workspaceAgents(r, person.Who),
		"Refused": s.refusal(r),
	}, nil
}

// roomsWith is each room the person asking is in that keep says to show.
func (s *Server) roomsWith(r *http.Request, keep func(instance.Instance) bool) []roomLink {
	rooms := []roomLink{}
	for _, room := range s.roomsFor(r) {
		if held, found := s.instances.Get(room.ID); found && keep(held) {
			rooms = append(rooms, roomLink{Name: room.Name, Where: room.Where})
		}
	}
	return rooms
}
