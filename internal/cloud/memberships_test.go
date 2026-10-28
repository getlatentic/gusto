package cloud

import (
	"context"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
	"github.com/getlatentic/gusto/internal/roster"
)

func rosterEmails(t *testing.T, s *Store, slug string) []string {
	t.Helper()
	list, err := roster.Read(s.RosterFile(slug))
	if err != nil {
		t.Fatal(err)
	}
	var emails []string
	for _, member := range list.Members {
		emails = append(emails, member.Email+":"+member.Role)
	}
	return emails
}

func TestAPersonInvitedFromAnotherOrganizationIsInBothWithARoleInEach(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		ada, acme := verifiedOrg(t, s, "ada@acme.com", "Acme")
		_, globex := verifiedOrg(t, s, "eve@globex.com", "Globex")
		joined(t, s, acme, ada, "Eve@Globex.com")
		all, err := s.Memberships("eve@globex.com")
		if err != nil || len(all) != 2 {
			t.Fatalf("%+v %v", all, err)
		}
		roles := map[string]Role{}
		for _, each := range all {
			roles[each.Org] = each.Role
			if each.Trial() {
				t.Errorf("one sign-in is verified in every organization: %+v", each)
			}
		}
		if roles[globex.Slug] != Admin || roles[acme.Slug] != Runner {
			t.Errorf("%v", roles)
		}
		if got := strings.Join(rosterEmails(t, s, acme.Slug), " "); got != "ada@acme.com:admin eve@globex.com:runner" {
			t.Errorf("acme's server sees its own members: %s", got)
		}
		s.PublishRoster(globex.Slug)
		if got := strings.Join(rosterEmails(t, s, globex.Slug), " "); got != "eve@globex.com:admin" {
			t.Errorf("globex's server sees only its own: %s", got)
		}
		orgs, err := s.OrgsOf("eve@globex.com")
		if err != nil || len(orgs) != 2 || orgs[0].Name != "Acme" || orgs[1].Name != "Globex" {
			t.Errorf("%+v %v", orgs, err)
		}
	})
}

func TestRemovingAPersonFromOneOrganizationLeavesTheirOthersAndTheirSignIn(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		ada, acme := verifiedOrg(t, s, "ada@acme.com", "Acme")
		_, globex := verifiedOrg(t, s, "eve@globex.com", "Globex")
		joined(t, s, acme, ada, "eve@globex.com")
		joined(t, s, acme, ada, "bob@acme.com")
		if err := s.Remove(acme.Slug, ada, "eve@globex.com"); err != nil {
			t.Fatal(err)
		}
		all, _ := s.Memberships("eve@globex.com")
		if len(all) != 1 || all[0].Org != globex.Slug || all[0].Role != Admin {
			t.Errorf("%+v", all)
		}
		if found, _ := s.HasAccount("eve@globex.com"); !found {
			t.Error("the sign-in went with one organization of two")
		}
		if err := s.Remove(acme.Slug, ada, "bob@acme.com"); err != nil {
			t.Fatal(err)
		}
		if found, _ := s.HasAccount("bob@acme.com"); found {
			t.Error("a person in no organization still has a sign-in")
		}
		if got := strings.Join(rosterEmails(t, s, acme.Slug), " "); got != "ada@acme.com:admin" {
			t.Errorf("%s", got)
		}
	})
}

func TestARoleIsChangedInOneOrganizationOnly(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		ada, acme := verifiedOrg(t, s, "ada@acme.com", "Acme")
		_, globex := verifiedOrg(t, s, "eve@globex.com", "Globex")
		joined(t, s, acme, ada, "eve@globex.com")
		if _, err := s.SetRole(acme.Slug, ada, "eve@globex.com", Admin); err != nil {
			t.Fatal(err)
		}
		inGlobex, _, _ := s.Person("eve@globex.com", globex.Slug)
		inAcme, _, _ := s.Person("eve@globex.com", acme.Slug)
		if inAcme.Role != Admin || inGlobex.Role != Admin {
			t.Fatalf("%+v %+v", inAcme, inGlobex)
		}
		if _, err := s.SetRole(acme.Slug, ada, "eve@globex.com", Runner); err != nil {
			t.Fatal(err)
		}
		if again, _, _ := s.Person("eve@globex.com", globex.Slug); again.Role != Admin {
			t.Errorf("a role in one organization changed another's: %+v", again)
		}
		// An admin of one organization is nobody in another.
		eve, _, _ := s.Person("eve@globex.com", globex.Slug)
		if _, err := s.SetRole(acme.Slug, eve, "ada@acme.com", Runner); err != ErrNotAdmin {
			t.Errorf("%v", err)
		}
	})
}

func TestEndingADemoKeepsItsPeoplesOtherOrganizations(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s, _ := clocked(t, demoSet)
		_, globex := verifiedOrg(t, s, "eve@globex.com", "Globex")
		ada, demo, err := s.StartDemo("Ada", "ada@acme.com", "Acme")
		if err != nil {
			t.Fatal(err)
		}
		joined(t, s, demo, ada, "eve@globex.com")
		if err := s.EndDemo(demo.Slug, "its time was up"); err != nil {
			t.Fatal(err)
		}
		if found, _ := s.HasAccount("ada@acme.com"); found {
			t.Error("the person who was only in the demo is still there")
		}
		all, _ := s.Memberships("eve@globex.com")
		if len(all) != 1 || all[0].Org != globex.Slug {
			t.Errorf("%+v", all)
		}
	})
}

