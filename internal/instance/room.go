package instance

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Roles a person holds in a room. An admin changes who and what is in it; a member talks in it.
const (
	RoleAdmin  = "admin"
	RoleMember = "member"
)

// Member is a person in a room, by the address they sign in with.
type Member struct {
	Who  string `json:"who"`
	Role string `json:"role"`
}

// Agent is an agent in a room: a harness and a model, known by a name people mention it by, and what it does there.
type Agent struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Harness string `json:"harness"`
	Model   string `json:"model,omitempty"`
	Job     string `json:"job,omitempty"`
	// Member is the workspace agent this is, whose name, agent type, model, skills and persona the room uses as they are now.
	Member string `json:"member,omitempty"`
	// Setup and Persona are the skills it is given and how it works, which come from the workspace agent.
	Setup   string `json:"-"`
	Persona string `json:"-"`
}

// Note is something a person said in a room that no agent was asked to answer, or something the room said of itself.
type Note struct {
	ID   string `json:"id"`
	Who  string `json:"who"`
	Text string `json:"text"`
	At   int64  `json:"at"`
	// Notice marks what the room said of itself: nobody in it said it.
	Notice bool `json:"notice,omitempty"`
	// After is how many turns the thread held when it was said, which places it among them.
	After int `json:"after"`
}

// agentName is what an agent may be called: a letter first, then letters, digits, spaces and a few marks, as people write names.
var agentName = regexp.MustCompile(`^[\p{L}][\p{L}\p{N} '’._-]{0,31}$`)

// StartRoom makes a room whose first member, its admin, is the person who made it.
func (s *Store) StartRoom(name, who string) (Instance, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Instance{}, fmt.Errorf("a room needs a name")
	}
	who = strings.ToLower(strings.TrimSpace(who))
	return s.Start(slug(name), Instance{Title: name, Who: who, Room: true, Members: []Member{{Who: who, Role: RoleAdmin}}})
}

// RoleOf is the role a person holds in a room, or nothing when they are not in it.
func (held Instance) RoleOf(who string) string {
	who = strings.ToLower(strings.TrimSpace(who))
	if who == "" {
		return ""
	}
	for _, member := range held.Members {
		if strings.EqualFold(member.Who, who) {
			return member.Role
		}
	}
	return ""
}

// AgentCalled is the room's agent of that name, whatever its case.
func (held Instance) AgentCalled(name string) (Agent, bool) {
	for _, agent := range held.Agents {
		if strings.EqualFold(agent.Name, name) {
			return agent, true
		}
	}
	return Agent{}, false
}

// AgentByID is the room's agent with that id.
func (held Instance) AgentByID(id string) (Agent, bool) {
	for _, agent := range held.Agents {
		if agent.ID == id {
			return agent, true
		}
	}
	return Agent{}, false
}

// SetMember puts a person in a room with a role, or changes the role they have. A room keeps at least one admin.
func (s *Store) SetMember(id, who, role string) error {
	who = strings.ToLower(strings.TrimSpace(who))
	if who == "" || !strings.Contains(who, "@") {
		return fmt.Errorf("a member is a sign-in address")
	}
	if role != RoleAdmin && role != RoleMember {
		return fmt.Errorf("a member is an admin or a member")
	}
	return s.changeRoom(id, func(held *Instance) error {
		for i, member := range held.Members {
			if strings.EqualFold(member.Who, who) {
				held.Members[i].Role = role
				return keepsAdmin(*held)
			}
		}
		held.Members = append(held.Members, Member{Who: who, Role: role})
		return nil
	})
}

// RemoveMember takes a person out of a room. The last admin cannot be taken out.
func (s *Store) RemoveMember(id, who string) error {
	return s.changeRoom(id, func(held *Instance) error {
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
		return keepsAdmin(*held)
	})
}

// AddAgent puts an agent in a room under a name nobody else there has.
func (s *Store) AddAgent(id string, agent Agent) (Agent, error) {
	agent.Name = strings.TrimSpace(agent.Name)
	if !agentName.MatchString(agent.Name) {
		return Agent{}, fmt.Errorf("%q cannot name an agent: start with a letter, and use up to 32 letters, digits and spaces", agent.Name)
	}
	if agent.Harness == "" {
		return Agent{}, fmt.Errorf("an agent needs a harness")
	}
	err := s.changeRoom(id, func(held *Instance) error {
		if _, taken := held.AgentCalled(agent.Name); taken {
			return fmt.Errorf("this room already has an agent called %s", agent.Name)
		}
		for _, other := range held.Agents {
			if agent.Member != "" && other.Member == agent.Member {
				return fmt.Errorf("%s is already in this room", agent.Name)
			}
		}
		agent.ID = fmt.Sprintf("a%d", len(held.Agents)+1)
		for {
			if _, used := held.AgentByID(agent.ID); !used {
				break
			}
			agent.ID += "x"
		}
		held.Agents = append(held.Agents, agent)
		return nil
	})
	return agent, err
}

