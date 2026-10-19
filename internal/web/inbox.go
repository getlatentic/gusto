package web

import (
	"context"
	"log/slog"
	"time"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/inbox"
)

// UseInbox says where the front door leaves what its admins did to the
// organisation's people. This server is the one writer of the change record, so it
// takes those events into it. Without a change record they wait where they are.
func (s *Server) UseInbox(dir string) { s.inbox = dir }

// RunInbox takes what is in the inbox into the change record every interval until
// ctx ends.
func (s *Server) RunInbox(ctx context.Context, every time.Duration) {
	if s.inbox == "" {
		return
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		s.TakeInbox()
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// TakeInbox writes each event that is waiting to the change record, oldest first, and
// removes its file, and says how many it wrote. An event that could not be written
// stays for the next pass, and the pass stops there, so the record keeps their order. A
// file that is not an event the front door reports is set aside, where it can be read,
// and is not taken.
func (s *Server) TakeInbox() int {
	if s.inbox == "" || s.changes == nil {
		return 0
	}
	s.taking.Lock()
	defer s.taking.Unlock()
	waiting, err := inbox.Pending(s.inbox)
	if err != nil {
		slog.Error("the inbox could not be read", "dir", s.inbox, "error", err)
		return 0
	}
	taken := 0
	for _, each := range waiting {
		if each.Err != nil || !changelog.FromFrontDoor(each.Item.Kind) {
			s.setAside(each)
			continue
		}
		// A server that stopped between writing an event and removing its file finds
		// the event in its record.
		if !s.changes.Taken(each.ID) {
			if _, err := s.changes.Append(eventOf(each)); err != nil {
				slog.Error("a change gusto cloud reported could not be recorded", "kind", each.Item.Kind, "error", err)
				return taken
			}
			taken++
		}
		if err := inbox.Done(s.inbox, each.ID); err != nil {
			slog.Error("a change that was recorded could not be taken out of the inbox", "file", each.ID, "error", err)
		}
	}
	return taken
}

func eventOf(each inbox.Waiting) changelog.Event {
	what := map[string]string{changelog.InboxKey: each.ID}
	for key, value := range each.Item.What {
		if key != changelog.InboxKey {
			what[key] = value
		}
	}
	at := ""
	if !each.Item.At.IsZero() {
		at = each.Item.At.UTC().Format(time.RFC3339Nano)
	}
	return changelog.Event{At: at, Who: each.Item.Who, Role: each.Item.Role, From: each.Item.From, Kind: each.Item.Kind, What: what}
}

func (s *Server) setAside(each inbox.Waiting) {
	slog.Error("a file in the inbox is not a change gusto cloud reports, and is set aside", "file", each.ID, "kind", each.Item.Kind, "error", each.Err)
	if err := inbox.SetAside(s.inbox, each.ID); err != nil {
		slog.Error("a file in the inbox could not be set aside", "file", each.ID, "error", err)
	}
}
