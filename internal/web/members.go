package web

import (
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/getlatentic/gusto/internal/crew"
	"github.com/getlatentic/gusto/internal/industry"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/roster"
	"github.com/getlatentic/gusto/internal/setup"
)

// Members is everybody in the workspace, people and agents, in a list beside whichever of them is open, as Kylon shows them. An
// agent is made here once, and from here it is added to rooms or talked to on its own.

// UseCrew is where the workspace's agents are kept.
func (s *Server) UseCrew(store *crew.Store) { s.crew = store }

func (s *Server) memberRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /members", s.guard(s.membersPage))
	mux.HandleFunc("GET /members/agents/{id}", s.guard(s.agentPage))
	mux.HandleFunc("GET /members/people/{who}", s.guard(s.personPage))
	mux.HandleFunc("POST /members/agents", s.guard(s.makeAgent))
	mux.HandleFunc("POST /members/agents/{id}", s.guard(s.changeAgent))
	mux.HandleFunc("POST /members/agents/{id}/access", s.guard(s.changeAgentAccess))
	mux.HandleFunc("POST /members/agents/{id}/remove", s.guard(s.removeAgent))
	mux.HandleFunc("POST /members/agents/{id}/message", s.guard(s.messageAgent))
}

// listedPerson is a person as the members list shows them.
type listedPerson struct {
	Who   string
	Name  string
	Role  string
	Where string
}

// listedAgent is an agent as the members list and its page show it.
type listedAgent struct {
	crew.Agent
	Engine string
	Brain  string
	Where  string
	Mine   bool
	Usable bool
}

func (s *Server) listedAgent(r *http.Request, member crew.Agent) listedAgent {
	who := s.askerName(r)
	return listedAgent{
		Agent:  member,
		Engine: s.engineName(member.Harness),
		Brain:  modelName(member.Model),
		Where:  "/members/agents/" + member.ID,
		Mine:   strings.EqualFold(member.Owner, who) || s.admin(r),
		Usable: member.MayUse(who),
	}
}

// workspacePeople is everybody in the workspace: the organisation's people where it keeps a list of them, and otherwise the
// people the person asking shares a room with, and themselves.
func (s *Server) workspacePeople(r *http.Request) []listedPerson {
	seen := map[string]listedPerson{}
	add := func(who, role string) {
		who = strings.ToLower(strings.TrimSpace(who))
		if _, found := seen[who]; who != "" && !found {
			seen[who] = listedPerson{Who: who, Name: personName(who), Role: role, Where: "/members/people/" + url.PathEscape(who)}
		}
	}
	if s.hasRoster() {
		if list, err := roster.Read(s.rosterPath); err == nil {
			for _, member := range list.Members {
				add(member.Email, roleWord(member.Role))
			}
		}
	}
	me := s.askerName(r)
	add(me, map[bool]string{true: "Owner", false: "Member"}[me == sharedSignIn])
	for _, held := range s.instances.List() {
		if held.Room && held.RoleOf(me) != "" {
			for _, member := range held.Members {
				add(member.Who, "Member")
			}
		}
	}
	people := make([]listedPerson, 0, len(seen))
	for _, person := range seen {
		people = append(people, person)
	}
	slices.SortFunc(people, func(a, b listedPerson) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return people
}

func roleWord(role string) string {
	if role == "" {
		return "Member"
	}
	return strings.ToUpper(role[:1]) + role[1:]
}

// membersData is what every members page shows: the list, and what the new agent dialog offers.
func (s *Server) membersData(r *http.Request, here string) map[string]any {
	var agents []listedAgent
	if s.crew != nil {
		for _, member := range s.crew.List() {
			agents = append(agents, s.listedAgent(r, member))
		}
	}
	return map[string]any{
		"Title": productName + " — members",
		"Nav":   "members",
		// The list and whoever is open fill the window, as a chat does.
		"Chat":       true,
		"Here":       here,
		"People":     s.workspacePeople(r),
		"Agents":     agents,
		"Kinds":      s.engines(),
		"FirstReady": firstReady(s.picksFor(r)),
		"Offered":    s.modelOptionsOf(s.offeredTo(r)),
		"Skills":     s.skillChoices(),
		"Keeps":      s.crew != nil,
		"Refused":    r.URL.Query().Get("refused"),
		"MakeOpen":   r.URL.Query().Has("new"),
	}
}

func (s *Server) membersPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "members.html", s.membersData(r, "/members"))
}

