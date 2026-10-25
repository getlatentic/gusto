package web

import (
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

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
	mux.HandleFunc("POST /rooms/{id}/members/add", s.guard(s.addRoomMembers))
	mux.HandleFunc("POST /rooms/{id}/members/role", s.guard(s.changeRoomRole))
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
	http.Redirect(w, r, threadPath(thread)+"?add=1", http.StatusSeeOther)
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
