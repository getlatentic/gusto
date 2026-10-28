package cloud

import (
	"context"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
)

// Accept puts the person an invitation's link was mailed to in its organization, and
// makes their account when they have none; a person already in another organization
// keeps it and is in both. They are verified, because they read the mail the link
// was in, and they are a runner. The room is checked again, since
// the organization may have filled while the invitation was out. A link that is
// followed is spent whether or not it gets a person in.
func (s *Store) Accept(secret string) (Person, Org, error) {
	if secret == "" || len(secret) > longestSecret {
		return Person{}, Org{}, ErrLink
	}
	ctx := context.Background()
	var joined acceptance
	err := s.db.InTx(ctx, func(tx dbkit.Tx) (err error) {
		joined, err = s.acceptIn(ctx, tx, digest(secret))
		return err
	})
	if err != nil {
		return Person{}, Org{}, err
	}
	if joined.refusal != nil {
		return Person{}, Org{}, joined.refusal
	}
	s.publishRoster(joined.org.Slug)
	return joined.person, joined.org, nil
}

// acceptance is what a transaction decided: the person made, or the refusal. A refusal is returned
// after the transaction commits, since the link it spent stays spent.
type acceptance struct {
	person  Person
	org     Org
	refusal error
}

func (s *Store) acceptIn(ctx context.Context, tx dbkit.Tx, hash string) (acceptance, error) {
	// The organization is locked before the link is deleted, in the order Invite takes them.
	var slug string
	if err := tx.QueryRowContext(ctx, `SELECT org FROM links WHERE hash = ? AND purpose = ?`, hash, string(Invite)).Scan(&slug); err == nil {
		if err := s.lock(ctx, tx, "org:"+slug); err != nil {
			return acceptance{}, err
		}
	}
	invited, found, err := s.spend(ctx, tx, hash)
	if err != nil {
		return acceptance{}, err
	}
	if !found || invited.Purpose != Invite || !s.now().Before(invited.Expires) {
		return acceptance{refusal: ErrLink}, nil
	}
	org, found, err := fetchOrg(ctx, tx, invited.Org)
	if err != nil {
		return acceptance{}, err
	}
	if !found {
		return acceptance{refusal: ErrLink}, nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM invitations WHERE org = ? AND email = ?`, org.Slug, invited.Email); err != nil {
		return acceptance{}, err
	}
	return s.join(ctx, tx, org, invited)
}

func (s *Store) join(ctx context.Context, tx dbkit.Tx, org Org, invited link) (acceptance, error) {
	quota, err := s.Quota(org.Slug)
	if err != nil {
		return acceptance{}, err
	}
	members, err := count(ctx, tx, `SELECT count(*) FROM memberships WHERE org = ?`, org.Slug)
	if err != nil {
		return acceptance{}, err
	}
	if members >= quota.Members {
		return acceptance{refusal: ErrFull}, nil
	}
	now := s.now().UTC().Truncate(time.Second)
	person := Person{Email: invited.Email, Org: org.Slug, Role: invited.Role, Created: now, Verified: now, By: invited.By}
	made, err := s.addMember(ctx, tx, person)
	if err != nil {
		return acceptance{}, err
	}
	if !made {
		return acceptance{refusal: ErrEmailTaken}, nil
	}
	return acceptance{person: person, org: org}, nil
}
