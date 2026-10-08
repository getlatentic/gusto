package desk

import (
	"context"

	"github.com/getlatentic/haas/internal/helper"
)

// Announcer says when a turn the person was waiting on ends. It speaks of a turn it saw going, so that the turns the server
// tells about again after a reconnect, or at the start, do not each become a notification.
type Announcer struct {
	Hub    *helper.Hub
	Server string
	Say    func(Note)

	going map[string]bool
}

// Run listens until the context ends.
func (a *Announcer) Run(ctx context.Context) {
	a.going = map[string]bool{}
	for _, turn := range a.Hub.Turns() {
		a.going[turn.Job] = !ended(turn.State)
	}
	events, stop := a.Hub.Listen()
	defer stop()
	for {
		select {
		case <-ctx.Done():
			return
		case event := <-events:
			a.hear(event.Turn)
		}
	}
}

func (a *Announcer) hear(turn helper.Turn) {
	wasGoing := a.going[turn.Job]
	a.going[turn.Job] = !ended(turn.State)
	if wasGoing && ended(turn.State) {
		a.Say(Note{Title: "Your agent " + stateWord(turn.State), Body: short(turn.Brief)})
	}
}
