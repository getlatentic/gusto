package cloud

import (
	"context"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
)

// The sizes the scale test seeds: a hosted gusto with a great many accounts.
const (
	scalePeople = 50_000
	scaleOrgs   = 5_000
	scaleLeads  = 20_000
)

// TestTheStoreAtScale seeds a large database and prints how long the operations a front door does most take. It runs
// only when GUSTO_CLOUD_SCALE=1 is set, and prints with -v.
func TestTheStoreAtScale(t *testing.T) {
	if os.Getenv("GUSTO_CLOUD_SCALE") != "1" {
		t.Skip("set GUSTO_CLOUD_SCALE=1 to measure")
	}
	onEachDatabase(t, func(t *testing.T) {
		set := Settings{PerDomain: 100000, Demo: DemoSettings{Length: time.Hour, MaxOrgs: 50, PerDay: 100000, PerPerson: 5, PerCompany: 100000}}
		s := open(t, set)
		seed(t, s)
		measure(t, "sign-in lookup (Person by address)", 2000, func(i int) error {
			_, found, err := s.Person(fmt.Sprintf("u%d@c%d.com", (i*37)%scalePeople, ((i*37)%scalePeople)%scaleOrgs))
			if err == nil && !found {
				err = fmt.Errorf("not found")
			}
			return err
		})
		measure(t, "sign-up (person, organization, directory)", 300, func(i int) error {
			_, _, err := s.SignUp(Signup{Email: fmt.Sprintf("new%d@fresh%d.com", i, i), OrgName: fmt.Sprintf("Fresh %d", i)})
			return err
		})
		measureInvitations(t, s)
		measure(t, "list every lead (20,000)", 5, func(int) error {
			leads, err := s.Leads()
			if err == nil && len(leads) != scaleLeads {
				err = fmt.Errorf("%d leads", len(leads))
			}
			return err
		})
		measure(t, "demo-limit check (address, company, day, host)", 1000, func(i int) error {
			return s.demoAllowed(context.Background(), s.db, fmt.Sprintf("visitor%d@lead%d.com", i, i))
		})
		measure(t, "start a demo (all of its checks and writes)", 45, func(i int) error {
			_, _, err := s.StartDemo("Visitor", fmt.Sprintf("v%d@visit%d.com", i, i), "Visitors")
			return err
		})
	})
}

func measureInvitations(t *testing.T, s *Store) {
	const n = 200
	secrets := make([]string, n)
	for i := range n {
		admin, org, err := s.SignUp(Signup{Email: fmt.Sprintf("boss%d@inv%d.com", i, i), OrgName: fmt.Sprintf("Inviting %d", i)})
		if err != nil {
			t.Fatal(err)
		}
		if admin, err = s.Verify(admin.Email); err != nil {
			t.Fatal(err)
		}
		if secrets[i], err = s.Invite(org.Slug, admin, fmt.Sprintf("guest%d@inv%d.com", i, i)); err != nil {
			t.Fatal(err)
		}
	}
	measure(t, "accept an invitation", n, func(i int) error {
		_, _, err := s.Accept(secrets[i])
		return err
	})
}

// measure runs fn n times and prints the mean and the 99th percentile.
func measure(t *testing.T, name string, n int, fn func(i int) error) {
	t.Helper()
	took := make([]time.Duration, n)
	for i := range n {
		start := time.Now()
		if err := fn(i); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		took[i] = time.Since(start)
	}
	var total time.Duration
	for _, d := range took {
		total += d
	}
	slices.Sort(took)
	t.Logf("%-52s mean %9s  p99 %9s  (%d runs)", name, (total / time.Duration(n)).Round(time.Microsecond), took[n*99/100].Round(time.Microsecond), n)
}

// seed fills the database in a few transactions, as an import of a large old layout would.
func seed(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	start := time.Now()
	now := s.now().UTC().Truncate(time.Second)
	err := s.db.InTx(ctx, func(tx dbkit.Tx) error {
		orgs := make([][]any, scaleOrgs)
		for i := range orgs {
			orgs[i] = s.orgArgs(Org{Slug: fmt.Sprintf("org-%d", i), Name: fmt.Sprintf("Org %d", i), Admin: fmt.Sprintf("u%d@c%d.com", i, i), Domain: fmt.Sprintf("c%d.com", i), Created: now})
		}
		if _, err := insertRows(ctx, tx, insertOrg, orgs); err != nil {
			return err
		}
		people := make([][]any, scalePeople)
		for i := range people {
			org := i % scaleOrgs
			people[i] = s.personArgs(Person{Email: fmt.Sprintf("u%d@c%d.com", i, org), Org: fmt.Sprintf("org-%d", org), Role: Runner, Created: now.Add(time.Duration(i) * time.Second), Verified: now})
		}
		if _, err := insertRows(ctx, tx, insertPerson, people); err != nil {
			return err
		}
		leads := make([][]any, scaleLeads)
		for i := range leads {
			leads[i] = s.leadArgs(Lead{Slug: fmt.Sprintf("demo-%d", i), Name: "Visitor", Email: fmt.Sprintf("lead%d@lead%d.com", i, i%4000), Company: "Co", Requested: now.Add(-time.Duration(i) * time.Minute)})
		}
		_, err := insertRows(ctx, tx, insertLead, leads)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("seeded %d people, %d organizations and %d leads in %s", scalePeople, scaleOrgs, scaleLeads, time.Since(start).Round(time.Millisecond))
}
