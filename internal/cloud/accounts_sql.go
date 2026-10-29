package cloud

import (
	"context"
	"os"

	"github.com/getlatentic/gusto/internal/dbkit"
	"github.com/getlatentic/gusto/internal/quota"
)

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

// orgsAt is how many organizations a company's domain has, and how many of them
// have an admin who has not yet confirmed their address. It is the second that
// is limited: a company may have as many organizations as it has real people to
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
// makes the organization's directory and quota. Nothing is kept unless the transaction commits.
func (s *Store) create(ctx context.Context, tx dbkit.Tx, person Person, org Org, q quota.Quota) error {
	if ok, err := inserted(tx.ExecContext(ctx, insertOrg.SQL(1), s.orgArgs(org)...)); err != nil || !ok {
		if err == nil {
			err = ErrSlugTaken
		}
		return err
	}
	if ok, err := inserted(tx.ExecContext(ctx, insertAccount.SQL(1), s.accountArgs(person)...)); err != nil || !ok {
		if err == nil {
			err = ErrEmailTaken
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, insertMembership.SQL(1), s.membershipArgs(person)...); err != nil {
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

// Org is an organization by its address.
func (s *Store) Org(slug string) (Org, bool, error) {
	if ValidSlug(slug) != nil {
		return Org{}, false, nil
	}
	return fetchOrg(context.Background(), s.db, slug)
}

// Orgs is every organization there is.
func (s *Store) Orgs() ([]Org, error) {
	return collect(context.Background(), s.db, scanOrg, `SELECT `+orgColumns+` FROM orgs ORDER BY slug`)
}
