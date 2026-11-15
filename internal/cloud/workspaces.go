package cloud

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/getlatentic/gusto/internal/dbkit"
	"github.com/getlatentic/gusto/internal/workspace"
)

// Refusals a change to a workspace can meet.
var (
	ErrNoWorkspace    = errors.New("this organization has no workspace by that name")
	ErrManyWorkspaces = errors.New("this organization has as many workspaces as it may")
	ErrWorkspaceName  = errors.New("a workspace's name is between 1 and 60 characters")
	ErrLastWorkspace  = errors.New("a person stays in at least one workspace, so remove them from the organization instead")
	ErrInWorkspace    = errors.New("that person is in this workspace already")
	ErrNotInWorkspace = errors.New("that person is not in this workspace")
	errWorkspaceTaken = errors.New("a workspace id is taken")
)

// firstWorkspaceRows give every organization with no workspace its first, named as the organization, and put everybody
// in no workspace in it.
var firstWorkspaceRows = []string{
	`INSERT INTO workspaces (org, id, name, created) SELECT slug, '` + workspace.First + `', name, created FROM orgs o
			WHERE NOT EXISTS (SELECT 1 FROM workspaces w WHERE w.org = o.slug)`,
	`INSERT INTO workspace_members (org, workspace, email, created) SELECT org, '` + workspace.First + `', email, created FROM memberships m
			WHERE NOT EXISTS (SELECT 1 FROM workspace_members w WHERE w.org = m.org AND w.email = m.email)`,
}

// Workspace is a team's space in an organization, and the addresses of the people in it, in the order they joined.
type Workspace struct {
	ID      string
	Name    string
	Created time.Time
	Members []string
}

// Workspaces is an organization's workspaces, the first first and then in the order they were made.
func (s *Store) Workspaces(ctx context.Context, slug string) ([]Workspace, error) {
	return workspacesOf(ctx, s.db, slug)
}

func workspacesOf(ctx context.Context, q dbkit.Querier, slug string) ([]Workspace, error) {
	spaces, err := collect(ctx, q, scanWorkspace, `SELECT id, name, created FROM workspaces WHERE org = ?
		ORDER BY CASE WHEN id = ? THEN 0 ELSE 1 END, created, id`, slug, workspace.First)
	if err != nil {
		return nil, err
	}
	type membership struct{ space, email string }
	rows, err := collect(ctx, q, func(r rowScanner) (membership, error) {
		var m membership
		return m, r.Scan(&m.space, &m.email)
	}, `SELECT workspace, email FROM workspace_members WHERE org = ? ORDER BY created, email`, slug)
	if err != nil {
		return nil, err
	}
	at := make(map[string]int, len(spaces))
	for i, space := range spaces {
		at[space.ID] = i
		spaces[i].Members = []string{}
	}
	for _, row := range rows {
		if i, found := at[row.space]; found {
			spaces[i].Members = append(spaces[i].Members, row.email)
		}
	}
	return spaces, nil
}

func scanWorkspace(r rowScanner) (Workspace, error) {
	var w Workspace
	var created dbkit.Time
	err := r.Scan(&w.ID, &w.Name, &created)
	w.Created = created.Time
	return w, err
}

// SpacesOf is the workspaces a person is in, by organization, each organization's first first.
func (s *Store) SpacesOf(ctx context.Context, address string) (map[string][]Workspace, error) {
	email, err := NormalEmail(address)
	if err != nil {
		return nil, nil
	}
	type held struct {
		org   string
		space Workspace
	}
	rows, err := collect(ctx, s.db, func(r rowScanner) (held, error) {
		var h held
		var created dbkit.Time
		err := r.Scan(&h.org, &h.space.ID, &h.space.Name, &created)
		h.space.Created = created.Time
		return h, err
	}, `SELECT w.org, w.id, w.name, w.created FROM workspaces w
		JOIN workspace_members m ON m.org = w.org AND m.workspace = w.id WHERE m.email = ?
		ORDER BY w.org, CASE WHEN w.id = ? THEN 0 ELSE 1 END, w.created, w.id`, email, workspace.First)
	if err != nil {
		return nil, err
	}
	by := map[string][]Workspace{}
	for _, row := range rows {
		by[row.org] = append(by[row.org], row.space)
	}
	return by, nil
}

