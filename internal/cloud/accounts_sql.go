package cloud

import (
	"context"
	"io/fs"
	"os"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
	"github.com/getlatentic/gusto/internal/quota"
)

const personColumns = `email, org, role, created, verified, invited_by, last_sign_in`

var personFields = []string{"email", "org", "role", "created", "verified", "invited_by", "last_sign_in"}

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

func fetchPerson(ctx context.Context, q dbkit.Querier, email string) (Person, bool, error) {
	return one(ctx, q, scanPerson, `SELECT `+personColumns+` FROM people WHERE email = ?`, email)
}

func (s *Store) personArgs(p Person) []any {
	return []any{p.Email, p.Org, string(p.Role), s.stamp(p.Created), s.when(p.Verified), p.By, s.when(p.LastSignIn)}
}

var insertPerson = dbkit.Upsert{Table: "people", Columns: personFields, Conflict: []string{"email"}}

const orgColumns = `slug, name, admin, domain, created, is_demo, demo_name, demo_company, demo_ready, demo_expires, demo_extended, demo_stopped`

var orgFields = []string{"slug", "name", "admin", "domain", "created", "is_demo", "demo_name", "demo_company", "demo_ready", "demo_expires", "demo_extended", "demo_stopped"}

func scanOrg(r rowScanner) (Org, error) {
	var o Org
	var created dbkit.Time
	var isDemo, extended, stopped bool
	var demo Demo
	var ready, expires nullTime
	err := r.Scan(&o.Slug, &o.Name, &o.Admin, &o.Domain, &created, &isDemo, &demo.Name, &demo.Company, &ready, &expires, &extended, &stopped)
	if err != nil {
		return Org{}, err
	}
	o.Created = created.Time
	if isDemo {
		demo.Ready, demo.Expires, demo.Extended, demo.Stopped = timeOf(ready), timeOf(expires), extended, stopped
		o.Demo = &demo
	}
	return o, nil
}

func fetchOrg(ctx context.Context, q dbkit.Querier, slug string) (Org, bool, error) {
	return one(ctx, q, scanOrg, `SELECT `+orgColumns+` FROM orgs WHERE slug = ?`, slug)
}

func (s *Store) orgArgs(o Org) []any {
	d := Demo{}
	if o.Demo != nil {
		d = *o.Demo
	}
	return []any{o.Slug, o.Name, o.Admin, o.Domain, s.stamp(o.Created), o.Demo != nil, d.Name, d.Company, s.when(d.Ready), s.when(d.Expires), d.Extended, d.Stopped}
}

var insertOrg = dbkit.Upsert{Table: "orgs", Columns: orgFields, Conflict: []string{"slug"}}

// orgsAt is how many organisations a company's domain has, and how many of them
// have an admin who has not yet confirmed their address. It is the second that
// is limited: a company may have as many organisations as it has real people to
// make them, and not more than a few that nobody has answered for.
func orgsAt(ctx context.Context, q dbkit.Querier, domain string) (existing, pending int, err error) {
	if domain == "" {
		return 0, 0, nil
	}
	err = q.QueryRowContext(ctx, `SELECT count(*), count(CASE WHEN p.email IS NOT NULL AND p.verified IS NULL THEN 1 END)
		FROM orgs o LEFT JOIN people p ON p.email = o.admin WHERE o.domain = ?`, domain).Scan(&existing, &pending)
	return existing, pending, err
}

// create claims the address and the slug in the transaction, which a second caller waits on, and
// makes the organisation's directory and quota. Nothing is kept unless the transaction commits.
func (s *Store) create(ctx context.Context, tx dbkit.Tx, person Person, org Org, q quota.Quota) error {
	if ok, err := inserted(tx.ExecContext(ctx, insertOrg.SQL(1), s.orgArgs(org)...)); err != nil || !ok {
		if err == nil {
			err = ErrSlugTaken
		}
		return err
	}
	if ok, err := inserted(tx.ExecContext(ctx, insertPerson.SQL(1), s.personArgs(person)...)); err != nil || !ok {
		if err == nil {
			err = ErrEmailTaken
		}
		return err
	}
	return s.makeOrgDir(org.Slug, q)
}

func (s *Store) makeOrgDir(slug string, q quota.Quota) error {
	if err := os.MkdirAll(s.DataDir(slug), 0o700); err != nil {
		return err
	}
	if err := s.setQuota(slug, q); err != nil {
		os.RemoveAll(s.OrgDir(slug))
		return err
	}
	return nil
}

// Person is the account at an address.
func (s *Store) Person(email string) (Person, bool, error) {
	email, err := NormalEmail(email)
	if err != nil {
		return Person{}, false, nil
	}
	return fetchPerson(context.Background(), s.db, email)
}

// Org is an organisation by its address.
func (s *Store) Org(slug string) (Org, bool, error) {
	if ValidSlug(slug) != nil {
		return Org{}, false, nil
	}
	return fetchOrg(context.Background(), s.db, slug)
}

// Orgs is every organisation there is.
func (s *Store) Orgs() ([]Org, error) {
	return collect(context.Background(), s.db, scanOrg, `SELECT `+orgColumns+` FROM orgs ORDER BY slug`)
}

// Verify records that a person read the mail sent to them, and lifts their
// organisation's quota when they administer it.
func (s *Store) Verify(email string) (Person, error) {
	ctx := context.Background()
	var person Person
	var changed bool
	err := s.db.InTx(ctx, func(tx dbkit.Tx) error {
		var found bool
		var err error
		if person, found, err = fetchPerson(ctx, tx, email); err != nil || !found {
			if err == nil {
				err = fs.ErrNotExist
			}
			return err
		}
		if changed = person.Verified.IsZero(); !changed {
			return nil
		}
		person.Verified = s.now().UTC().Truncate(time.Second)
		_, err = tx.ExecContext(ctx, `UPDATE people SET verified = ? WHERE email = ?`, s.stamp(person.Verified), email)
		return err
	})
	if err != nil || !changed || person.Role != Admin {
		return person, err
	}
	return person, s.setQuota(person.Org, s.set.Limits.Verified)
}
