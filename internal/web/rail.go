package web

import (
	"net/http"
	"time"
)

const (
	// chatsPerProject is how many of a project's conversations the side list shows.
	chatsPerProject = 8
	// recentProjects is how many projects are open in the side list; the rest are folded away under it.
	recentProjects = 6
)

// railGroup is one project in the side list: an agent's folder, with its latest conversations under it.
type railGroup struct {
	ID    string
	Name  string
	Runs  string
	Chats []railChat
	Open  bool
}

// railChat is one conversation in the side list.
type railChat struct {
	Name  string
	Where string
	When  int64
}

// railList is the side list: the projects used lately, the older ones folded away, and where the page is, so its conversation shows as
// the one open.
type railList struct {
	Rooms  []railChat
	Recent []railGroup
	Older  []railGroup
	Here   string
}

// railFor is every project the person has that holds a conversation, the one used last first, each with its conversations newest first.
// The chat page and every conversation show the same list, so moving between them does not change what is beside you.
func (s *Server) railFor(r *http.Request, here string) railList {
	list := railList{Here: here, Rooms: s.roomsFor(r)}
	used := s.lastUsed(r)
	names := projectNames(used)
	var groups []railGroup
	for _, each := range used {
		chats := s.railChats(each.ID)
		if len(chats) == 0 {
			continue
		}
		groups = append(groups, railGroup{ID: each.ID, Name: names[each.ID], Runs: each.Runs(), Chats: chats})
	}
	for i := range groups {
		groups[i].Open = i == 0 || holds(groups[i], here)
	}
	list.Recent = groups[:min(len(groups), recentProjects)]
	list.Older = groups[min(len(groups), recentProjects):]
	return list
}

// holds is whether the conversation at here is one of the group's.
func holds(group railGroup, here string) bool {
	for _, chat := range group.Chats {
		if chat.Where == here {
			return true
		}
	}
	return false
}

// projectNames is what each project is called where there are several: a name that two projects share is followed by the day each was
// started, so they can be told apart.
func projectNames(used []yours) map[string]string {
	count := map[string]int{}
	for _, each := range used {
		count[each.Name]++
	}
	names := make(map[string]string, len(used))
	for _, each := range used {
		names[each.ID] = each.Name
		if count[each.Name] > 1 {
			names[each.ID] += " · " + time.Unix(each.Started, 0).Format("2 Jan 15:04")
		}
	}
	return names
}

// railChats are a project's conversations that have something said in them, the newest first.
func (s *Server) railChats(id string) []railChat {
	threads := s.namedThreads(id)
	chats := make([]railChat, 0, min(len(threads), chatsPerProject))
	for i := len(threads) - 1; i >= 0 && len(chats) < chatsPerProject; i-- {
		if len(threads[i].Turns) == 0 {
			continue
		}
		chats = append(chats, railChat{Name: threads[i].Name, Where: threadPath(threads[i].Thread), When: s.lastAsked(threads[i].Turns)})
	}
	return chats
}

// lastAsked is when the newest of a conversation's turns was asked.
func (s *Server) lastAsked(turns []string) int64 {
	if len(turns) == 0 {
		return 0
	}
	if record, found := s.index.Get(turns[len(turns)-1]); found {
		return record.Queued
	}
	return 0
}
