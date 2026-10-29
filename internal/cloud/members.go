package cloud

import (
	"context"
	"errors"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
)

// Refusals a member's invitation or removal can meet.
var (
	ErrNotAdmin     = errors.New("only an admin can do that")
	ErrNoInvites    = errors.New("this organization cannot invite people until its admin has confirmed their email address")
	ErrLapsed       = errors.New("this organization's subscription has ended, so it cannot invite people until an admin renews it")
	ErrFull         = errors.New("this organization has as many people as it may")
	ErrCannotInvite = errors.New("that address cannot be invited")
	ErrLastAdmin    = errors.New("an organization keeps its last admin")
	ErrNoMember     = errors.New("nobody by that address is in this organization")
)

// Invitation is an invitation that has not been answered.
type Invitation struct {
	Email   string    `json:"email"`
	Role    Role      `json:"role"`
	By      string    `json:"by"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires"`
	// Link is the digest of the link's secret, which is how a canceled
	// invitation's link is taken away without anyone holding the secret.
	Link string `json:"link"`
}

// InviteLife is how long an invitation can be accepted.
const InviteLife = 7 * 24 * time.Hour

// Members is the people in an organization, its admins first and then in the order
// they joined.
func (s *Store) Members(slug string) ([]Person, error) {
	return membersOf(context.Background(), s.db, slug)
}

func membersOf(ctx context.Context, q dbkit.Querier, slug string) ([]Person, error) {
	return collect(ctx, q, scanPerson, `SELECT `+personColumns+personFrom+` WHERE m.org = ?
		ORDER BY CASE WHEN m.role = 'admin' THEN 0 ELSE 1 END, m.created, m.email`, slug)
}

// Invitations is what an organization has sent that nobody has answered, oldest
// first. One that ran out is not listed.
func (s *Store) Invitations(slug string) ([]Invitation, error) {
	return s.openInvitations(context.Background(), s.db, slug)
}

// Invite makes an invitation for an address, from an admin. It is refused unless the
// organization may invite, has room for one more person counting those already
// invited, and the address is not in this organization already. An address that is
// in another organization is invited like any other, so the form says nothing of
// who has an account elsewhere.
func (s *Store) Invite(slug string, by Person, address string) (secret string, err error) {
	email, err := NormalEmail(address)
	if err != nil {
		return "", err
	}
	if err := workEmail(email, true, s.refused); err != nil {
		return "", err
	}
	if by.Org != slug || by.Role != Admin {
		return "", ErrNotAdmin
	}
	quota, err := s.Quota(slug)
	if err != nil {
		return "", err
	}
	if !quota.Invites {
		if billing, found, err := s.Billing(slug); err == nil && found && billing.Lapsed() {
			return "", ErrLapsed
		}
		return "", ErrNoInvites
	}
	ctx := context.Background()
	err = s.db.InTx(ctx, func(tx dbkit.Tx) (err error) {
		secret, err = s.invite(ctx, tx, slug, by.Email, email, quota.Members)
		return err
	})
	if err != nil {
		return "", err
	}
	s.publishRoster(slug)
	return secret, nil
}

func (s *Store) invite(ctx context.Context, tx dbkit.Tx, slug, by, email string, most int) (string, error) {
	if err := s.lock(ctx, tx, "org:"+slug); err != nil {
		return "", err
	}
	if _, found, err := fetchMember(ctx, tx, email, slug); err != nil || found {
		if err == nil {
			err = ErrCannotInvite
		}
		return "", err
	}
	if err := s.roomFor(ctx, tx, slug, most, email); err != nil {
		return "", err
	}
	now := s.now().UTC().Truncate(time.Second)
	secret, err := s.putLink(ctx, tx, link{Email: email, Purpose: Invite, Expires: now.Add(InviteLife), Org: slug, Role: Runner, By: by})
	if err != nil {
		return "", err
	}
	invitation := Invitation{Email: email, Role: Runner, By: by, Created: now, Expires: now.Add(InviteLife), Link: digest(secret)}
	// Inviting an address again replaces the earlier invitation, and takes its link away.
	if err := dropInvitationLink(ctx, tx, slug, email); err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, putInvitation.SQL(1), s.invitationArgs(slug, invitation)...)
	return secret, err
}

// roomFor is a refusal when one more person would pass the number the organization may
// have. People and the invitations that are open both count, and an address that is
// already invited does not count twice.
func (s *Store) roomFor(ctx context.Context, tx dbkit.Tx, slug string, most int, email string) error {
	members, err := count(ctx, tx, `SELECT count(*) FROM memberships WHERE org = ?`, slug)
	if err != nil {
		return err
	}
	open, err := count(ctx, tx, `SELECT count(*) FROM invitations WHERE org = ? AND expires > ? AND email <> ?`,
		slug, s.stamp(s.now()), email)
	if err != nil {
		return err
	}
	if members+open >= most {
		return ErrFull
	}
	return nil
}

// Cancel takes an invitation back, and its link with it.
func (s *Store) Cancel(slug string, by Person, address string) error {
	email, err := NormalEmail(address)
	if err != nil {
		return err
	}
	if by.Org != slug || by.Role != Admin {
		return ErrNotAdmin
	}
	ctx := context.Background()
	err = s.db.InTx(ctx, func(tx dbkit.Tx) error {
		if err := dropInvitationLink(ctx, tx, slug, email); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM invitations WHERE org = ? AND email = ?`, slug, email)
		return err
	})
	if err != nil {
		return err
	}
	s.publishRoster(slug)
	return nil
}
