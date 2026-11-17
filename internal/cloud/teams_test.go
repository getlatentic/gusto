package cloud

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/dbkit"
	"github.com/getlatentic/gusto/internal/roster"
)

func spaceIn(list roster.Roster, id string) (roster.Team, bool) {
	at := slices.IndexFunc(list.Teams, func(w roster.Team) bool { return w.ID == id })
	if at < 0 {
		return roster.Team{}, false
	}
	return list.Teams[at], true
}

func TestANewOrganizationHasOneTeamWithItsAdminAndAPersonInvitedJoinsIt(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f, ada := acme(t)
		list := f.roster()
		if len(list.Teams) != 1 || list.Teams[0].ID != "main" || list.Teams[0].Name != "Acme" || !slices.Equal(list.Teams[0].Members, []string{"ada@acme.com"}) {
			t.Fatalf("a new organization's teams: %+v", list.Teams)
		}
		if got := shownAt(t, ada, "/org"); !got.IsAdmin || len(got.Teams) != 1 || got.Teams[0].ID != "main" {
			t.Fatalf("an admin sees the teams, and with one, the invitation does not ask which: %+v", got.Teams)
		}
		f.invitedAndJoined(ada, "bob@acme.com")
		if main, _ := spaceIn(f.roster(), "main"); !slices.Equal(main.Members, []string{"ada@acme.com", "bob@acme.com"}) {
			t.Fatalf("a person who joins is in the first team: %+v", main)
		}
	})
}

func TestAnAdminMakesATeamAndInvitesAPersonStraightIntoIt(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f, ada := acme(t)
		if status, _, _ := ada.post("/org/teams", url.Values{"name": {"  Business  team "}}); status != http.StatusSeeOther {
			t.Fatalf("making a team: %d", status)
		}
		business, found := spaceIn(f.roster(), "business-team")
		if !found || business.Name != "Business team" || !slices.Equal(business.Members, []string{"ada@acme.com"}) {
			t.Fatalf("the new team, with its maker in it: %+v", f.roster().Teams)
		}
		if status, _, _ := ada.post("/org/teams", url.Values{"name": {"Business team"}}); status != http.StatusSeeOther {
			t.Fatalf("a second team by the same name: %d", status)
		}
		if _, found := spaceIn(f.roster(), "business-team-2"); !found {
			t.Fatalf("a second team by one name gets an id of its own: %+v", f.roster().Teams)
		}
		if status, _, _ := ada.post("/org/invite", url.Values{"email": {"sam@acme.com"}, "team": {"business-team"}}); status != http.StatusSeeOther {
			t.Fatalf("inviting into a team: %d", status)
		}
		sam := f.browser()
		if status, _, _ := sam.post("/invite", url.Values{"t": {tokenOf(f.linkTo("sam@acme.com", "/invite?t="))}}); status != http.StatusSeeOther {
			t.Fatalf("joining: %d", status)
		}
		list := f.roster()
		if spaces := list.SpacesOf("sam@acme.com"); len(spaces) != 1 || spaces[0].ID != "business-team" {
			t.Fatalf("sam joins only the team he was invited to: %+v", spaces)
		}
		if status, _, _ := ada.post("/org/teams/rename", url.Values{"team": {"business-team"}, "name": {"Sales"}}); status != http.StatusSeeOther {
			t.Fatalf("renaming: %d", status)
		}
		if renamed, _ := spaceIn(f.roster(), "business-team"); renamed.Name != "Sales" {
			t.Fatalf("a renamed team keeps its id: %+v", f.roster().Teams)
		}
		kinds := map[string]bool{}
		for _, event := range f.events() {
			kinds[event.Item.Kind] = true
		}
		if !kinds[changelog.TeamMade] || !kinds[changelog.TeamRenamed] {
			t.Fatalf("the organization's record is told of the teams: %v", kinds)
		}
	})
}

