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
	Name  string
	Runs  string
	Chats []railChat
}

// railChat is one conversation in the side list.
type railChat struct {
	Name  string
	Where string
}

// railList is the side list: the projects used lately, the older ones folded away, and where the page is, so its conversation shows as
// the one open.
type railList struct {
	Recent []railGroup
	Older  []railGroup
	Here   string
}

// railFor is every project the person has that holds a conversation, the one used last first, each with its conversations newest first.
// The chat page and every conversation show the same list, so moving between them does not change what is beside you.
func (s *Server) railFor(r *http.Request, here string) railList {
	list := railList{Here: here}
	used := s.lastUsed(r)
	names := projectNames(used)
	var groups []railGroup
	for _, each := range used {
		chats := s.railChats(each.ID)
		if len(chats) == 0 {
			continue
		}
		groups = append(groups, railGroup{Name: names[each.ID], Runs: each.Runs(), Chats: chats})
	}
	list.Recent = groups[:min(len(groups), recentProjects)]
	list.Older = groups[min(len(groups), recentProjects):]
	return list
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
		chats = append(chats, railChat{Name: threads[i].Name, Where: threadPath(threads[i].Thread)})
	}
	return chats
}

// sideChats is the side list on a page that does not carry it: the same groups, fetched once the page is there.
func (s *Server) sideChats(w http.ResponseWriter, r *http.Request) {
	s.render(w, "side-chats", s.railFor(r, r.URL.Query().Get("here")))
}