// firstMigration is the cloud schema as it was before memberships.
func firstMigration(t *testing.T) dbkit.Migrations {
	t.Helper()
	only := fstest.MapFS{}
	for _, dialect := range []string{"sqlite", "postgres"} {
		name := filepath.ToSlash(filepath.Join("migrations", dialect, "0001_cloud.sql"))
		text, err := fs.ReadFile(migrationFiles, name)
		if err != nil {
			t.Fatal(err)
		}
		only[name] = &fstest.MapFile{Data: text}
	}
	return dbkit.Migrations{Scope: migrations.Scope, Table: migrations.Table, FS: only, Dir: migrations.Dir}
}

func TestEachPersonOfAnEarlierSchemaBecomesOneMembership(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		ctx := context.Background()
		db := freshDatabase(t)
		if err := dbkit.Migrate(ctx, db, firstMigration(t)); err != nil {
			t.Fatal(err)
		}
		at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
		for _, statement := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO orgs (slug, name, admin, created) VALUES (?, ?, ?, ?)`, []any{"acme-com", "Acme", "ada@acme.com", db.TimeArg(at)}},
			{`INSERT INTO people (email, org, role, created, verified, invited_by, last_sign_in) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				[]any{"ada@acme.com", "acme-com", "admin", db.TimeArg(at), db.TimeArg(at), "", db.TimeArg(at.Add(time.Hour))}},
			{`INSERT INTO people (email, org, role, created, verified, invited_by, last_sign_in) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				[]any{"bob@acme.com", "acme-com", "runner", db.TimeArg(at.Add(time.Minute)), nil, "ada@acme.com", nil}},
		} {
			if _, err := db.ExecContext(ctx, statement.sql, statement.args...); err != nil {
				t.Fatal(err)
			}
		}
		s, err := openOn(db, filepath.Join(t.TempDir(), "cloud"), Settings{})
		if err != nil {
			t.Fatal(err)
		}
		ada, found, err := s.Person("ada@acme.com", "acme-com")
		if err != nil || !found || ada.Role != Admin || ada.Trial() || !ada.LastSignIn.Equal(at.Add(time.Hour)) {
			t.Errorf("%+v %v %v", ada, found, err)
		}
		bob, _, _ := s.Person("bob@acme.com", "acme-com")
		if bob.Role != Runner || bob.By != "ada@acme.com" || !bob.Trial() || !bob.Created.Equal(at.Add(time.Minute)) {
			t.Errorf("%+v", bob)
		}
		if n, _ := count(ctx, db, `SELECT count(*) FROM memberships`); n != 2 {
			t.Errorf("%d memberships", n)
		}
	})
}

// signInByLink signs a fresh browser in the way a returning person does, with the link mailed to them.
func (f *front) signInByLink(email string) *browser {
	f.t.Helper()
	b := f.browser()
	b.post("/signin", url.Values{"email": {email}})
	if status, _, _ := b.post("/signin/link", url.Values{"t": {tokenOf(f.linkTo(email, "/signin/link?t="))}}); status != http.StatusSeeOther {
		f.t.Fatalf("signing in as %s: %d", email, status)
	}
	return b
}

func TestAPersonInSeveralOrganizationsChoosesOneAndOpensOnlyTheirOwn(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := withServers(t)
		ada := f.verifiedAdmin("ada@acme.com", "Acme")
		f.verifiedAdmin("eve@globex.com", "Globex")
		f.verifiedAdmin("ned@initech.com", "Initech")
		f.invitedAndJoined(ada, "eve@globex.com")

		eve := f.signInByLink("eve@globex.com")
		if status, _, header := eve.get2("/org"); status != http.StatusSeeOther || header.Get("Location") != "/orgs" {
			t.Fatalf("a person in two organizations is asked which: %d %v", status, header)
		}
		_, page := eve.get("/orgs")
		if !strings.Contains(page, `href="/org?org=acme-com"`) || !strings.Contains(page, `href="/org?org=globex-com"`) || strings.Contains(page, "initech") {
			t.Fatalf("the choice is their own organizations:\n%s", page)
		}
		if _, page := eve.get("/org?org=globex-com"); !strings.Contains(page, ">Globex</h1>") || !strings.Contains(page, `href="/orgs"`) {
			t.Fatalf("choosing Globex shows Globex, with a way back to the choice:\n%s", page)
		}
		if _, page := eve.get("/org"); !strings.Contains(page, ">Globex</h1>") {
			t.Errorf("the choice is remembered:\n%s", page)
		}
		if _, page := eve.get("/org?org=initech-com"); !strings.Contains(page, ">Globex</h1>") {
			t.Errorf("an organization they are not in is not chosen:\n%s", page)
		}
		entered := func(org string) (string, string) {
			status, _, header := eve.get2("/open?org=" + org)
			location, _ := url.Parse(header.Get("Location"))
			if status != http.StatusSeeOther || location == nil {
				t.Fatalf("%d %v", status, header)
			}
			_, _, answer := eve.at(location.Host, "GET", location.RequestURI(), nil)
			return location.Host, strings.Join(answer.Values("Set-Cookie"), ";")
		}
		if host, cookies := entered("acme-com"); host != "acme-com.cloud.test" || !strings.Contains(cookies, "gusto=eve@globex.com|runner") {
			t.Errorf("in Acme they are a runner: %s %s", host, cookies)
		}
		if host, cookies := entered("globex-com"); host != "globex-com.cloud.test" || !strings.Contains(cookies, "gusto=eve@globex.com|admin") {
			t.Errorf("in Globex they are its admin: %s %s", host, cookies)
		}
		if host, _ := entered("initech-com"); host != "globex-com.cloud.test" {
			t.Errorf("another organization's name opens the one they are in, not it: %s", host)
		}
		if _, page := ada.get("/org"); strings.Contains(page, `href="/orgs"`) {
			t.Errorf("a person in one organization is offered no switch:\n%s", page)
		}
	})
}
