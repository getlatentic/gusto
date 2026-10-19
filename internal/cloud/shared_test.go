package cloud

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit/dbkittest"
)

// eachShared runs a test against settings that name a database several front doors can open: a SQLite file
// under their root, and, when GUSTO_TEST_POSTGRES_URL is set, a fresh Postgres schema.
func eachShared(t *testing.T, body func(t *testing.T, set Settings)) {
	t.Helper()
	t.Run("sqlite", func(t *testing.T) { body(t, Settings{}) })
	if os.Getenv(dbkittest.PostgresEnv) == "" {
		return
	}
	t.Run("postgres", func(t *testing.T) { body(t, Settings{DatabaseURL: dbkittest.PostgresURL(t)}) })
}

// doors opens n front doors on one database and one root, as n processes on a shared disk would.
func doors(t *testing.T, set Settings, n int) []*Store {
	t.Helper()
	root := filepath.Join(t.TempDir(), "cloud")
	stores := make([]*Store, n)
	for i := range stores {
		s, err := Open(root, set)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		stores[i] = s
	}
	return stores
}

// race runs fn in 2*len(stores) goroutines at once, spread over the front doors, and waits for all.
func race(stores []*Store, rounds int, fn func(s *Store, i int)) {
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range rounds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			fn(stores[i%len(stores)], i)
		}()
	}
	close(start)
	wg.Wait()
}

func TestTwoFrontDoorsCannotBothAcceptOneInvitation(t *testing.T) {
	eachShared(t, func(t *testing.T, set Settings) {
		ds := doors(t, set, 2)
		admin, org := verifiedOrg(t, ds[0], "ada@acme.com", "Acme")
		secret, err := ds[0].Invite(org.Slug, admin, "bob@acme.com")
		if err != nil {
			t.Fatal(err)
		}
		var won atomic.Int32
		race(ds, 24, func(s *Store, _ int) {
			if _, _, err := s.Accept(secret); err == nil {
				won.Add(1)
			}
		})
		if members, _ := ds[1].Members(org.Slug); won.Load() != 1 || len(members) != 2 {
			t.Errorf("%d accepted one invitation and %d are in", won.Load(), len(members))
		}
	})
}

func TestTwoFrontDoorsCannotBothUseOneLink(t *testing.T) {
	eachShared(t, func(t *testing.T, set Settings) {
		ds := doors(t, set, 2)
		for _, purpose := range []Purpose{SignIn, Verify} {
			secret, err := ds[0].MakeLink("ada@acme.com", purpose, time.Hour)
			if err != nil {
				t.Fatal(err)
			}
			var used atomic.Int32
			race(ds, 24, func(s *Store, _ int) {
				if _, err := s.UseLink(secret, purpose); err == nil {
					used.Add(1)
				}
			})
			if used.Load() != 1 {
				t.Errorf("%s: %d requests used one link", purpose, used.Load())
			}
		}
	})
}

func TestTwoFrontDoorsCannotBothTakeOneAddressOrOneSlug(t *testing.T) {
	eachShared(t, func(t *testing.T, set Settings) {
		set.PerDomain = 100
		ds := doors(t, set, 2)
		var bySlug, byEmail atomic.Int32
		var refused atomic.Value
		race(ds, 40, func(s *Store, i int) {
			_, _, err := s.SignUp(Signup{Email: fmt.Sprintf("p%d@company%d.com", i, i), OrgName: "Contested", Slug: "contested"})
			switch {
			case err == nil:
				bySlug.Add(1)
			case !errors.Is(err, ErrSlugTaken):
				refused.Store(err)
			}
		})
		race(ds, 40, func(s *Store, i int) {
			_, _, err := s.SignUp(Signup{Email: "same@acme-two.com", OrgName: fmt.Sprintf("Org %d", i)})
			switch {
			case err == nil:
				byEmail.Add(1)
			case !errors.Is(err, ErrEmailTaken):
				refused.Store(err)
			}
		})
		if err := refused.Load(); err != nil {
			t.Errorf("a loser should be told what was taken: %v", err)
		}
		orgs, _ := ds[0].Orgs()
		people, _ := count(context.Background(), ds[1].db, `SELECT count(*) FROM people`)
		if bySlug.Load() != 1 || byEmail.Load() != 1 || len(orgs) != 2 || people != 2 {
			t.Errorf("%d took the slug, %d took the address, %d organisations and %d people were made", bySlug.Load(), byEmail.Load(), len(orgs), people)
		}
	})
}

