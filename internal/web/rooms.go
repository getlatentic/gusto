package web

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/pkg/job"
)

// A room is a conversation its people share. Each of its agents keeps a conversation of its own in it and answers when it is
// mentioned, or always when it is the room's only agent. An agent that mentions another in its reply hands the work on.

const (
	// handOffs is how many times a chain of agents may hand a request on before it stops and waits for a person.
	handOffs = 3
	// sinceLast is how many things said since an agent last spoke it is told of, newest kept.
	sinceLast = 30
	// heard is how much of each of those it is told, so one long reply does not crowd out the rest.
	heard = 1500
)

// mention is an @ and a name, which is how an agent is asked to act.
var mention = regexp.MustCompile(`(?:^|[^\w@])@([A-Za-z][A-Za-z0-9_-]{0,31})`)

func (s *Server) roomRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /rooms/new", s.guard(s.newRoomPage))
	mux.HandleFunc("POST /rooms", s.guard(s.makeRoom))
	mux.HandleFunc("GET /rooms/{id}/members", s.guard(s.roomMembersPage))
	mux.HandleFunc("POST /rooms/{id}/members", s.guard(s.setRoomMember))
	mux.HandleFunc("POST /rooms/{id}/members/remove", s.guard(s.removeRoomMember))
	mux.HandleFunc("POST /rooms/{id}/agents", s.guard(s.addRoomAgent))
	mux.HandleFunc("POST /rooms/{id}/agents/{agent}/remove", s.guard(s.removeRoomAgent))
}

func (s *Server) newRoomPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "room-new.html", map[string]any{"Title": productName + " — new room", "Nav": "chat"})
}

// makeRoom makes a room with the person asking as its admin, and opens its members, where its agents are added.
func (s *Server) makeRoom(w http.ResponseWriter, r *http.Request) {
	held, err := s.instances.StartRoom(r.FormValue("name"), s.askerName(r))
	if err != nil {
		s.turnAway(w, r, "/rooms/new", err.Error())
		return
	}
	thread := s.instances.Latest(held.ID)
	if err := s.instances.Name(held.ID, thread.ID, held.Title); err != nil {
		s.turnAway(w, r, "/rooms/new", err.Error())
		return
	}
	http.Redirect(w, r, "/rooms/"+held.ID+"/members", http.StatusSeeOther)
}

// roomOf is the room the path names when the person asking is in it, or is an admin of this server.
func (s *Server) roomOf(w http.ResponseWriter, r *http.Request) (instance.Instance, bool) {
	held, found := s.shared(r)
	if !found || !held.Room {
		http.NotFound(w, r)
		return instance.Instance{}, false
	}
	return held, true
}

// runsRoom says whether the person asking may change who and what is in a room.
func (s *Server) runsRoom(r *http.Request, held instance.Instance) bool {
	return s.admin(r) || held.RoleOf(s.askerName(r)) == instance.RoleAdmin
}

// roomPerson is a member as the members page lists them.
type roomPerson struct {
	instance.Member
	You bool
}

// roomAgent is an agent as the members page and a conversation show it.
type roomAgent struct {
	instance.Agent
	Engine string
	Brain  string
}

func (s *Server) roomAgents(held instance.Instance) []roomAgent {
	shown := make([]roomAgent, 0, len(held.Agents))
	for _, agent := range held.Agents {
		shown = append(shown, roomAgent{Agent: agent, Engine: s.engineName(agent.Harness), Brain: modelName(agent.Model)})
	}
	return shown
}

