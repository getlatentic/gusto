package cloud

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
)

func TestTheOldFilesAreLoadedIntoTheDatabaseAndLeftAsTheyWere(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		root, _ := oldLayout(t)
		before := digests(t, root)
		s, record := importedAt(t, freshDatabase(t), root, fixtureNow.Add(10*time.Minute))

		want := map[string]int{"organisations": 5, "people": 6, "memberships": 6, "invitations": 1, "leads": 4, "links": 3, "runtime": 1, "settings": 3}
		if !maps.Equal(record.Imported, want) {
			t.Errorf("imported %v, want %v", record.Imported, want)
		}
		wantSkipped := map[string]int{"invitations expired": 1, "links expired": 2}
		if !maps.Equal(record.Skipped, wantSkipped) {
			t.Errorf("left out %v, want %v", record.Skipped, wantSkipped)
		}
		if after := digests(t, root); !maps.Equal(after, before) {
			t.Error("the import changed, added or removed a file")
		}

		ada, found, _ := s.only("ada@acme.com")
		if !found || ada.Role != Admin || ada.Verified.IsZero() || ada.LastSignIn.IsZero() || ada.Org != "acme-com" {
			t.Errorf("%+v", ada)
		}
		if bob, _, _ := s.only("bob@acme.com"); bob.Role != Runner || bob.By != "ada@acme.com" {
			t.Errorf("%+v", bob)
		}
		if eve, _, _ := s.only("eve@globex.com"); !eve.Trial() {
			t.Errorf("an address not yet confirmed is still not: %+v", eve)
		}
		acme, _, _ := s.Org("acme-com")
		if acme.Name != "Acme Research" || acme.Domain != "acme.com" || acme.Admin != "ada@acme.com" || acme.Demo != nil {
			t.Errorf("%+v", acme)
		}
		if members, _ := s.Members("acme-com"); len(members) != 2 || members[0].Email != "ada@acme.com" {
			t.Errorf("%+v", members)
		}
		if rt, _ := s.Runtime("acme-com"); rt.State != Ready || rt.Cell != "cell-a" || rt.Port != 9100 || rt.ProxyPort != 9101 || rt.Restarts != 2 {
			t.Errorf("%+v", rt)
		}
		// The quota is the old file's, which was written before an organization had a credit.
		old := DefaultLimits().Verified
		old.CreditUSD = 0
		if q, err := s.Quota("acme-com"); err != nil || q != old {
			t.Errorf("an organization's own directory is found where it was: %+v %v", q, err)
		}
		if _, err := os.Stat(s.DataDir("globex-com")); err != nil {
			t.Error(err)
		}
		if open, _ := s.Invitations("acme-com"); len(open) != 1 || open[0].Email != "cy@acme.com" {
			t.Errorf("only the invitation that has not run out is open: %+v", open)
		}
		if !s.DemosPaused() || !s.DemoLimitsSet() || s.DemoLimits() != (DemoLimits{PerDay: 25, PerPerson: 1, PerCompany: 4, MaxOrgs: 3}) {
			t.Errorf("the owner's choices: paused %v, %+v", s.DemosPaused(), s.DemoLimits())
		}
		key, _ := os.ReadFile(filepath.Join(root, "secret"))
		if got, err := s.sessionKey(); err != nil || string(got) != string(key) {
			t.Errorf("a restart of the front door signs nobody out: %v", err)
		}
	})
}

func TestTheLinksThatWereOutAreStillGoodOnceAndTheOnesThatRanOutAreNot(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		root, secrets := oldLayout(t)
		s, _ := importedAt(t, freshDatabase(t), root, fixtureNow.Add(10*time.Minute))
		if email, err := s.UseLink(secrets["signinLive"], SignIn); err != nil || email != "ada@acme.com" {
			t.Errorf("%q %v", email, err)
		}
		if _, err := s.UseLink(secrets["signinLive"], SignIn); !errors.Is(err, ErrLink) {
			t.Errorf("a link works once: %v", err)
		}
		if _, err := s.UseLink(secrets["signinExpired"], SignIn); !errors.Is(err, ErrLink) {
			t.Errorf("a link that had run out: %v", err)
		}
		if email, err := s.UseLink(secrets["verifyEve"], Verify); err != nil || email != "eve@globex.com" {
			t.Errorf("%q %v", email, err)
		}
		person, org, err := s.Accept(secrets["inviteCy"])
		if err != nil || person.Email != "cy@acme.com" || person.Role != Runner || person.Verified.IsZero() || org.Slug != "acme-com" {
			t.Errorf("%+v %+v %v", person, org, err)
		}
		if _, _, err := s.Accept(secrets["inviteExpired"]); !errors.Is(err, ErrLink) {
			t.Errorf("an invitation that had run out: %v", err)
		}
	})
}

