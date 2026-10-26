package web

import (
	"fmt"
	"hash/fnv"
	"net/http"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
)

// A room is looked at often: its page asks every few seconds whether anything changed, and every page's side list counts what is
// unread. Both are answered from memory. A finished turn's reply never changes, so what the counts need of it is read from disk
// once and kept.

// replyNote is what the unread counts need of a finished turn's reply: whether there is one, and the text after each @ in it.
type replyNote struct {
	said     bool
	mentions string
}

// replyNotes is a finished turn's replyNote by turn id, up to mostReplyNotes of them.
type replyNotes struct {
	guard sync.Mutex
	held  map[string]replyNote
}

// mostReplyNotes bounds the kept notes; at the bound they are all dropped and read again as they are asked for.
const mostReplyNotes = 20000

// mentionReach is how much of what follows an @ is kept: the longest name an agent or a person can have, in the widest letters,
// and the character after it, which says whether the name ended there.
const mentionReach = 4 * 34

func (n *replyNotes) of(s *Server, record jobs.Record) replyNote {
	n.guard.Lock()
	note, found := n.held[record.ID]
	n.guard.Unlock()
	if found {
		return note
	}
	reply := said(record.Dir(s.index.Root()))
	note = replyNote{said: strings.TrimSpace(reply) != "", mentions: mentionsOf(reply)}
	n.guard.Lock()
	if n.held == nil || len(n.held) >= mostReplyNotes {
		n.held = map[string]replyNote{}
	}
	n.held[record.ID] = note
	n.guard.Unlock()
	return note
}

// mentionsOf is the text that begins at each @ that starts a mention, one to a line, so it can be read for mentions as the reply
// itself would be.
func mentionsOf(text string) string {
	var kept strings.Builder
	for at := strings.IndexByte(text, '@'); at >= 0; {
		if at == 0 || !wordy(lastRune(text[:at])) {
			end := min(len(text), at+mentionReach)
			for end < len(text) && !utf8.RuneStart(text[end]) {
				end--
			}
			kept.WriteString(text[at:end])
			kept.WriteByte('\n')
		}
		next := strings.IndexByte(text[at+1:], '@')
		if next < 0 {
			break
		}
		at += 1 + next
	}
	return kept.String()
}

// roomActivity is when the newest thing in a conversation happened, and a word that changes whenever anything in it does: a turn
// or a note added, or a turn moving on.
func (s *Server) roomActivity(thread instance.Thread) (int64, string) {
	var newest int64
	states := fnv.New64a()
	for _, id := range thread.Turns {
		record, found := s.index.Get(id)
		if !found {
			continue
		}
		newest = max(newest, record.Queued, record.Ended)
		states.Write([]byte(record.State))
		if wait, queued := s.index.WaitOf(id); queued {
			fmt.Fprintf(states, "/%d/%d/%d", wait.Reason, wait.Position, wait.Running)
		}
	}
	for _, note := range thread.Notes {
		newest = max(newest, note.At)
	}
	return newest, fmt.Sprintf("%d.%d.%d.%x", len(thread.Turns), len(thread.Notes), newest, states.Sum64())
}

// turnsVersion answers whether a conversation changed since the page last drew it, so a room's page fetches its turns only then.
func (s *Server) turnsVersion(w http.ResponseWriter, r *http.Request) {
	held, found := s.sharedOr404(w, r)
	if !found {
		return
	}
	thread, found := s.instances.Thread(held.ID, r.PathValue("thread"))
	if !found {
		http.NotFound(w, r)
		return
	}
	_, version := s.roomActivity(thread)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, version)
}