func (s *Server) roomMembersPage(w http.ResponseWriter, r *http.Request) {
	held, found := s.roomOf(w, r)
	if !found {
		return
	}
	you := s.askerName(r)
	people := make([]roomPerson, 0, len(held.Members))
	for _, member := range held.Members {
		people = append(people, roomPerson{Member: member, You: strings.EqualFold(member.Who, you)})
	}
	picks := s.picksFor(r)
	s.render(w, "room-members.html", map[string]any{
		"Title":   productName + " — " + held.Title + " members",
		"Nav":     "chat",
		"Room":    held,
		"Where":   threadPath(s.instances.Latest(held.ID)),
		"People":  people,
		"Agents":  s.roomAgents(held),
		"Runs":    s.runsRoom(r, held),
		"Picks":   picks,
		"Models":  s.modelOptionsOf(s.offeredTo(r)),
		"Refused": r.URL.Query().Get("refused"),
	})
}

func (s *Server) membersPath(held instance.Instance) string { return "/rooms/" + held.ID + "/members" }

func (s *Server) setRoomMember(w http.ResponseWriter, r *http.Request) {
	held, found := s.roomOf(w, r)
	if !found {
		return
	}
	if !s.runsRoom(r, held) {
		http.Error(w, "only a room's admin changes who is in it", http.StatusForbidden)
		return
	}
	role := r.FormValue("role")
	if role == "" {
		role = instance.RoleMember
	}
	if err := s.instances.SetMember(held.ID, r.FormValue("who"), role); err != nil {
		s.turnAway(w, r, s.membersPath(held), err.Error())
		return
	}
	http.Redirect(w, r, s.membersPath(held), http.StatusSeeOther)
}

