package cloud

import (
	"context"
	"io/fs"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
)

// A person is one row in people, which is their address and whether they read their mail, and one row in memberships
// for each organization they are in, which is their role there. A Person is the two read together for one organization.
const personColumns = `m.email, m.org, m.role, m.created, p.verified, m.invited_by, m.last_sign_in`

const personFrom = ` FROM memberships m JOIN people p ON p.email = m.email`

func scanPerson(r rowScanner) (Person, error) {
	var p Person
	var role string
	var created dbkit.Time
	var verified, lastSignIn nullTime
	if err := r.Scan(&p.Email, &p.Org, &role, &created, &verified, &p.By, &lastSignIn); err != nil {
		return Person{}, err
	}
	p.Role, p.Created, p.Verified, p.LastSignIn = Role(role), created.Time, timeOf(verified), timeOf(lastSignIn)
	return p, nil
}

// fetchMember is a person as they are in one organization.
func fetchMember(ctx context.Context, q dbkit.Querier, email, org string) (Person, bool, error) {
	return one(ctx, q, scanPerson, `SELECT `+personColumns+personFrom+` WHERE m.email = ? AND m.org = ?`, email, org)
}

// membershipsOf is a person in each organization they are in, in the order they joined them.
func membershipsOf(ctx context.Context, q dbkit.Querier, email string) ([]Person, error) {
	return collect(ctx, q, scanPerson, `SELECT `+personColumns+personFrom+` WHERE m.email = ? ORDER BY m.created, m.org`, email)
}

// hasAccount says whether an address has signed up, joined or been given a demo.
func hasAccount(ctx context.Context, q dbkit.Querier, email string) (bool, error) {
	n, err := count(ctx, q, `SELECT count(*) FROM people WHERE email = ?`, email)
	return n > 0, err
}

var accountFields = []string{"email", "created", "verified"}

var insertAccount = dbkit.Upsert{Table: "people", Columns: accountFields, Conflict: []string{"email"}}

func (s *Store) accountArgs(p Person) []any {
	return []any{p.Email, s.stamp(p.Created), s.when(p.Verified)}
}

var membershipFields = []string{"email", "org", "role", "created", "invited_by", "last_sign_in"}

var insertMembership = dbkit.Upsert{Table: "memberships", Columns: membershipFields, Conflict: []string{"email", "org"}}

func (s *Store) membershipArgs(p Person) []any {
	return []any{p.Email, p.Org, string(p.Role), s.stamp(p.Created), p.By, s.when(p.LastSignIn)}
}

// addMember puts a person in an organization, making their account when they have none, and says whether they were
// not in it already. A person who joins verified, by following a link mailed to them, is verified from then on.
func (s *Store) addMember(ctx context.Context, tx dbkit.Tx, p Person) (bool, error) {
	if _, err := tx.ExecContext(ctx, insertAccount.SQL(1), s.accountArgs(p)...); err != nil {
		return false, err
	}
	if !p.Verified.IsZero() {
		if _, err := tx.ExecContext(ctx, `UPDATE people SET verified = ? WHERE email = ? AND verified IS NULL`, s.stamp(p.Verified), p.Email); err != nil {
			return false, err
		}
	}
	return inserted(tx.ExecContext(ctx, insertMembership.SQL(1), s.membershipArgs(p)...))
}

// dropMember takes a person out of an organization, and takes their account away when it was the last they were in,
// since an address that belongs nowhere has nothing to sign in to.
func dropMember(ctx context.Context, tx dbkit.Tx, email, org string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM memberships WHERE email = ? AND org = ?`, email, org); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM people WHERE email = ? AND NOT EXISTS (SELECT 1 FROM memberships WHERE email = ?)`, email, email)
	return err
}

// Person is the account at an address as it is in one organization.
func (s *Store) Person(email, org string) (Person, bool, error) {
	email, err := NormalEmail(email)
	if err != nil || ValidSlug(org) != nil {
		return Person{}, false, nil
	}
	return fetchMember(context.Background(), s.db, email, org)
}

// Memberships is the account at an address in each organization it is in, oldest first, and none for an address
// that has no account.
func (s *Store) Memberships(email string) ([]Person, error) {
	email, err := NormalEmail(email)
	if err != nil {
		return nil, nil
	}
	return membershipsOf(context.Background(), s.db, email)
}

// OrgsOf is the organizations an address is in, by name.
func (s *Store) OrgsOf(email string) ([]Org, error) {
	return collect(context.Background(), s.db, scanOrg, `SELECT `+orgColumns+` FROM orgs
		WHERE slug IN (SELECT org FROM memberships WHERE email = ?) ORDER BY name, slug`, email)
}

// HasAccount says whether an address can be signed in.
func (s *Store) HasAccount(email string) (bool, error) {
	email, err := NormalEmail(email)
	if err != nil {
		return false, nil
	}
	return hasAccount(context.Background(), s.db, email)
}

// Verify records that a person read the mail sent to them, and the first time works out again the quota of
// each organization they administer, which takes an organization that does not pay off the trial.
func (s *Store) Verify(email string) error {
	ctx := context.Background()
	var administers []Person
	err := s.db.InTx(ctx, func(tx dbkit.Tx) error {
		at := s.stamp(s.now().UTC().Truncate(time.Second))
		done, err := tx.ExecContext(ctx, `UPDATE people SET verified = ? WHERE email = ? AND verified IS NULL`, at, email)
		if err != nil {
			return err
		}
		if changed, err := done.RowsAffected(); err != nil || changed == 0 {
			if err == nil {
				err = s.mustHaveAccount(ctx, tx, email)
			}
			return err
		}
		all, err := membershipsOf(ctx, tx, email)
		for _, each := range all {
			if each.Role == Admin {
				administers = append(administers, each)
			}
		}
		return err
	})
	if err != nil {
		return err
	}
	for _, each := range administers {
		if err := s.ApplyQuota(each.Org); err != nil {
			return err
		}
	}
	return nil
}

// mustHaveAccount is nothing for an address that has an account, and fs.ErrNotExist for one that has none.
func (s *Store) mustHaveAccount(ctx context.Context, q dbkit.Querier, email string) error {
	found, err := hasAccount(ctx, q, email)
	if err == nil && !found {
		err = fs.ErrNotExist
	}
	return err
}
