package cloud

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
	"github.com/getlatentic/gusto/internal/roster"
)

// RosterFile is the list of people the organization's server reads, and InboxDir is
// where the front door leaves it what its admins did, for its change record.
func (s *Store) RosterFile(slug string) string { return filepath.Join(s.DataDir(slug), "members.json") }
func (s *Store) InboxDir(slug string) string   { return filepath.Join(s.DataDir(slug), "inbox") }

// PublishRoster writes the organization's people where its server reads them. The people are read and the
// file is written under a lock, so two front doors that publish together leave the list of the later read.
func (s *Store) PublishRoster(slug string) error {
	ctx := context.Background()
	return s.db.InTx(ctx, func(tx dbkit.Tx) error {
		if err := s.lock(ctx, tx, "roster:"+slug); err != nil {
			return err
		}
		return s.writeRoster(ctx, tx, slug)
	})
}

// publishRoster is PublishRoster for a change that has already been made. The
// change stands whether or not the list is written, so a failure is said loudly and
// does not undo it: the next change writes the list again, and so does a restart.
func (s *Store) publishRoster(slug string) {
	if err := s.PublishRoster(slug); err != nil {
		slog.Error("an organization's list of people was not written", "org", slug, "error", err)
	}
}

func (s *Store) writeRoster(ctx context.Context, q dbkit.Querier, slug string) error {
	members, err := membersOf(ctx, q, slug)
	if err != nil {
		return err
	}
	invited, err := s.openInvitations(ctx, q, slug)
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

// RecordSignIn notes that a person was signed in to their organization's server, and
// tells the server.
func (s *Store) RecordSignIn(email string) error {
	ctx := context.Background()
	person, found, err := fetchPerson(ctx, s.db, email)
	if err != nil || !found {
		return err
	}
	at := s.now().UTC().Truncate(time.Second)
	if _, err := s.db.ExecContext(ctx, `UPDATE people SET last_sign_in = ? WHERE email = ?`, s.stamp(at), person.Email); err != nil {
		return err
	}
	s.publishRoster(person.Org)
	return nil
}
