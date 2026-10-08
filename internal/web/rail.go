package web

import "net/http"

// chatsPerAgent is how many of an agent's conversations the side list shows.
const chatsPerAgent = 8

// railGroup is one agent in the side list with its latest conversations under it.
type railGroup struct {
	Name  string
	Runs  string
	Chats []railChat
}

// railChat is one conversation in the side list.
type railChat struct {
	Name  string
	Where string
}

// railFor is every agent the person has, the one used last first, each with its conversations newest first. The
// chat page and every conversation show the same list, so moving between them does not change what is beside you.
func (s *Server) railFor(r *http.Request) []railGroup {
	used := s.lastUsed(r)
	groups := make([]railGroup, 0, len(used))
	for _, each := range used {
		groups = append(groups, railGroup{Name: each.Name, Runs: each.Runs(), Chats: s.railChats(each.ID)})
	}
	return groups
}

func (s *Server) railChats(id string) []railChat {
	threads := s.namedThreads(id)
	chats := make([]railChat, 0, min(len(threads), chatsPerAgent))
	for i := len(threads) - 1; i >= 0 && len(chats) < chatsPerAgent; i-- {
		chats = append(chats, railChat{Name: threads[i].Name, Where: threadPath(threads[i].Thread)})
	}
	return chats
}
