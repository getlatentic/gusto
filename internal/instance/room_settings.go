package instance

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxGuidelines is how many characters a room's guidelines may hold. Every turn of every agent in the room is given them, so they
// are short on purpose.
const MaxGuidelines = 2000

// errUnchanged is what a change to a room returns when the room is already as asked, so nothing is written and nothing is
// announced.
var errUnchanged = errors.New("the room is already so")

// SetGuidelines sets what the room's agents are told at the start of every turn. Empty clears it.
func (s *Store) SetGuidelines(id, text string) error {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
	if n := utf8.RuneCountInString(text); n > MaxGuidelines {
		return fmt.Errorf("room guidelines hold at most %d characters, and these are %d", MaxGuidelines, n)
	}
	return s.changeRoom(id, func(held *Instance) error {
		if held.Guidelines == text {
			return errUnchanged
		}
		held.Guidelines = text
		return nil
	})
}

// SetArchived archives a room, or brings it back. A person's own conversation with an agent is not archived.
func (s *Store) SetArchived(id string, on bool) error {
	return s.changeRoom(id, func(held *Instance) error {
		if on == (held.Archived != 0) {
			return errUnchanged
		}
		if on && held.Direct {
			return fmt.Errorf("a conversation of your own is not archived")
		}
		held.Archived = 0
		if on {
			held.Archived = time.Now().Unix()
		}
		return nil
	})
}

// SetMuted mutes a room for one of its people, or unmutes it.
func (s *Store) SetMuted(id, who string, on bool) error {
	return s.changeRoom(id, func(held *Instance) error {
		for i, member := range held.Members {
			if !strings.EqualFold(member.Who, strings.TrimSpace(who)) {
				continue
			}
			if member.Muted == on {
				return errUnchanged
			}
			held.Members[i].Muted = on
			return nil
		}
		return fmt.Errorf("%s is not in this room", who)
	})
}

// MutedBy says whether a person has muted the room.
func (held Instance) MutedBy(who string) bool {
	who = strings.TrimSpace(who)
	for _, member := range held.Members {
		if strings.EqualFold(member.Who, who) {
			return member.Muted
		}
	}
	return false
}
