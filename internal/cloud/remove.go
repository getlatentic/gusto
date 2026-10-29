package cloud

import (
	"context"

	"github.com/getlatentic/gusto/internal/dbkit"
)

// Remove takes a person out of an organization, and leaves them in any other they are in.
// An admin may remove anybody but themselves and the last admin, so an organization is
// never left with nobody to run it.
func (s *Store) Remove(slug string, by Person, address string) error {
	email, err := NormalEmail(address)
	if err != nil {
		return ErrNoMember
	}
	if by.Org != slug || by.Role != Admin {
		return ErrNotAdmin
	}
	ctx := context.Background()
	if err := s.db.InTx(ctx, func(tx dbkit.Tx) error { return s.removeIn(ctx, tx, slug, by.Email, email) }); err != nil {
		return err
	}
	s.publishRoster(slug)
	// Their computer is marked, and the organization's server, which reaches the gateway,
	// removes it. A person who never had one has nothing marked.
	registry, err := s.Computers(slug)
	if err != nil {
		return err
	}
	return registry.Retire(email)
}

func (s *Store) removeIn(ctx context.Context, tx dbkit.Tx, slug, by, email string) error {
	if err := s.lock(ctx, tx, "org:"+slug); err != nil {
		return err
	}
	target, found, err := fetchMember(ctx, tx, email, slug)
	if err != nil {
		return err
	}
	if !found {
		return ErrNoMember
	}
	if target.Role == Admin {
		admins, err := adminsOf(ctx, tx, slug)
		if err != nil {
			return err
		}
		if admins <= 1 || target.Email == by {
			return ErrLastAdmin
		}
	}
	return dropMember(ctx, tx, email, slug)
}

// adminsOf is how many admins the organization has.
func adminsOf(ctx context.Context, q dbkit.Querier, slug string) (int, error) {
	return count(ctx, q, `SELECT count(*) FROM memberships WHERE org = ? AND role = ?`, slug, string(Admin))
}
