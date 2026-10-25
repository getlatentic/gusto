package web

import (
	"cmp"
	"slices"
	"strings"

	"github.com/getlatentic/gusto/internal/instance"
)

// unread is what a person has not read in a room since they last looked: how many messages and replies, and how many of those
// mention them.
type unread struct {
	Count  int
	ForYou int
}

func (s *Server) unreadIn(thread instance.Thread, who string) unread {
	who = strings.ToLower(who)
	seen := thread.Seen[who]
	var waiting unread
	count := func(text string) {
		waiting.Count++
		if mentionsPerson(text, who) {
			waiting.ForYou++
		}
	}
	for _, id := range thread.Turns {
		record, found := s.index.Get(id)
		if !found {
			continue
		}
		if record.HandedBy == "" && record.Answers == "" && record.Queued > seen && !strings.EqualFold(record.Asked, who) {
			count(record.Brief)
		}
		if record.State.Done() && record.Ended > seen {
			if note := s.replies.of(s, record); note.said {
				count(note.mentions)
			}
		}
	}
	for _, note := range thread.Notes {
		if note.At > seen && !strings.EqualFold(note.Who, who) {
			count(note.Text)
		}
	}
	return waiting
}

// mentionsPerson says whether a message mentions a person by the name a room calls them.
func mentionsPerson(text, who string) bool {
	return len(mentionSpans(text, []string{personName(who)})) > 0
}

// latest is every conversation in the side list, rooms and projects together, the newest first, as its Recent tab lists them.
func latest(list railList) []railChat {
	all := slices.Concat(list.Direct, list.Rooms)
	for _, group := range slices.Concat(list.Recent, list.Older) {
		all = append(all, group.Chats...)
	}
	slices.SortStableFunc(all, func(a, b railChat) int { return cmp.Compare(b.When, a.When) })
	return all[:min(len(all), recentChats)]
}

// recentChats is how many conversations the Recent tab lists.
const recentChats = 30