// RemoveAgent takes an agent out of a room. What it did stays in the conversation.
func (s *Store) RemoveAgent(id, agent string) error {
	return s.changeRoom(id, func(held *Instance) error {
		kept := held.Agents[:0:0]
		for _, each := range held.Agents {
			if each.ID != agent {
				kept = append(kept, each)
			}
		}
		if len(kept) == len(held.Agents) {
			return fmt.Errorf("no agent %s in this room", agent)
		}
		held.Agents = kept
		return nil
	})
}

// Note keeps something a person said in a room that no agent answers.
func (s *Store) Note(id, thread, who, text string) (Note, error) {
	return s.addNote(id, thread, Note{Who: strings.ToLower(who), Text: text})
}

// Notice keeps something the room says to itself, such as that two agents changed the same file.
func (s *Store) Notice(id, thread, text string) (Note, error) {
	return s.addNote(id, thread, Note{Text: text, Notice: true})
}

func (s *Store) addNote(id, thread string, note Note) (Note, error) {
	s.guard.Lock()
	defer s.guard.Unlock()
	held, found := s.threads[id][thread]
	if !found {
		return Note{}, fmt.Errorf("no thread %s on %s", thread, id)
	}
	note.ID, note.At, note.After = fmt.Sprintf("n%d", len(held.Notes)+1), time.Now().Unix(), len(held.Turns)
	held.Notes = append(held.Notes, note)
	s.threads[id][held.ID] = held
	return note, s.writeThread(held)
}

// Learned keeps what an agent calls a conversation, so its next turn resumes it. A room's agents each keep their own.
func (s *Store) Learned(id, thread, agent, remembered string) error {
	if agent == "" {
		return s.Said(id, thread, "", remembered)
	}
	s.guard.Lock()
	defer s.guard.Unlock()
	held, found := s.threads[id][thread]
	if !found {
		return fmt.Errorf("no thread %s on %s", thread, id)
	}
	if remembered == "" {
		return nil
	}
	if held.Sessions == nil {
		held.Sessions = map[string]string{}
	}
	held.Sessions[agent] = remembered
	s.threads[id][held.ID] = held
	return s.writeThread(held)
}

// Saw keeps that a person looked at a conversation at a time. It writes only when that is later than they last looked.
func (s *Store) Saw(id, thread, who string, at int64) error {
	who = strings.ToLower(strings.TrimSpace(who))
	s.guard.Lock()
	defer s.guard.Unlock()
	held, found := s.threads[id][thread]
	if !found {
		return fmt.Errorf("no thread %s on %s", thread, id)
	}
	if who == "" || held.Seen[who] >= at {
		return nil
	}
	if held.Seen == nil {
		held.Seen = map[string]int64{}
	}
	held.Seen[who] = at
	s.threads[id][held.ID] = held
	return s.writeThread(held)
}

// Direct marks a room as one person's own with one agent, or as a room again.
func (s *Store) Direct(id string, direct bool) error {
	return s.changeRoom(id, func(held *Instance) error {
		held.Direct = direct
		return nil
	})
}

// changeRoom changes a room under the store's guard and writes it.
func (s *Store) changeRoom(id string, change func(*Instance) error) error {
	s.guard.Lock()
	defer s.guard.Unlock()
	held, found := s.held[id]
	if !found || !held.Room {
		return fmt.Errorf("no room called %s", id)
	}
	held.Members = append([]Member(nil), held.Members...)
	held.Agents = append([]Agent(nil), held.Agents...)
	if err := change(&held); err != nil {
		return err
	}
	if err := s.write(held); err != nil {
		return err
	}
	s.put(held)
	return nil
}

func keepsAdmin(held Instance) error {
	for _, member := range held.Members {
		if member.Role == RoleAdmin {
			return nil
		}
	}
	return fmt.Errorf("a room keeps at least one admin")
}

// slug is a room name as an id: lower case, its words joined by -.
func slug(name string) string {
	var out strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
			dash = false
		case !dash && out.Len() > 0:
			out.WriteByte('-')
			dash = true
		}
	}
	id := strings.Trim(out.String(), "-")
	if id == "" {
		id = "room"
	}
	return "room-" + id
}
