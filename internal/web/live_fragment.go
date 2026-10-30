package web

import (
	"bytes"
	"context"
	"fmt"
	"slices"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/roomlive"
	"github.com/getlatentic/gusto/internal/roomlog"
)

// A live update is the HTML of one turn or note, drawn from the same templates as the page and sent to everybody in the room.
// What is a viewer's own is not drawn into it: the turn is drawn for nobody in particular, with the elements that depend on who
// is looking marked for the browser to settle, so one drawing serves every viewer.

// drawEvent draws what an event of a room's log changed.
func (s *Server) drawEvent(ctx context.Context, room string, event roomlog.Event) (roomlive.Drawn, error) {
	held, found := s.instances.Get(room)
	if !found || !held.Room {
		return roomlive.Drawn{}, roomlive.ErrNotFound
	}
	if event.Kind == instance.KindPeople {
		return roomlive.Drawn{Type: roomlive.FramePeople}, nil
	}
	if event.Kind != instance.KindTurn && event.Kind != instance.KindNote {
		return roomlive.Drawn{}, fmt.Errorf("an event of kind %q is not drawn", event.Kind)
	}
	if err := ctx.Err(); err != nil {
		return roomlive.Drawn{}, err
	}
	return s.drawItem(s.liveRoom(held), event)
}

// drawItem draws one turn or note of a conversation, in its place among the others, since whether it is grouped under the
// sender before it and whether it has a time above it depend on what came before.
func (s *Server) drawItem(held instance.Instance, event roomlog.Event) (roomlive.Drawn, error) {
	thread, found := s.instances.Thread(held.ID, event.Thread)
	if !found {
		return roomlive.Drawn{}, fmt.Errorf("no conversation %s in %s", event.Thread, held.ID)
	}
	timeline := s.roomTimeline(held, thread, s.sketchTurns(thread))
	at := slices.IndexFunc(timeline, func(each turn) bool { return each.ID == event.Ref })
	if at < 0 {
		return roomlive.Drawn{}, fmt.Errorf("%s is not in conversation %s", event.Ref, event.Thread)
	}
	shown := timeline[at]
	if !shown.Note {
		ledger, _ := s.spending()
		full := s.fullTurn(shown.Record, ledger)
		full.roomTurn = shown.roomTurn
		full.Files = s.fileCards(held.ID, full.Changed, full.Deleted)
		shown = full
	}
	var html bytes.Buffer
	data := map[string]any{"T": shown, "P": s.fragmentPage(held, thread)}
	if err := s.templates.ExecuteTemplate(&html, "room-turn", data); err != nil {
		return roomlive.Drawn{}, err
	}
	kind := roomlive.FrameTurn
	if shown.Note {
		kind = roomlive.FrameNote
	}
	return roomlive.Drawn{Type: kind, Thread: thread.ID, ID: shown.ID, HTML: html.String()}, nil
}

// sketchTurns is a conversation's turns with what the timeline needs of them and no more: who spoke and when, without reading
// any turn's files.
func (s *Server) sketchTurns(thread instance.Thread) []turn {
	turns := make([]turn, 0, len(thread.Turns))
	for _, id := range thread.Turns {
		if record, found := s.index.Get(id); found {
			turns = append(turns, turn{Record: record, Said: record.Brief})
		}
	}
	return turns
}

// fragmentPage is the page's own data that a turn is drawn with, for no viewer in particular: nobody is "me", and what only the
// room's owner is offered is drawn hidden, for the browser to show to the owner.
func (s *Server) fragmentPage(held instance.Instance, thread instance.Thread) map[string]any {
	return map[string]any{
		"Instance":    held,
		"Thread":      thread,
		"Path":        threadPath(thread),
		"Names":       s.roomNames(held),
		"Me":          "",
		"Sharing":     false,
		"ShareHidden": s.apps != nil,
	}
}