// MakeWorkspace makes a workspace in the organization, from an admin, who is its first member.
func (s *Store) MakeWorkspace(ctx context.Context, slug string, by Person, name string) (Workspace, error) {
	name, err := workspaceName(name)
	if err != nil {
		return Workspace{}, err
	}
	if by.Org != slug || by.Role != Admin {
		return Workspace{}, ErrNotAdmin
	}
	var made Workspace
	err = s.db.InTx(ctx, func(tx dbkit.Tx) (err error) {
		made, err = s.makeWorkspaceIn(ctx, tx, slug, by.Email, name)
		return err
	})
	if err != nil {
		return Workspace{}, err
	}
	s.publishRoster(ctx, slug)
	return made, nil
}

func (s *Store) makeWorkspaceIn(ctx context.Context, tx dbkit.Tx, slug, by, name string) (Workspace, error) {
	if err := s.lock(ctx, tx, "org:"+slug); err != nil {
		return Workspace{}, err
	}
	n, err := count(ctx, tx, `SELECT count(*) FROM workspaces WHERE org = ?`, slug)
	if err != nil {
		return Workspace{}, err
	}
	if n >= workspace.MaxPerOrg {
		return Workspace{}, ErrManyWorkspaces
	}
	now := s.now().UTC().Truncate(time.Second)
	base := workspace.IDFor(name)
	for try := 1; try <= workspace.MaxPerOrg+1; try++ {
		id := base
		if try > 1 {
			suffix := "-" + strconv.Itoa(try)
			id = strings.TrimRight(base[:min(len(base), 32-len(suffix))], "-") + suffix
		}
		err := s.putWorkspace(ctx, tx, slug, Workspace{ID: id, Name: name, Created: now}, by)
		if errors.Is(err, errWorkspaceTaken) {
			continue
		}
		return Workspace{ID: id, Name: name, Created: now, Members: []string{by}}, err
	}
	return Workspace{}, ErrManyWorkspaces
}