func TestFrontDoorsTogetherNeverPassTheNumberOfPeopleAnOrganisationMayHave(t *testing.T) {
	eachShared(t, func(t *testing.T, set Settings) {
		ds := doors(t, set, 2)
		admin, org := verifiedOrg(t, ds[0], "ada@acme.com", "Acme")
		var sent atomic.Int32
		race(ds, 20, func(s *Store, i int) {
			if _, err := s.Invite(org.Slug, admin, fmt.Sprintf("p%d@acme.com", i)); err == nil {
				sent.Add(1)
			}
		})
		if open, _ := ds[1].Invitations(org.Slug); sent.Load() != 3 || len(open) != 3 {
			t.Errorf("%d were sent and %d are open, and an admin and three is all that is allowed", sent.Load(), len(open))
		}
	})
}

func TestFrontDoorsTogetherNeverPassTheOrganisationsOneCompanyMayLeaveUnconfirmed(t *testing.T) {
	eachShared(t, func(t *testing.T, set Settings) {
		ds := doors(t, set, 2)
		var made, full atomic.Int32
		race(ds, 12, func(s *Store, i int) {
			_, _, err := s.SignUp(Signup{Email: fmt.Sprintf("p%d@acme.com", i), OrgName: fmt.Sprintf("Team %d", i)})
			switch {
			case err == nil:
				made.Add(1)
			case errors.Is(err, ErrDomainFull):
				full.Add(1)
			}
		})
		if made.Load() != 3 || full.Load() != 9 {
			t.Errorf("%d organisations were made and %d turned away", made.Load(), full.Load())
		}
	})
}

func TestFrontDoorsTogetherNeverLeaveAnOrganisationWithoutAnAdmin(t *testing.T) {
	eachShared(t, func(t *testing.T, set Settings) {
		ds := doors(t, set, 2)
		for round := range 6 {
			admin, org := verifiedOrg(t, ds[0], fmt.Sprintf("ada%d@acme%d.com", round, round), "Acme "+fmt.Sprint(round))
			bob := joined(t, ds[0], org, admin, fmt.Sprintf("bob%d@acme%d.com", round, round))
			if _, err := ds[0].SetRole(org.Slug, admin, bob.Email, Admin); err != nil {
				t.Fatal(err)
			}
			bob.Role = Admin
			attempts := []func(s *Store) error{
				func(s *Store) error { return s.Remove(org.Slug, admin, bob.Email) },
				func(s *Store) error { return s.Remove(org.Slug, bob, admin.Email) },
				func(s *Store) error { _, err := s.SetRole(org.Slug, admin, bob.Email, Runner); return err },
				func(s *Store) error { _, err := s.SetRole(org.Slug, bob, admin.Email, Runner); return err },
			}
			race(ds, len(attempts), func(s *Store, i int) { attempts[i](s) })
			members, _ := ds[1].Members(org.Slug)
			admins := 0
			for _, member := range members {
				if member.Role == Admin {
					admins++
				}
			}
			if admins < 1 {
				t.Fatalf("round %d: %d people and no admin", round, len(members))
			}
		}
	})
}

func TestFrontDoorsTogetherHoldADemoToItsLimits(t *testing.T) {
	eachShared(t, func(t *testing.T, set Settings) {
		set.Demo = DemoSettings{Length: 30 * time.Minute, MaxOrgs: 100, PerDay: 3, PerPerson: 2, PerCompany: 2}
		ds := doors(t, set, 2)
		started := func(email func(i int) string) int32 {
			var n atomic.Int32
			race(ds, 12, func(s *Store, i int) {
				if _, _, err := s.StartDemo("Visitor", email(i), "Company"); err == nil {
					n.Add(1)
				}
			})
			return n.Load()
		}
		if n := started(func(i int) string { return fmt.Sprintf("p%d@big.com", i) }); n != 2 {
			t.Errorf("%d demos for one company, which may have 2", n)
		}
		if n := started(func(i int) string { return fmt.Sprintf("p%d@other%d.com", i, i) }); n != 1 {
			t.Errorf("%d more demos, when the day allows 3 in all and 2 are taken", n)
		}
		live, _ := count(context.Background(), ds[0].db, `SELECT count(*) FROM orgs WHERE is_demo`)
		if live != 3 {
			t.Errorf("%d demos run", live)
		}
	})
}

