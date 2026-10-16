package desk

import (
	"fmt"

	"github.com/getlatentic/gusto/internal/helper"
)

// Slots is how many turns the menu lists.
const Slots = 8

// Entry is one line of the menu. An entry with no address is text.
type Entry struct {
	Label string
	Page  string
}

// Menu is the lines of the menu for what the helper knows: the line that says how it stands, then the newest turns, running
// ones before the ones that ended.
func Menu(server string, status helper.Status, turns []helper.Turn) []Entry {
	lines := []Entry{{Label: standing(status)}}
	for _, turn := range ordered(turns) {
		if len(lines) > Slots {
			break
		}
		lines = append(lines, Entry{Label: fmt.Sprintf("%s — %s", short(turn.Brief), stateWord(turn.State)), Page: PageOf(server, turn)})
	}
	return lines
}

func standing(status helper.Status) string {
	switch {
	case !status.SignedIn:
		return "Not signed in: run gusto-helper login"
	case !status.Connected:
		return "Reconnecting…"
	}
	return "Signed in as " + status.Person
}

// ordered puts the turns still going first, each group keeping its order.
func ordered(turns []helper.Turn) []helper.Turn {
	out := make([]helper.Turn, 0, len(turns))
	for _, turn := range turns {
		if !ended(turn.State) {
			out = append(out, turn)
		}
	}
	for _, turn := range turns {
		if ended(turn.State) {
			out = append(out, turn)
		}
	}
	return out
}