// putWorkspace makes a workspace with one member, or says its id is taken.
func (s *Store) putWorkspace(ctx context.Context, tx dbkit.Tx, slug string, w Workspace, member string) error {
	ok, err := inserted(tx.ExecContext(ctx, `INSERT INTO workspaces (org, id, name, created) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		slug, w.ID, w.Name, s.stamp(w.Created)))
	if err != nil {
		return err
	}
	if !ok {
		return errWorkspaceTaken
	}
	return s.putInWorkspace(ctx, tx, slug, w.ID, member)
}

func (s *Store) putInWorkspace(ctx context.Context, tx dbkit.Tx, slug, id, email string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO workspace_members (org, workspace, email, created) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		slug, id, email, s.stamp(s.now().UTC().Truncate(time.Second)))
	return err
}

// RenameWorkspace gives a workspace another name. Its id, which its records are kept under, stays.
func (s *Store) RenameWorkspace(ctx context.Context, slug string, by Person, id, name string) error {
	name, err := workspaceName(name)
	if err != nil {
		return err
	}
	if by.Org != slug || by.Role != Admin {
		return ErrNotAdmin
	}
	done, err := s.db.ExecContext(ctx, `UPDATE workspaces SET name = ? WHERE org = ? AND id = ?`, name, slug, id)
	if err != nil {
		return err
	}
	if changed, err := done.RowsAffected(); err != nil || changed == 0 {
		if err == nil {
			err = ErrNoWorkspace
		}
		return err
	}
	s.publishRoster(ctx, slug)
	return nil
}

// AddToWorkspace puts a person of the organization in one of its workspaces, from an admin.
func (s *Store) AddToWorkspace(ctx context.Context, slug string, by Person, id, address string) error {
	email, err := NormalEmail(address)
	if err != nil {
		return ErrNoMember
	}
	if by.Org != slug || by.Role != Admin {
		return ErrNotAdmin
	}
	err = s.db.InTx(ctx, func(tx dbkit.Tx) error { return s.addToWorkspaceIn(ctx, tx, slug, id, email) })
	if err != nil {
		return err
	}
	s.publishRoster(ctx, slug)
	return nil
}

// RemoveFromWorkspace takes a person out of one workspace, from an admin, and marks their computer in it for removal.
// Their computers in other workspaces stay. A person stays in at least one workspace while they are in the organization.
func (s *Store) RemoveFromWorkspace(ctx context.Context, slug string, by Person, id, address string) error {
	email, err := NormalEmail(address)
	if err != nil {
		return ErrNoMember
	}
	if by.Org != slug || by.Role != Admin {
		return ErrNotAdmin
	}
	err = s.db.InTx(ctx, func(tx dbkit.Tx) error {
		if err := s.lock(ctx, tx, "org:"+slug); err != nil {
			return err
		}
		return removeFromWorkspaceIn(ctx, tx, slug, id, email)
	})
	if err != nil {
		return err
	}
	s.publishRoster(ctx, slug)
	registry, err := s.Computers(slug)
	if err != nil {
		return err
	}
	return registry.RetireIn(email, id)
}

func (s *Store) addToWorkspaceIn(ctx context.Context, tx dbkit.Tx, slug, id, email string) error {
	if err := s.lock(ctx, tx, "org:"+slug); err != nil {
		return err
	}
	if err := mustBeWorkspace(ctx, tx, slug, id); err != nil {
		return err
	}
	if _, found, err := fetchMember(ctx, tx, email, slug); err != nil || !found {
		if err == nil {
			err = ErrNoMember
		}
		return err
	}
	ok, err := inserted(tx.ExecContext(ctx, `INSERT INTO workspace_members (org, workspace, email, created) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		slug, id, email, s.stamp(s.now().UTC().Truncate(time.Second))))
	if err == nil && !ok {
		err = ErrInWorkspace
	}
	return err
}

func removeFromWorkspaceIn(ctx context.Context, tx dbkit.Tx, slug, id, email string) error {
	here, err := count(ctx, tx, `SELECT count(*) FROM workspace_members WHERE org = ? AND workspace = ? AND email = ?`, slug, id, email)
	if err != nil {
		return err
	}
	if here == 0 {
		return ErrNotInWorkspace
	}
	in, err := count(ctx, tx, `SELECT count(*) FROM workspace_members WHERE org = ? AND email = ?`, slug, email)
	if err != nil {
		return err
	}
	if in <= 1 {
		return ErrLastWorkspace
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM workspace_members WHERE org = ? AND workspace = ? AND email = ?`, slug, id, email)
	return err
}

func mustBeWorkspace(ctx context.Context, q dbkit.Querier, slug, id string) error {
	n, err := count(ctx, q, `SELECT count(*) FROM workspaces WHERE org = ? AND id = ?`, slug, id)
	if err == nil && n == 0 {
		err = ErrNoWorkspace
	}
	return err
}

// joinWorkspace is the workspace a person who accepts an invitation joins: the one it names while it is there, and the
// first otherwise.
func joinWorkspace(ctx context.Context, q dbkit.Querier, slug, id string) (string, error) {
	if id == "" || id == workspace.First {
		return workspace.First, nil
	}
	if err := mustBeWorkspace(ctx, q, slug, id); err != nil {
		if errors.Is(err, ErrNoWorkspace) {
			return workspace.First, nil
		}
		return "", err
	}
	return id, nil
}

// fillFirstWorkspaces runs firstWorkspaceRows.
func fillFirstWorkspaces(ctx context.Context, tx dbkit.Tx) error {
	for _, statement := range firstWorkspaceRows {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func workspaceName(name string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" || len([]rune(name)) > 60 || strings.ContainsFunc(name, unicode.IsControl) {
		return "", ErrWorkspaceName
	}
	return name, nil
}