func TestFrontDoorsTogetherNeverRunMoreDemosAtOnceThanTheHostAllows(t *testing.T) {
	eachShared(t, func(t *testing.T, set Settings) {
		set.Demo = DemoSettings{Length: 30 * time.Minute, MaxOrgs: 2}
		ds := doors(t, set, 2)
		var n atomic.Int32
		race(ds, 16, func(s *Store, i int) {
			if _, _, err := s.StartDemo("Visitor", fmt.Sprintf("p%d@one%d.com", i, i), "Company"); err == nil {
				n.Add(1)
			}
		})
		if n.Load() != 2 {
			t.Errorf("%d demos started, and the host allows 2 at once", n.Load())
		}
	})
}

func TestARateLimitIsOneCountForEveryFrontDoorOnTheDatabase(t *testing.T) {
	eachShared(t, func(t *testing.T, set Settings) {
		ds := doors(t, set, 2)
		limiters := []*Limiter{ds[0].Limiter("invites", 5, time.Hour), ds[1].Limiter("invites", 5, time.Hour)}
		var allowed atomic.Int32
		race(ds, 30, func(_ *Store, i int) {
			if limiters[i%2].Allow("acme-com") {
				allowed.Add(1)
			}
		})
		if allowed.Load() != 5 {
			t.Errorf("%d events went ahead, and the limit is 5", allowed.Load())
		}
		if !limiters[0].Allow("globex-com") {
			t.Error("another key has its own count")
		}
		now := time.Now().Add(2 * time.Hour)
		ds[0].now, ds[1].now = func() time.Time { return now }, func() time.Time { return now }
		if !limiters[1].Allow("acme-com") {
			t.Error("the window slides")
		}
	})
}

func TestWhatAnOperatorSetsOnOneFrontDoorHoldsOnTheOthers(t *testing.T) {
	eachShared(t, func(t *testing.T, set Settings) {
		set.Demo = DemoSettings{Length: time.Minute, MaxOrgs: 5}
		ds := doors(t, set, 2)
		ds[0].SetDemosPaused(true)
		if !ds[1].DemosPaused() {
			t.Error("a pause on one front door is not seen by the other")
		}
		if err := ds[0].SetDemoLimits(DemoLimits{PerDay: 7, MaxOrgs: 2}); err != nil {
			t.Fatal(err)
		}
		if got := ds[1].DemoLimits(); got.PerDay != 7 || got.MaxOrgs != 2 || !ds[1].DemoLimitsSet() {
			t.Errorf("%+v", got)
		}
		ds[1].ResetDemoLimits()
		if ds[0].DemoLimitsSet() {
			t.Error("a reset on one front door is not seen by the other")
		}
	})
}

func TestEveryFrontDoorOnADatabaseSignsAndChecksTheSameCookies(t *testing.T) {
	eachShared(t, func(t *testing.T, set Settings) {
		ds := doors(t, set, 4)
		sessions := make([]*Sessions, len(ds))
		race(ds, len(ds), func(s *Store, i int) {
			var err error
			if sessions[i], err = s.OpenSessions(false); err != nil {
				t.Error(err)
			}
		})
		cookie := cookieFor(t, sessions[0], "ada@acme.com")
		for i, s := range sessions {
			if email, ok := whoAmI(s, cookie); !ok || email != "ada@acme.com" {
				t.Errorf("door %d: %q %v", i, email, ok)
			}
		}
	})
}

func TestTwoOrganisationsOnOneCellAreNeverGivenOnePort(t *testing.T) {
	eachShared(t, func(t *testing.T, set Settings) {
		set.PerDomain = 100
		ds := doors(t, set, 2)
		var slugs []string
		for i := range 8 {
			_, org, err := ds[0].SignUp(Signup{Email: fmt.Sprintf("p%d@c%d.com", i, i), OrgName: fmt.Sprintf("Org %d", i)})
			if err != nil {
				t.Fatal(err)
			}
			slugs = append(slugs, org.Slug)
		}
		var given atomic.Int32
		race(ds, len(slugs), func(s *Store, i int) {
			if s.SetRuntime(slugs[i], Runtime{State: Starting, Cell: "cell-a", Port: 9100, ProxyPort: 9101}) == nil {
				given.Add(1)
			}
		})
		if given.Load() != 1 {
			t.Errorf("%d organisations were given the same two ports on one cell", given.Load())
		}
	})
}
