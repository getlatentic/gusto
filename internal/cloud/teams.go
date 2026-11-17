package cloud

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/getlatentic/gusto/internal/dbkit"
	"github.com/getlatentic/gusto/internal/team"
)

// Refusals a change to a team can meet.
var (
	ErrNoTeam    = errors.New("this organization has no team by that name")
	ErrManyTeams = errors.New("this organization has as many teams as it may")
	ErrTeamName  = errors.New("a team's name is between 1 and 60 characters")
	ErrLastTeam  = errors.New("a person stays in at least one team, so remove them from the organization instead")
	ErrInTeam    = errors.New("that person is in this team already")
	ErrNotInTeam = errors.New("that person is not in this team")
	errTeamTaken = errors.New("a team id is taken")
)

// firstTeamRows give every organization with no team its first, named as the organization, and put everybody
// in no team in it.
var firstTeamRows = []string{
	`INSERT INTO workspaces (org, id, name, created) SELECT slug, '` + team.First + `', name, created FROM orgs o
			WHERE NOT EXISTS (SELECT 1 FROM workspaces w WHERE w.org = o.slug)`,
	`INSERT INTO workspace_members (org, workspace, email, created) SELECT org, '` + team.First + `', email, created FROM memberships m
			WHERE NOT EXISTS (SELECT 1 FROM workspace_members w WHERE w.org = m.org AND w.email = m.email)`,
}

// Team is one of an organization's teams, and the addresses of the people in it, in the order they joined.
type Team struct {
	ID      string
	Name    string
	Created time.Time
	Members []string
}

// Teams is an organization's teams, the first first and then in the order they were made.
func (s *Store) Teams(ctx context.Context, slug string) ([]Team, error) {
	return teamsOf(ctx, s.db, slug)
}

