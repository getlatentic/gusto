package instance

import (
	"fmt"
	"github.com/getlatentic/gusto/internal/workspace"
	"strings"
)

// Arrival is an agent joining a room, and the person whose agent it is, who has to be in the room.
type Arrival struct {
	Agent Agent
	Owner string
}

// Join adds people to a room as members, and agents, as one change: all of them or none. A person already in the room is refused,
// since adding them would set their role.
func (s *Store) Join(id string, people []string, arrivals []Arrival) ([]Agent, error) {
	for i, who := range people {
		people[i] = strings.ToLower(strings.TrimSpace(who))
		if !strings.Contains(people[i], "@") {
			return nil, fmt.Errorf("a member is a sign-in address")
		}
	}
	for i := range arrivals {
		if err := checkAgent(&arrivals[i].Agent); err != nil {
			return nil, err
		}
	}
	var added []Agent
	err := s.changeRoom(id, func(held *Instance) error {
		for _, who := range people {
			if held.RoleOf(who) != "" {
				return AlreadyInError{Who: who}
			}
			held.Members = append(held.Members, Member{Who: who, Role: RoleMember})
		}
		for _, arrival := range arrivals {
			if arrival.Owner != "" && held.RoleOf(arrival.Owner) == "" {
				return fmt.Errorf("%s is not in this room, so their agent %s is not either", arrival.Owner, arrival.Agent.Name)
			}
			agent, err := s.placeAgent(held, arrival.Agent)
			if err != nil {
				return err
			}
			added = append(added, agent)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return added, nil
}

// AlreadyInError is a person added to a room they are in.
type AlreadyInError struct{ Who string }

func (e AlreadyInError) Error() string { return e.Who + " is in this room already" }

// RemoveMember takes a person out of a room, and with them the room's agents that theirs says are theirs, which it gives back.
// The last admin cannot be taken out.
func (s *Store) RemoveMember(id, who string, theirs func(Agent) bool) ([]Agent, error) {
	var gone []Agent
	err := s.changeRoom(id, func(held *Instance) error {
		kept := held.Members[:0:0]
		for _, member := range held.Members {
			if !strings.EqualFold(member.Who, who) {
				kept = append(kept, member)
			}
		}
		if len(kept) == len(held.Members) {
			return fmt.Errorf("%s is not in this room", who)
		}
		held.Members = kept
		staying := held.Agents[:0:0]
		for _, agent := range held.Agents {
			if theirs != nil && theirs(agent) {
				gone = append(gone, agent)
				continue
			}
			staying = append(staying, agent)
		}
		held.Agents = staying
		return keepsAdmin(*held)
	})
	if err != nil {
		return nil, err
	}
	return gone, nil
}

// DirectRoom is the room of two a person keeps with a workspace agent, when there is one.
func (s *Store) DirectRoom(who, member string) (Instance, bool) {
	for _, held := range s.RoomsOf(who) {
		if held.Direct && len(held.Members) == 1 && len(held.Agents) == 1 && held.Agents[0].Member == member {
			return held, true
		}
	}
	return Instance{}, false
}

// StartDirect is a person's own room with a workspace agent: the one they have, or a new one, made whole in one step and named
// for the agent.
func (s *Store) StartDirect(who, space string, agent Agent) (Instance, error) {
	if err := checkAgent(&agent); err != nil {
		return Instance{}, err
	}
	who = strings.ToLower(strings.TrimSpace(who))
	s.directs.Lock()
	defer s.directs.Unlock()
	if held, found := s.DirectRoom(who, agent.Member); found {
		return held, nil
	}
	agent.ID = "a1"
	held, err := s.Start(slug(agent.Name), Instance{
		Title: agent.Name, Who: who, Workspace: workspace.Stored(space), Room: true, Direct: true,
		Members: []Member{{Who: who, Role: RoleAdmin}}, Agents: []Agent{agent},
	})
	if err != nil {
		return Instance{}, err
	}
	// The room is found by what it holds, so a thread left unnamed by a failure here is named by what is first said in it.
	_ = s.Name(held.ID, s.Latest(held.ID).ID, agent.Name)
	return held, nil
}
