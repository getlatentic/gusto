package cloud

import (
	"context"

	"github.com/getlatentic/gusto/internal/dbkit"
)

var invitationFields = []string{"org", "email", "role", "invited_by", "created", "expires", "link"}

var putInvitation = dbkit.Upsert{Table: "invitations", Columns: invitationFields, Conflict: []string{"org", "email"},
	Update: []string{"role", "invited_by", "created", "expires", "link"}}

func (s *Store) invitationArgs(slug string, i Invitation) []any {
	return []any{slug, i.Email, string(i.Role), i.By, s.stamp(i.Created), s.stamp(i.Expires), i.Link}
}

func (s *Store) scanInvitation(r rowScanner) (Invitation, error) {
	var i Invitation
	var role string
	var created, expires dbkit.Time
	if err := r.Scan(&i.Email, &role, &i.By, &created, &expires, &i.Link); err != nil {
		return Invitation{}, err
	}
	i.Role, i.Created, i.Expires = Role(role), created.Time, expires.Time
	return i, nil
}

func (s *Store) openInvitations(ctx context.Context, q dbkit.Querier, slug string) ([]Invitation, error) {
	return collect(ctx, q, s.scanInvitation, `SELECT email, role, invited_by, created, expires, link FROM invitations
		WHERE org = ? AND expires > ? ORDER BY created, email`, slug, s.stamp(s.now()))
}

// dropInvitationLink takes away the link of the invitation an address has, if it has one.
func dropInvitationLink(ctx context.Context, tx dbkit.Tx, slug, email string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM links WHERE hash IN (SELECT link FROM invitations WHERE org = ? AND email = ?)`, slug, email)
	return err
}
