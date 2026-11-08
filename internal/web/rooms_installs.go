package web

import (
	"fmt"
	"github.com/getlatentic/gusto/internal/workspace"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/runner"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/spend"
)

// An agent in a room is the room's own record of it: who put it there, whether an admin of the room accepted it, and what it may
// spend there in a month. An agent belongs to the person who made it, and is only in rooms that person is in. A room's admin adds
// an agent at once; any other member proposes one of their own, and it waits for an admin.

// mostAllowance is the most a month's allowance may be, so a slip of the hand is caught.
const mostAllowance = 100000.0

// crewArrival is a workspace agent joining a room: at once when the person asking runs the room, and waiting for an admin when a
// member proposes it. A member proposes only an agent of their own, and an agent's owner has to be in the room.
func (s *Server) crewArrival(r *http.Request, held instance.Instance, memberID string) (instance.Arrival, error) {
	if s.crew == nil {
		return instance.Arrival{}, fmt.Errorf("this server keeps no agents")
	}
	asker := s.askerName(r)
	member, found := s.crew.Get(memberID)
	if !found || !member.MayUse(asker) || !workspace.Same(member.Workspace, held.Workspace) {
		return instance.Arrival{}, fmt.Errorf("that agent is not one you may work with")
	}
	runs := s.runsRoom(r, held)
	if !runs && !strings.EqualFold(member.Owner, asker) {
		return instance.Arrival{}, fmt.Errorf("you may propose only an agent of your own; %s is %s", member.Name, s.ownerLabel(member.Owner, asker))
	}
	if member.Owner != "" && held.RoleOf(member.Owner) == "" {
		return instance.Arrival{}, fmt.Errorf("%s is %s, and %s is not in this room", member.Name, s.ownerLabel(member.Owner, asker), s.personName(member.Owner))
	}
	allowance, err := allowanceOf(r.FormValue("allowance"))
	if err != nil {
		return instance.Arrival{}, err
	}
	return instance.Arrival{Owner: member.Owner, Agent: instance.Agent{
		Name: member.Name, Harness: member.Harness, Model: member.Model, Member: member.ID,
		Pending: !runs, ProposedBy: asker, AllowanceUSD: allowance,
	}}, nil
}

// allowanceOf reads a month's allowance in dollars, where nothing is no limit.
func allowanceOf(text string) (*float64, error) {
	text = strings.TrimPrefix(strings.TrimSpace(text), "$")
	if text == "" {
		return nil, nil
	}
	dollars, err := strconv.ParseFloat(text, 64)
	if err != nil || dollars < 0 || dollars > mostAllowance {
		return nil, fmt.Errorf("an allowance is dollars a month, from 0 to %.0f", mostAllowance)
	}
	return &dollars, nil
}

// acceptRoomAgent is a room's admin letting an agent a member proposed work in the room.
func (s *Server) acceptRoomAgent(w http.ResponseWriter, r *http.Request) {
	held, found := s.roomOf(w, r)
	if !found {
		return
	}
	if !s.runsRoom(r, held) {
		http.Error(w, "only a room's admin accepts an agent", http.StatusForbidden)
		return
	}
	if err := s.instances.AcceptAgent(held.ID, r.PathValue("agent")); err != nil {
		s.turnAway(w, r, s.membersPath(held), err.Error())
		return
	}
	http.Redirect(w, r, s.membersPath(held), http.StatusSeeOther)
}

// removeRoomAgent takes an agent out of a room, which its owner may do as well as the room's admin. Its turns there stop, so
// nothing it was doing is merged into the room's folder afterwards.
func (s *Server) removeRoomAgent(w http.ResponseWriter, r *http.Request) {
	held, found := s.roomOf(w, r)
	if !found {
		return
	}
	agent, in := held.AgentByID(r.PathValue("agent"))
	asker := s.askerName(r)
	if !in || (!s.runsRoom(r, held) && !strings.EqualFold(s.ownerOf(agent), asker)) {
		http.Error(w, "only a room's admin or the agent's owner takes it out of the room", http.StatusForbidden)
		return
	}
	if err := s.takeOutAgent(held.ID, agent.ID, asker); err != nil {
		s.turnAway(w, r, s.membersPath(held), err.Error())
		return
	}
	http.Redirect(w, r, s.membersPath(held), http.StatusSeeOther)
}

// setRoomAgentAllowance changes what an agent may spend in a room a month, which its owner may do as well as the room's admin,
// as either may when it is added. It holds from the agent's next turn.
func (s *Server) setRoomAgentAllowance(w http.ResponseWriter, r *http.Request) {
	held, found := s.roomOf(w, r)
	if !found {
		return
	}
	agent, in := held.AgentByID(r.PathValue("agent"))
	if !in || (!s.runsRoom(r, held) && !strings.EqualFold(s.ownerOf(agent), s.askerName(r))) {
		http.Error(w, "only a room's admin or the agent's owner sets its allowance", http.StatusForbidden)
		return
	}
	allowance, err := allowanceOf(r.FormValue("allowance"))
	if err == nil {
		err = s.instances.SetAllowance(held.ID, agent.ID, allowance)
	}
	if err != nil {
		s.turnAway(w, r, s.membersPath(held), err.Error())
		return
	}
	http.Redirect(w, r, s.membersPath(held), http.StatusSeeOther)
}

// takeOutAgent removes an agent from a room and stops what it was doing there.
func (s *Server) takeOutAgent(room, agent, by string) error {
	if err := s.instances.RemoveAgent(room, agent); err != nil {
		return err
	}
	s.stopTurns(room, agent, by)
	return nil
}

// takeOutMember takes a person out of a room, and every agent of theirs with them in the same change, then stops those agents'
// turns there.
func (s *Server) takeOutMember(room, who, by string) error {
	gone, err := s.instances.RemoveMember(room, who, func(agent instance.Agent) bool { return strings.EqualFold(s.ownerOf(agent), who) })
	if err != nil {
		return err
	}
	for _, agent := range gone {
		s.stopTurns(room, agent.ID, by)
	}
	return nil
}

// stopTurns stops every turn of an agent's in a room that has not finished.
func (s *Server) stopTurns(room, agent, by string) {
	for _, turn := range s.index.Unfinished(tasks.Record{Instance: room, Agent: agent}.Lane()) {
		_ = runner.NoteStop(turn.Dir(s.index.Root()), by, time.Now().Unix())
		_, _, _ = s.queue.Cancel(turn.ID, by)
	}
}

// overAllowance is why an agent may not take another turn in its room this month, or nothing when it may.
func (s *Server) overAllowance(held instance.Instance, agent instance.Agent) string {
	if agent.AllowanceUSD == nil || s.ledger == nil {
		return ""
	}
	spent := s.ledger.SpentThisMonth(spend.ByAllowance, spend.AllowanceOf(held.ID, agent.ID))
	if !spend.Reached(spent, *agent.AllowanceUSD) {
		return ""
	}
	return fmt.Sprintf("%s has spent $%.2f of its $%.2f allowance in this room this month, so it was not asked. Its owner or the room's admin can raise it from the room's members.",
		agent.Name, spent, *agent.AllowanceUSD)
}
