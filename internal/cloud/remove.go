package cloud

import (
	"context"

	"github.com/getlatentic/gusto/internal/dbkit"
	"github.com/getlatentic/gusto/internal/workspace"
)

// Remove takes a person out of an organization, and leaves them in any other they are in.
// An admin may remove anybody but themselves and the last admin, so an organization is
// never left with nobody to run it.
func (s *Store) Remove(ctx context.Context, slug string, by Person, address string) error {
	email, err := NormalEmail(address)
	if err != nil {
		return ErrNoMember
	}
	if by.Org != slug || by.Role != Admin {
		return ErrNotAdmin
	}

	var spaces []string
	err = s.db.InTx(ctx, func(tx dbkit.Tx) (err error) {
		spaces, err = s.removeIn(ctx, tx, slug, by.Email, email)
		return err
	})
	if err != nil {
		return err
	}
	s.publishRoster(ctx, slug)
	// Their computer in each workspace they were in is marked, with or without a record, and the organization's server, which
	// keeps the files and reaches the gateway, removes it.
	registry, err := s.Computers(slug)
	if err != nil {
		return err
	}
	return registry.Retire(email, spaces...)
}

// removeIn drops the person and says which workspaces they were in.
func (s *Store) removeIn(ctx context.Context, tx dbkit.Tx, slug, by, email string) ([]string, error) {
	if err := s.lock(ctx, tx, "org:"+slug); err != nil {
		return nil, err
	}
	target, found, err := fetchMember(ctx, tx, email, slug)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNoMember
	}
	if target.Role == Admin {
		admins, err := adminsOf(ctx, tx, slug)
		if err != nil {
			return nil, err
		}
		if admins <= 1 || target.Email == by {
			return nil, ErrLastAdmin
		}
	}
	spaces, err := workspaceIDsOf(ctx, tx, slug, email)
	if err != nil {
		return nil, err
	}
	return spaces, dropMember(ctx, tx, email, slug)
}

// workspaceIDsOf is the workspaces a person is in, as the ids their computers are named by. A person in none is in the first.
func workspaceIDsOf(ctx context.Context, q dbkit.Querier, slug, email string) ([]string, error) {
	spaces, err := collect(ctx, q, func(r rowScanner) (string, error) {
		var id string
		return id, r.Scan(&id)
	}, `SELECT workspace FROM workspace_members WHERE org = ? AND email = ?`, slug, email)
	if err == nil && len(spaces) == 0 {
		spaces = []string{workspace.First}
	}
	return spaces, err
}

// adminsOf is how many admins the organization has.
func adminsOf(ctx context.Context, q dbkit.Querier, slug string) (int, error) {
	return count(ctx, q, `SELECT count(*) FROM memberships WHERE org = ? AND role = ?`, slug, string(Admin))
}