func TestADemosLeadSurvivesTheImportAndTheDemosGoOnWhereTheyWere(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		root, secrets := oldLayout(t)
		s, _ := importedAt(t, freshDatabase(t), root, fixtureNow)
		for slug, who := range map[string]string{secrets["demoReady"]: "Fay Fox", secrets["demoWaiting"]: "Gus Gray", secrets["demoTimeUp"]: "Ivy Ito", secrets["demoEnded"]: "Hal Hill"} {
			if lead, found := s.LeadOf(slug); !found || lead.Name != who {
				t.Errorf("%s: %+v %v", slug, lead, found)
			}
		}
		ended, _ := s.LeadOf(secrets["demoEnded"])
		if ended.Reason != "its time was up" || ended.Ended.IsZero() {
			t.Errorf("a demo that ended keeps why: %+v", ended)
		}
		if _, found, _ := s.Org(secrets["demoEnded"]); found {
			t.Error("a demo that ended has no organization")
		}
		leads, _ := s.Leads()
		if len(leads) != 4 || leads[0].Slug != secrets["demoWaiting"] {
			t.Errorf("newest first: %+v", leads)
		}
		fay, left, found := s.DemoLeft(secrets["demoReady"])
		if !found || !fay.Extended || fay.Ready.IsZero() || left != 20*time.Minute {
			t.Errorf("%+v %v %v", fay, left, found)
		}
		due, _ := s.DemosDue()
		if len(due) != 1 || due[secrets["demoTimeUp"]] != "its time was up" {
			t.Errorf("a demo whose time was up when the files were read is ended by the next sweep: %v", due)
		}
		s.now = func() time.Time { return fixtureNow.Add(11 * time.Minute) }
		if due, _ := s.DemosDue(); due[secrets["demoWaiting"]] != "it was never ready" {
			t.Errorf("%v", due)
		}
		if err := s.EndDemo(secrets["demoTimeUp"], "its time was up"); err != nil {
			t.Fatal(err)
		}
		if lead, _ := s.LeadOf(secrets["demoTimeUp"]); lead.Reason != "its time was up" {
			t.Errorf("%+v", lead)
		}
	})
}

func TestADemoLimitCountsTheLeadsTheOldFilesHeld(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		root, _ := oldLayout(t)
		s, _ := importedAt(t, freshDatabase(t), root, fixtureNow)
		if err := s.SetDemoLimits(DemoLimits{PerDay: 40, PerPerson: 1, PerCompany: 6, MaxOrgs: 5}); err != nil {
			t.Fatal(err)
		}
		s.SetDemosPaused(false)
		if _, _, err := s.StartDemo("Gus Again", "gus+other@hooli.com", "Hooli"); !errors.Is(err, ErrDemoRepeat) {
			t.Errorf("an address that started a demo before the move has had it: %v", err)
		}
	})
}

func TestAHalfWrittenFileIsLeftOutAndTheRestAreLoaded(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		root, _ := oldLayout(t)
		broken := map[string]string{
			"people/ffffffffffffffffffffffffffffffff.json": `{"email": "half@acme.com", "org": "acme`,
			"people/.write-123":                            `{"email": "temp@acme.com"}`,
			"people/nobody.json":                           `{"email": "lost@nowhere.com", "org": "no-such-org", "role": "admin", "created": "2026-09-29T12:00:00Z"}`,
			"tokens/" + digest("x") + ".spent":             `{"email": "x@acme.com", "purpose": "signin", "expires": "2030-01-01T00:00:00Z"}`,
			"tokens/" + digest("y") + ".json":              `{"email": "y@acme.com", "purpose": "sig`,
			"demos/half.json":                              `{"slug": "half", "na`,
			"orgs/half-org/org.json":                       `{"slug": "half-org", "na`,
			"orgs/acme-com/invites/zzzz.json":              ``,
		}
		for name, text := range broken {
			path := filepath.Join(root, name)
			os.MkdirAll(filepath.Dir(path), 0o700)
			if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		s, record := importedAt(t, freshDatabase(t), root, fixtureNow.Add(10*time.Minute))
		if record.Imported["people"] != 6 || record.Imported["leads"] != 4 || record.Imported["links"] != 3 || record.Imported["organisations"] != 5 {
			t.Errorf("the whole files were loaded: %v", record.Imported)
		}
		for _, reason := range []string{"people unreadable", "people orphaned", "links unreadable", "leads unreadable", "organisations unreadable", "invitations unreadable"} {
			if record.Skipped[reason] != 1 {
				t.Errorf("%q: %v", reason, record.Skipped)
			}
		}
		if _, found, _ := s.only("half@acme.com"); found {
			t.Error("half a record was loaded")
		}
		if _, found, _ := s.only("lost@nowhere.com"); found {
			t.Error("a person of no organization was loaded")
		}
	})
}

