package web

import (
	"github.com/getlatentic/gusto/internal/workspace"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/getlatentic/gusto/internal/crew"
	"github.com/getlatentic/gusto/internal/roster"
)

// Members is everybody in the workspace, people and agents, in a list beside whichever of them is open, as Kylon shows them. An
// agent is made here once, and from here it is added to rooms or talked to on its own.

// UseCrew is where the workspace's agents are kept.
func (s *Server) UseCrew(store *crew.Store) { s.crew = store }

func (s *Server) memberRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /workspace/{id}", s.guard(s.chooseWorkspace))
	mux.HandleFunc("POST /members/people/{who}/bio", s.guard(s.setPersonBio))
	mux.HandleFunc("POST /members/agents", s.guard(s.makeAgent))
	mux.HandleFunc("POST /members/agents/{id}", s.guard(s.changeAgent))
	mux.HandleFunc("POST /members/agents/{id}/access", s.guard(s.changeAgentAccess))
	mux.HandleFunc("POST /members/agents/{id}/remove", s.guard(s.removeAgent))
	mux.HandleFunc("POST /members/agents/{id}/message", s.guard(s.messageAgent))
	mux.HandleFunc("POST /agents/{name}/install", s.guard(s.installAgent))
}

// listedPerson is a person as the members list and their page show them. SharedSignIn is the server's one shared sign-in,
// which has no email address.
type listedPerson struct {
	Who, Name, Initials, Role, Where string
	SharedSignIn                     bool
}

// listedAgent is an agent as the members list shows it: Engine is its agent type, by name.
type listedAgent struct {
	ID, Name, Harness, Mark, MarkDark string
	Engine, Specialty, Where          string
}

func (s *Server) listedAgent(member crew.Agent) listedAgent {
	return listedAgent{ID: member.ID, Name: member.Name, Harness: member.Harness, Mark: logo(member.Harness), MarkDark: logoDark(member.Harness),
		Engine: s.engineName(member.Harness), Specialty: member.Specialty, Where: "/members/agents/" + member.ID}
}

// workspacePeople is everybody in the workspace: the organisation's people where it keeps a list of them, and otherwise the
// people the person asking shares a room with, and themselves.
func (s *Server) workspacePeople(r *http.Request) []listedPerson {
	seen := map[string]listedPerson{}
	add := func(who, role string) {
		who = strings.ToLower(strings.TrimSpace(who))
		if _, found := seen[who]; who != "" && !found {
			seen[who] = listedPerson{Who: who, Name: s.personName(who), Initials: s.initials(who), Role: role,
				Where: "/members/people/" + url.PathEscape(who), SharedSignIn: who == sharedSignIn}
		}
	}
	for _, member := range s.rosterHere(r) {
		add(member.Email, roleWord(member.Role))
	}
	me, here := s.askerName(r), s.workspaceOf(r)
	add(me, map[bool]string{true: "Owner", false: "Member"}[me == sharedSignIn])
	for _, held := range s.instances.RoomsOf(me) {
		if !workspace.Same(held.Workspace, here) {
			continue
		}
		for _, member := range held.Members {
			add(member.Who, "Member")
		}
	}
	people := make([]listedPerson, 0, len(seen))
	for _, person := range seen {
		people = append(people, person)
	}
	slices.SortFunc(people, func(a, b listedPerson) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return people
}

// rosterHere is the people of the organization's roster who are in the workspace the request is in, and nobody where it
// keeps no roster.
func (s *Server) rosterHere(r *http.Request) []roster.Member {
	if !s.hasRoster() {
		return nil
	}
	list, err := roster.Read(s.rosterPath)
	if err != nil {
		return nil
	}
	here, in := s.workspaceOf(r), map[string]bool{}
	for _, space := range list.Spaces() {
		for _, member := range space.Members {
			in[strings.ToLower(member)] = in[strings.ToLower(member)] || space.ID == here
		}
	}
	var kept []roster.Member
	for _, member := range list.Members {
		if in[strings.ToLower(member.Email)] {
			kept = append(kept, member)
		}
	}
	return kept
}

func roleWord(role string) string {
	if role == "" {
		return "Member"
	}
	return strings.ToUpper(role[:1]) + role[1:]
}

// workspaceAgents is each agent kept in the workspace the request is in, made by the person given, or by anybody when none is.
func (s *Server) workspaceAgents(r *http.Request, madeBy string) []listedAgent {
	agents := []listedAgent{}
	if s.crew == nil {
		return agents
	}
	here := s.workspaceOf(r)
	for _, member := range s.crew.List() {
		if workspace.Same(member.Workspace, here) && (madeBy == "" || member.Owner == madeBy) {
			agents = append(agents, s.listedAgent(member))
		}
	}
	return agents
}

// crewAgent is the agent the path names, when it is in a workspace the person asking is in.
func (s *Server) crewAgent(r *http.Request) (crew.Agent, bool) {
	if s.crew == nil {
		return crew.Agent{}, false
	}
	member, found := s.crew.Get(r.PathValue("id"))
	if !found || !s.inWorkspace(r, member.Workspace) {
		return crew.Agent{}, false
	}
	return member, true
}