func (s *Server) removeRoomMember(w http.ResponseWriter, r *http.Request) {
	held, found := s.roomOf(w, r)
	if !found {
		return
	}
	who := r.FormValue("who")
	// Anybody may leave; only an admin takes somebody else out.
	if !s.runsRoom(r, held) && !strings.EqualFold(who, s.askerName(r)) {
		http.Error(w, "only a room's admin takes somebody out of it", http.StatusForbidden)
		return
	}
	if err := s.instances.RemoveMember(held.ID, who); err != nil {
		s.turnAway(w, r, s.membersPath(held), err.Error())
		return
	}
	if strings.EqualFold(who, s.askerName(r)) && !s.admin(r) {
		http.Redirect(w, r, "/chat", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, s.membersPath(held), http.StatusSeeOther)
}

func (s *Server) addRoomAgent(w http.ResponseWriter, r *http.Request) {
	held, found := s.roomOf(w, r)
	if !found {
		return
	}
	if !s.runsRoom(r, held) {
		http.Error(w, "only a room's admin adds agents to it", http.StatusForbidden)
		return
	}
	harness := r.FormValue("harness")
	if !slices.ContainsFunc(s.picksFor(r), func(each pick) bool { return each.Name == harness }) {
		s.turnAway(w, r, s.membersPath(held), "that agent cannot run on this server")
		return
	}
	model := r.FormValue("model")
	if model != "" && !s.offeredTo(r).Offers(model) {
		s.turnAway(w, r, s.membersPath(held), "that model is not one you may use")
		return
	}
	agent := instance.Agent{Name: r.FormValue("name"), Harness: harness, Model: model, Job: strings.TrimSpace(r.FormValue("job"))}
	if _, err := s.instances.AddAgent(held.ID, agent); err != nil {
		s.turnAway(w, r, s.membersPath(held), err.Error())
		return
	}
	http.Redirect(w, r, s.membersPath(held), http.StatusSeeOther)
}

func (s *Server) removeRoomAgent(w http.ResponseWriter, r *http.Request) {
	held, found := s.roomOf(w, r)
	if !found {
		return
	}
	if !s.runsRoom(r, held) {
		http.Error(w, "only a room's admin takes agents out of it", http.StatusForbidden)
		return
	}
	if err := s.instances.RemoveAgent(held.ID, r.PathValue("agent")); err != nil {
		s.turnAway(w, r, s.membersPath(held), err.Error())
		return
	}
	http.Redirect(w, r, s.membersPath(held), http.StatusSeeOther)
}

// mentioned is each of a room's agents that the text mentions, in the order they are first mentioned.
func mentioned(held instance.Instance, text string) []instance.Agent {
	var named []instance.Agent
	for _, found := range mention.FindAllStringSubmatch(text, -1) {
		agent, in := held.AgentCalled(found[1])
		if in && !slices.ContainsFunc(named, func(each instance.Agent) bool { return each.ID == agent.ID }) {
			named = append(named, agent)
		}
	}
	return named
}

// roomSay is somebody saying something in a room: a turn for each agent it mentions, for the only agent when there is one and
// none is mentioned, and otherwise something the room reads and no agent answers.
func (s *Server) roomSay(w http.ResponseWriter, r *http.Request, held instance.Instance, thread instance.Thread, said string) {
	agents := mentioned(held, said)
	if len(agents) == 0 && len(held.Agents) == 1 {
		agents = held.Agents
	}
	asked := s.keeper.who(r)
	if len(agents) == 0 {
		note, err := s.instances.Note(held.ID, thread.ID, asked, said)
		if err != nil {
			s.turnAway(w, r, threadPath(thread), err.Error())
			return
		}
		http.Redirect(w, r, threadPath(thread)+"#"+note.ID, http.StatusSeeOther)
		return
	}
	first := ""
	for _, agent := range agents {
		record, err := s.startRoomTurn(asked, held, thread, said, agent, roomAsk{answers: first})
		if err != nil {
			s.turnAway(w, r, threadPath(thread), err.Error())
			return
		}
		first = firstOf(first, record.ID)
	}
	http.Redirect(w, r, threadPath(thread)+"#"+first, http.StatusSeeOther)
}

// roomAsk is how a room's turn came to be asked for: the agent that handed it on and how many had before it, and the first turn
// of the same message when one message asked several agents.
type roomAsk struct {
	from    string
	hops    int
	answers string
}

// startRoomTurn queues a turn for one of a room's agents. The record keeps what was said as the room shows it; the agent is
// given that with who is in the room and what was said since it last spoke, since it keeps a conversation of its own.
func (s *Server) startRoomTurn(asked string, held instance.Instance, thread instance.Thread, said string, agent instance.Agent, how roomAsk) (jobs.Record, error) {
	if fresh, found := s.instances.Thread(held.ID, thread.ID); found {
		thread = fresh
	}
	brief := s.roomBrief(held, thread, agent, asked, said, how.from)
	spec := job.Spec{
		Brief:       brief,
		Harness:     agent.Harness,
		Model:       agent.Model,
		Session:     held.ID,
		Thread:      thread.ID,
		Dir:         thread.Dir,
		Remembered:  thread.Sessions[agent.ID],
		Agent:       agent.ID,
		Repo:        held.Repo,
		Setup:       setupOf(held),
		Sensitivity: sensitivityOf(held),
	}
	id, err := s.index.Reserve(said, time.Now())
	if err != nil {
		return jobs.Record{}, err
	}
	record := jobs.Record{
		ID: id, State: jobs.Queued,
		Brief: said, Repo: held.Repo, Harness: agent.Harness, Model: agent.Model,
		Instance: held.ID, Thread: thread.ID, Agent: agent.ID, HandedBy: how.from, Answers: how.answers, Hops: how.hops,
		Asked: asked, Queued: jobs.Now(),
	}
	if err := s.place(spec, record); err != nil {
		return jobs.Record{}, err
	}
	if err := s.instances.Said(held.ID, thread.ID, record.ID, ""); err != nil {
		return jobs.Record{}, err
	}
	s.queue.Wake()
	return record, nil
}

// roomBrief is what one of a room's agents is given for a turn: who it is and who else is in the room, what was said since it
// last spoke, and what it is asked now.
func (s *Server) roomBrief(held instance.Instance, thread instance.Thread, agent instance.Agent, asked, said, from string) string {
	var brief strings.Builder
	role := ""
	if agent.Job != "" {
		role = ", for " + agent.Job
	}
	fmt.Fprintf(&brief, "You are %s%s, an agent in the room %q. People and agents share this room and its folder.\n", agent.Name, role, held.Title)
	brief.WriteString("People here:")
	for _, member := range held.Members {
		fmt.Fprintf(&brief, " %s (%s);", member.Who, member.Role)
	}
	brief.WriteString("\nAgents here:")
	for _, each := range held.Agents {
		if each.ID == agent.ID {
			continue
		}
		fmt.Fprintf(&brief, " @%s (%s, %s);", each.Name, firstOf(each.Job, "no job named"), s.engineName(each.Harness))
	}
	brief.WriteString("\nTo hand work to another agent, mention it as @Name in your reply, only when it should act now. What you reply is posted to the room.\n")
	if earlier := s.sinceLastSpoke(held, thread, agent); len(earlier) > 0 {
		brief.WriteString("\nSaid in the room since you last spoke:\n")
		for _, line := range earlier {
			brief.WriteString(line + "\n")
		}
	}
	speaker := asked
	if from != "" {
		if handing, found := held.AgentByID(from); found {
			speaker = "@" + handing.Name
		}
	}
	fmt.Fprintf(&brief, "\n%s says to you:\n%s\n", speaker, said)
	return brief.String()
}

// sinceLastSpoke is what was said in a room after an agent's last turn there, by people and the other agents, oldest first.
func (s *Server) sinceLastSpoke(held instance.Instance, thread instance.Thread, agent instance.Agent) []string {
	type line struct {
		at   int64
		text string
	}
	var last int64
	var lines []line
	for _, id := range thread.Turns {
		record, found := s.index.Get(id)
		if !found {
			continue
		}
		if record.Agent == agent.ID {
			last = max(last, record.Queued)
			continue
		}
		if record.Hops == 0 {
			lines = append(lines, line{record.Queued, record.Asked + ": " + clip(record.Brief)})
		}
		if reply := strings.TrimSpace(said(record.Dir(s.index.Root()))); reply != "" {
			name := record.Agent
			if other, found := held.AgentByID(record.Agent); found {
				name = other.Name
			}
			lines = append(lines, line{max(record.Ended, record.Queued), "@" + name + ": " + clip(reply)})
		}
	}
	for _, note := range thread.Notes {
		lines = append(lines, line{note.At, note.Who + ": " + clip(note.Text)})
	}
	lines = slices.DeleteFunc(lines, func(each line) bool { return each.at < last })
	slices.SortStableFunc(lines, func(a, b line) int { return int(a.at - b.at) })
	if len(lines) > sinceLast {
		lines = lines[len(lines)-sinceLast:]
	}
	out := make([]string, 0, len(lines))
	for _, each := range lines {
		out = append(out, each.text)
	}
	return out
}

// clip is a message cut to what an agent is told of it.
func clip(text string) string {
	text = strings.TrimSpace(text)
	if len(text) <= heard {
		return text
	}
	cut := heard
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "…"
}

func firstOf(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// handOff starts a turn for each agent a finished room turn's reply mentions, so agents can pass work along. A chain stops after
// handOffs turns, and a turn that did not finish hands nothing on.
func (s *Server) handOff(record jobs.Record) {
	if record.Agent == "" || record.State != jobs.Passed || record.Hops >= handOffs {
		return
	}
	held, found := s.instances.Get(record.Instance)
	if !found || !held.Room {
		return
	}
	thread, found := s.instances.Thread(held.ID, record.Thread)
	if !found {
		return
	}
	reply := strings.TrimSpace(said(record.Dir(s.index.Root())))
	first := ""
	for _, next := range mentioned(held, reply) {
		if next.ID == record.Agent {
			continue
		}
		handed, err := s.startRoomTurn(record.Asked, held, thread, reply, next, roomAsk{from: record.Agent, hops: record.Hops + 1, answers: first})
		if err == nil {
			first = firstOf(first, handed.ID)
		}
	}
}