func TestTheImportRunsOnceAndRunningItAgainChangesNothing(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		root, secrets := oldLayout(t)
		s, first := importedAt(t, freshDatabase(t), root, fixtureNow.Add(10*time.Minute))
		ctx := context.Background()
		var added ImportRecord
		err := s.db.InTx(ctx, func(tx dbkit.Tx) (err error) {
			added, err = s.importFiles(ctx, tx)
			return err
		})
		if err != nil || added.Total() != 0 {
			t.Errorf("the files read a second time add nothing: %v %v", added.Imported, err)
		}
		if _, err := s.UseLink(secrets["signinLive"], SignIn); err != nil {
			t.Fatal(err)
		}
		again, ran, err := s.importOnce(ctx)
		if err != nil || ran || !maps.Equal(again.Imported, first.Imported) {
			t.Errorf("a second import is the record of the first: ran %v %v %v", ran, again, err)
		}
		if _, err := s.UseLink(secrets["signinLive"], SignIn); !errors.Is(err, ErrLink) {
			t.Error("a link that was used came back")
		}
	})
}

func TestTwoFrontDoorsStartedTogetherImportOnce(t *testing.T) {
	eachShared(t, func(t *testing.T, set Settings) {
		root, _ := oldLayout(t)
		const doors = 4
		stores := make([]*Store, doors)
		errs := make(chan error, doors)
		for i := range doors {
			go func() {
				var err error
				stores[i], err = Open(root, set)
				errs <- err
			}()
		}
		for range doors {
			if err := <-errs; err != nil {
				t.Fatal(err)
			}
		}
		orgs, _ := stores[0].Orgs()
		people, _ := count(context.Background(), stores[1].db, `SELECT count(*) FROM people`)
		records, _ := count(context.Background(), stores[2].db, `SELECT count(*) FROM imports`)
		if len(orgs) != 5 || people != 6 || records != 1 {
			t.Errorf("%d organizations, %d people, %d import records", len(orgs), people, records)
		}
		for _, s := range stores {
			s.Close()
		}
	})
}

func TestAttachNeverMigratesOrImports(t *testing.T) {
	root, _ := oldLayout(t)
	if _, err := Attach(root, Settings{}); err == nil {
		t.Fatal("a root nobody has run gusto cloud on has no accounts to look at")
	}
	if _, err := os.Stat(filepath.Join(root, databaseFile)); err == nil {
		t.Error("looking at a store made its database")
	}
	front, err := Open(root, fixtureSettings)
	if err != nil {
		t.Fatal(err)
	}
	defer front.Close()
	look, err := Attach(root, Settings{})
	if err != nil {
		t.Fatal(err)
	}
	defer look.Close()
	if orgs, _ := look.Orgs(); len(orgs) != 5 {
		t.Errorf("%d organizations", len(orgs))
	}
}

func TestTheReportCountsWhatTheDatabaseAndTheOldFilesHold(t *testing.T) {
	root, _ := oldLayout(t)
	ctx := context.Background()
	before, err := Inspect(ctx, root, Settings{})
	if err != nil || before.Version != 0 || before.Latest == 0 || before.Files["people"] != 6 {
		t.Fatalf("%+v %v", before, err)
	}
	if _, err := os.Stat(filepath.Join(root, databaseFile)); err == nil {
		t.Error("asking made the database")
	}
	record, ran, err := ImportNow(ctx, root, fixtureSettings)
	if err != nil || !ran || record.Imported["people"] == 0 {
		t.Fatalf("%+v %v %v", record, ran, err)
	}
	if _, ran, err := ImportNow(ctx, root, fixtureSettings); err != nil || ran {
		t.Errorf("a second import: %v %v", ran, err)
	}
	after, err := Inspect(ctx, root, Settings{})
	if err != nil || after.Version != after.Latest || after.Counts["people"] == 0 || after.Import == nil || after.Counts["organisations"] != 5 {
		t.Errorf("%+v %v", after, err)
	}
}
