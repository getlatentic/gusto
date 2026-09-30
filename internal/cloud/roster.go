package cloud

import (
	"log/slog"
	"path/filepath"
	"time"

	"github.com/getlatentic/haas/internal/roster"
)

// RosterFile is the list of people the organisation's server reads, and InboxDir is
// where the front door leaves it what its admins did, for its change record.
func (s *Store) RosterFile(slug string) string { return filepath.Join(s.DataDir(slug), "members.json") }
func (s *Store) InboxDir(slug string) string   { return filepath.Join(s.DataDir(slug), "inbox") }

// PublishRoster writes the organisation's people where its server reads them.
func (s *Store) PublishRoster(slug string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeRoster(slug)
}

// publishRoster is PublishRoster for a change that has already been made. The
// change stands whether or not the list is written, so a failure is said loudly and
// does not undo it: the next change writes the list again, and so does a restart.
// The caller holds the lock.
func (s *Store) publishRoster(slug string) {
	if err := s.writeRoster(slug); err != nil {
		slog.Error("an organisation's list of people was not written", "org", slug, "error", err)
	}
}

func (s *Store) writeRoster(slug string) error {
	members, err := s.members(slug)
	if err != nil {
		return err
	}
	invited, err := s.invitations(slug)
	if err != nil {
		return err
	}
	list := roster.Roster{Written: s.now().UTC().Truncate(time.Second), Members: []roster.Member{}, Invited: []roster.Invited{}}
	for _, person := range members {
		list.Members = append(list.Members, roster.Member{
			Email: person.Email, Role: string(person.Role), Joined: person.Created, LastSignIn: person.LastSignIn,
		})
	}
	for _, invitation := range invited {
		list.Invited = append(list.Invited, roster.Invited{
			Email: invitation.Email, By: invitation.By, Created: invitation.Created, Expires: invitation.Expires,
		})
	}
	return roster.Write(s.RosterFile(slug), list)
}

// RecordSignIn notes that a person was signed in to their organisation's server, and
// tells the server.
func (s *Store) RecordSignIn(email string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	person, found, err := s.person(email)
	if err != nil || !found {
		return err
	}
	person.LastSignIn = s.now().UTC().Truncate(time.Second)
	if err := writeFile(s.personFile(person.Email), person, 0o600); err != nil {
		return err
	}
	s.publishRoster(person.Org)
	return nil
}