func teamsOf(ctx context.Context, q dbkit.Querier, slug string) ([]Team, error) {
	spaces, err := collect(ctx, q, scanTeam, `SELECT id, name, created FROM workspaces WHERE org = ?
		ORDER BY CASE WHEN id = ? THEN 0 ELSE 1 END, created, id`, slug, team.First)
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

func scanTeam(r rowScanner) (Team, error) {
	var w Team
	var created dbkit.Time
	err := r.Scan(&w.ID, &w.Name, &created)
	w.Created = created.Time
	return w, err
}

// SpacesOf is the teams a person is in, by organization, each organization's first first.
func (s *Store) SpacesOf(ctx context.Context, address string) (map[string][]Team, error) {
	email, err := NormalEmail(address)
	if err != nil {
		return nil, nil
	}
	type held struct {
		org   string
		space Team
	}
	rows, err := collect(ctx, s.db, func(r rowScanner) (held, error) {
		var h held
		var created dbkit.Time
		err := r.Scan(&h.org, &h.space.ID, &h.space.Name, &created)
		h.space.Created = created.Time
		return h, err
	}, `SELECT w.org, w.id, w.name, w.created FROM workspaces w
		JOIN workspace_members m ON m.org = w.org AND m.workspace = w.id WHERE m.email = ?
		ORDER BY w.org, CASE WHEN w.id = ? THEN 0 ELSE 1 END, w.created, w.id`, email, team.First)
	if err != nil {
		return nil, err
	}
	by := map[string][]Team{}
	for _, row := range rows {
		by[row.org] = append(by[row.org], row.space)
	}
	return by, nil
}

// MakeTeam makes a team in the organization, from an admin, who is its first member.
func (s *Store) MakeTeam(ctx context.Context, slug string, by Person, name string) (Team, error) {
	name, err := teamName(name)
	if err != nil {
		return Team{}, err
	}
	if by.Org != slug || by.Role != Admin {
		return Team{}, ErrNotAdmin
	}
	var made Team
	err = s.db.InTx(ctx, func(tx dbkit.Tx) (err error) {
		made, err = s.makeTeamIn(ctx, tx, slug, by.Email, name)
		return err
	})
	if err != nil {
		return Team{}, err
	}
	s.publishRoster(ctx, slug)
	return made, nil
}

func (s *Store) makeTeamIn(ctx context.Context, tx dbkit.Tx, slug, by, name string) (Team, error) {
	if err := s.lock(ctx, tx, "org:"+slug); err != nil {
		return Team{}, err
	}
	n, err := count(ctx, tx, `SELECT count(*) FROM workspaces WHERE org = ?`, slug)
	if err != nil {
		return Team{}, err
	}
	if n >= team.MaxPerOrg {
		return Team{}, ErrManyTeams
	}
	now := s.now().UTC().Truncate(time.Second)
	base := team.IDFor(name)
	for try := 1; try <= team.MaxPerOrg+1; try++ {
		id := base
		if try > 1 {
			suffix := "-" + strconv.Itoa(try)
			id = strings.TrimRight(base[:min(len(base), 32-len(suffix))], "-") + suffix
		}
		err := s.putTeam(ctx, tx, slug, Team{ID: id, Name: name, Created: now}, by)
		if errors.Is(err, errTeamTaken) {
			continue
		}
		return Team{ID: id, Name: name, Created: now, Members: []string{by}}, err
	}
	return Team{}, ErrManyTeams
}

// putTeam makes a team with one member, or says its id is taken.
func (s *Store) putTeam(ctx context.Context, tx dbkit.Tx, slug string, w Team, member string) error {
	ok, err := inserted(tx.ExecContext(ctx, `INSERT INTO workspaces (org, id, name, created) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		slug, w.ID, w.Name, s.stamp(w.Created)))
	if err != nil {
		return err
	}
	if !ok {
		return errTeamTaken
	}
	return s.putInTeam(ctx, tx, slug, w.ID, member)
}

func (s *Store) putInTeam(ctx context.Context, tx dbkit.Tx, slug, id, email string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO workspace_members (org, workspace, email, created) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		slug, id, email, s.stamp(s.now().UTC().Truncate(time.Second)))
	return err
}

// RenameTeam gives a team another name. Its id, which its records are kept under, stays.
func (s *Store) RenameTeam(ctx context.Context, slug string, by Person, id, name string) error {
	name, err := teamName(name)
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
			err = ErrNoTeam
		}
		return err
	}
	s.publishRoster(ctx, slug)
	return nil
}

// AddToTeam puts a person of the organization in one of its teams, from an admin.
func (s *Store) AddToTeam(ctx context.Context, slug string, by Person, id, address string) error {
	email, err := NormalEmail(address)
	if err != nil {
		return ErrNoMember
	}
	if by.Org != slug || by.Role != Admin {
		return ErrNotAdmin
	}
	err = s.db.InTx(ctx, func(tx dbkit.Tx) error { return s.addToTeamIn(ctx, tx, slug, id, email) })
	if err != nil {
		return err
	}
	s.publishRoster(ctx, slug)
	return nil
}

// RemoveFromTeam takes a person out of one team, from an admin, and marks their computer in it for removal.
// Their computers in other teams stay. A person stays in at least one team while they are in the organization.
func (s *Store) RemoveFromTeam(ctx context.Context, slug string, by Person, id, address string) error {
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
		return removeFromTeamIn(ctx, tx, slug, id, email)
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

func (s *Store) addToTeamIn(ctx context.Context, tx dbkit.Tx, slug, id, email string) error {
	if err := s.lock(ctx, tx, "org:"+slug); err != nil {
		return err
	}
	if err := mustBeTeam(ctx, tx, slug, id); err != nil {
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
		err = ErrInTeam
	}
	return err
}

func removeFromTeamIn(ctx context.Context, tx dbkit.Tx, slug, id, email string) error {
	here, err := count(ctx, tx, `SELECT count(*) FROM workspace_members WHERE org = ? AND workspace = ? AND email = ?`, slug, id, email)
	if err != nil {
		return err
	}
	if here == 0 {
		return ErrNotInTeam
	}
	in, err := count(ctx, tx, `SELECT count(*) FROM workspace_members WHERE org = ? AND email = ?`, slug, email)
	if err != nil {
		return err
	}
	if in <= 1 {
		return ErrLastTeam
	}
	_, err = tx.ExecContext(ctx, `DELETE FROM workspace_members WHERE org = ? AND workspace = ? AND email = ?`, slug, id, email)
	return err
}

func mustBeTeam(ctx context.Context, q dbkit.Querier, slug, id string) error {
	n, err := count(ctx, q, `SELECT count(*) FROM workspaces WHERE org = ? AND id = ?`, slug, id)
	if err == nil && n == 0 {
		err = ErrNoTeam
	}
	return err
}

// joinTeam is the team a person who accepts an invitation joins: the one it names while it is there, and the
// first otherwise.
func joinTeam(ctx context.Context, q dbkit.Querier, slug, id string) (string, error) {
	if id == "" || id == team.First {
		return team.First, nil
	}
	if err := mustBeTeam(ctx, q, slug, id); err != nil {
		if errors.Is(err, ErrNoTeam) {
			return team.First, nil
		}
		return "", err
	}
	return id, nil
}

// fillFirstTeams runs firstTeamRows.
func fillFirstTeams(ctx context.Context, tx dbkit.Tx) error {
	for _, statement := range firstTeamRows {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func teamName(name string) (string, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" || len([]rune(name)) > 60 || strings.ContainsFunc(name, unicode.IsControl) {
		return "", ErrTeamName
	}
	return name, nil
}
