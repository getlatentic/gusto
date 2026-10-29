package cloud

import (
	"context"
	"errors"

	"github.com/getlatentic/gusto/internal/dbkit"
)

// ErrBadRole is the refusal of a role that is neither of the two there are.
var ErrBadRole = errors.New("a role is admin or runner")

// SetRole changes the role of a person in the organization, and says what it was. An
// admin may change anybody's, and the organization keeps at least one admin, so the
// last cannot be made a runner.
func (s *Store) SetRole(slug string, by Person, address string, role Role) (was Role, err error) {
	if role != Admin && role != Runner {
		return "", ErrBadRole
	}
	email, err := NormalEmail(address)
	if err != nil {
		return "", ErrNoMember
	}
	if by.Org != slug || by.Role != Admin {
		return "", ErrNotAdmin
	}
	ctx := context.Background()
	err = s.db.InTx(ctx, func(tx dbkit.Tx) (err error) {
		was, err = s.setRoleIn(ctx, tx, slug, email, role)
		return err
	})
	if err != nil {
		return "", err
	}
	if was != role {
		s.publishRoster(slug)
	}
	return was, nil
}

func (s *Store) setRoleIn(ctx context.Context, tx dbkit.Tx, slug, email string, role Role) (Role, error) {
	if err := s.lock(ctx, tx, "org:"+slug); err != nil {
		return "", err
	}
	target, found, err := fetchMember(ctx, tx, email, slug)
	if err != nil {
		return "", err
	}
	if !found {
		return "", ErrNoMember
	}
	if target.Role == role {
		return role, nil
	}
	if target.Role == Admin {
		if admins, err := adminsOf(ctx, tx, slug); err != nil || admins <= 1 {
			if err == nil {
				err = ErrLastAdmin
			}
			return "", err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE memberships SET role = ? WHERE email = ? AND org = ?`, string(role), email, slug)
	return target.Role, err
}