func TestARunnerCannotChangeTeamsNorSeeThemWithOnlyOne(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f, ada := acme(t)
		bob := f.invitedAndJoined(ada, "bob@acme.com")
		if got := shownAt(t, bob, "/org"); got.IsAdmin || len(got.Teams) != 1 {
			t.Fatalf("a runner in one team is shown the teams: %+v", got.Teams)
		}
		if status, _, _ := bob.post("/org/teams", url.Values{"name": {"Mine"}}); status != http.StatusForbidden {
			t.Fatalf("a runner made a team: %d", status)
		}
		if len(f.roster().Teams) != 1 {
			t.Fatalf("a runner's refused team was kept: %+v", f.roster().Teams)
		}
	})
}

func TestLeavingATeamRemovesThatComputerOnlyAndAPersonKeepsOneTeam(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f, ada := acme(t)
		f.invitedAndJoined(ada, "bob@acme.com")
		ada.post("/org/teams", url.Values{"name": {"Engineering"}})
		if status, _, _ := ada.post("/org/teams/add", url.Values{"team": {"engineering"}, "email": {"bob@acme.com"}}); status != http.StatusSeeOther {
			t.Fatalf("adding bob: %d", status)
		}
		registry, err := f.store.Computers("acme-com")
		if err != nil {
			t.Fatal(err)
		}
		first, _, _ := registry.Ensure("bob@acme.com", "", "local")
		engineering, _, _ := registry.Ensure("bob@acme.com", "engineering", "local")
		if status, _, _ := ada.post("/org/teams/remove", url.Values{"team": {"engineering"}, "email": {"bob@acme.com"}}); status != http.StatusSeeOther {
			t.Fatalf("taking bob out of engineering: %d", status)
		}
		retiring, _ := registry.Retiring()
		if len(retiring) != 1 || retiring[0].ID != engineering.ID || retiring[0].ID == first.ID {
			t.Fatalf("leaving engineering marks only that computer: %+v", retiring)
		}
		if status, page, _ := ada.post("/org/teams/remove", url.Values{"team": {"main"}, "email": {"bob@acme.com"}}); status != http.StatusConflict || !strings.Contains(shown(t, page).Notice, "at least one team") {
			t.Fatalf("taking bob out of his last team: %d", status)
		}
		if spaces := f.roster().SpacesOf("bob@acme.com"); len(spaces) != 1 || spaces[0].ID != "main" {
			t.Fatalf("bob is still in his last team: %+v", spaces)
		}
	})
}

func TestRemovingAPersonFromTheOrganizationTakesThemOutOfEveryTeam(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f, ada := acme(t)
		f.invitedAndJoined(ada, "bob@acme.com")
		ada.post("/org/teams", url.Values{"name": {"Engineering"}})
		ada.post("/org/teams/add", url.Values{"team": {"engineering"}, "email": {"bob@acme.com"}})
		if status, _, _ := ada.post("/org/people/remove", url.Values{"email": {"bob@acme.com"}}); status != http.StatusSeeOther {
			t.Fatalf("removing bob: %d", status)
		}
		if spaces := f.roster().SpacesOf("bob@acme.com"); len(spaces) != 0 {
			t.Fatalf("a removed person is in no team: %+v", spaces)
		}
	})
}

func TestEveryOrganizationAndPersonWithNoTeamIsGivenTheFirst(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f, ada := acme(t)
		f.invitedAndJoined(ada, "bob@acme.com")
		ctx := context.Background()
		if _, err := f.store.db.ExecContext(ctx, `DELETE FROM workspace_members WHERE org = ?`, "acme-com"); err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.db.ExecContext(ctx, `DELETE FROM workspaces WHERE org = ?`, "acme-com"); err != nil {
			t.Fatal(err)
		}
		if err := f.store.db.InTx(ctx, func(tx dbkit.Tx) error { return fillFirstTeams(ctx, tx) }); err != nil {
			t.Fatal(err)
		}
		spaces, err := f.store.Teams(ctx, "acme-com")
		if err != nil || len(spaces) != 1 || spaces[0].ID != "main" || spaces[0].Name != "Acme" || !slices.Equal(spaces[0].Members, []string{"ada@acme.com", "bob@acme.com"}) {
			t.Fatalf("an organization kept from before teams: %+v, %v", spaces, err)
		}
	})
}