// firstReady is the agent type a new agent is offered first: the first this person can run.
func firstReady(picks []pick) string {
	if len(picks) == 0 {
		return ""
	}
	return picks[0].Name
}

// skillChoice is a set of skills an agent can be given: one of the ready-made roles, or a skill set somebody made.
type skillChoice struct {
	Name  string
	Title string
	Role  bool
}

func (s *Server) skillChoices() []skillChoice {
	var choices []skillChoice
	roles := map[string]bool{}
	for _, role := range industry.All() {
		roles[role.Name] = true
		choices = append(choices, skillChoice{Name: role.Name, Title: role.Title, Role: true})
	}
	for _, held := range s.setupsList() {
		if !roles[held.Name] {
			choices = append(choices, skillChoice{Name: held.Name, Title: held.Name})
		}
	}
	return choices
}

// skillsOf is the skill set an agent is given, with its skills, or nothing when it has none.
func (s *Server) skillsOf(name string) (setup.Setup, bool) {
	if name == "" || s.setups == nil {
		return setup.Setup{}, false
	}
	return s.setups.Get(name)
}

func (s *Server) agentPage(w http.ResponseWriter, r *http.Request) {
	member, found := s.crewAgent(r)
	if !found {
		http.NotFound(w, r)
		return
	}
	shown := s.listedAgent(r, member)
	page := s.membersData(r, shown.Where)
	held, has := s.skillsOf(member.Setup)
	tab := r.URL.Query().Get("tab")
	if !slices.Contains([]string{"access", "skills"}, tab) {
		tab = "profile"
	}
	page["Title"] = productName + " — " + member.Name
	page["Agent"] = shown
	page["Tab"] = tab
	page["SkillSet"], page["HasSkills"] = held, has
	page["SkillTitle"] = s.skillTitle(member.Setup)
	page["Rooms"] = s.roomsWith(r, member.ID)
	s.render(w, "members.html", page)
}

func (s *Server) skillTitle(name string) string {
	for _, choice := range s.skillChoices() {
		if choice.Name == name {
			return choice.Title
		}
	}
	return name
}

// roomsWith is each room the person asking is in that has the agent in it.
func (s *Server) roomsWith(r *http.Request, member string) []railChat {
	var rooms []railChat
	for _, room := range s.roomsFor(r) {
		held, found := s.instances.Get(room.ID)
		if found && slices.ContainsFunc(held.Agents, func(agent instance.Agent) bool { return agent.Member == member }) {
			rooms = append(rooms, room)
		}
	}
	return rooms
}

func (s *Server) personPage(w http.ResponseWriter, r *http.Request) {
	who := strings.ToLower(r.PathValue("who"))
	page := s.membersData(r, "/members/people/"+url.PathEscape(who))
	people := page["People"].([]listedPerson)
	at := slices.IndexFunc(people, func(each listedPerson) bool { return each.Who == who })
	if at < 0 {
		http.NotFound(w, r)
		return
	}
	var shared []railChat
	for _, room := range s.roomsFor(r) {
		held, found := s.instances.Get(room.ID)
		if found && held.RoleOf(who) != "" {
			shared = append(shared, room)
		}
	}
	page["Title"] = productName + " — " + people[at].Name
	page["Person"] = people[at]
	page["Rooms"] = shared
	page["Made"] = s.madeBy(who)
	s.render(w, "members.html", page)
}

// madeBy is each agent a person made.
func (s *Server) madeBy(who string) []crew.Agent {
	if s.crew == nil {
		return nil
	}
	var made []crew.Agent
	for _, member := range s.crew.List() {
		if member.Owner == who {
			made = append(made, member)
		}
	}
	return made
}

func (s *Server) crewAgent(r *http.Request) (crew.Agent, bool) {
	if s.crew == nil {
		return crew.Agent{}, false
	}
	return s.crew.Get(r.PathValue("id"))
}
