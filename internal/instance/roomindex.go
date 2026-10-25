package instance

import "strings"

// The store keeps which rooms each person is in, so the rooms of one person are found without walking every instance.

// put keeps an instance, and the rooms index with it. The guard is held.
func (s *Store) put(held Instance) {
	if old, found := s.held[held.ID]; found {
		s.unindex(old)
	}
	s.held[held.ID] = held
	if !held.Room {
		return
	}
	for _, member := range held.Members {
		who := strings.ToLower(member.Who)
		if s.rooms[who] == nil {
			s.rooms[who] = map[string]struct{}{}
		}
		s.rooms[who][held.ID] = struct{}{}
	}
}

// drop forgets an instance, and takes it out of the rooms index. The guard is held.
func (s *Store) drop(id string) {
	if old, found := s.held[id]; found {
		s.unindex(old)
	}
	delete(s.held, id)
	delete(s.threads, id)
}

func (s *Store) unindex(held Instance) {
	for _, member := range held.Members {
		who := strings.ToLower(member.Who)
		delete(s.rooms[who], held.ID)
		if len(s.rooms[who]) == 0 {
			delete(s.rooms, who)
		}
	}
}

// RoomsOf is every room a person is a member of, in no particular order.
func (s *Store) RoomsOf(who string) []Instance {
	s.guard.Lock()
	defer s.guard.Unlock()
	ids := s.rooms[strings.ToLower(strings.TrimSpace(who))]
	rooms := make([]Instance, 0, len(ids))
	for id := range ids {
		rooms = append(rooms, s.held[id])
	}
	return rooms
}
