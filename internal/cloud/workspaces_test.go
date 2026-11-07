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

func spaceIn(list roster.Roster, id string) (roster.Workspace, bool) {
	at := slices.IndexFunc(list.Workspaces, func(w roster.Workspace) bool { return w.ID == id })
	if at < 0 {
		return roster.Workspace{}, false
	}
	return list.Workspaces[at], true
}

func TestANewOrganizationHasOneWorkspaceWithItsAdminAndAPersonInvitedJoinsIt(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f, ada := acme(t)
		list := f.roster()
		if len(list.Workspaces) != 1 || list.Workspaces[0].ID != "main" || list.Workspaces[0].Name != "Acme" || !slices.Equal(list.Workspaces[0].Members, []string{"ada@acme.com"}) {
			t.Fatalf("a new organization's workspaces: %+v", list.Workspaces)
		}
		if _, page := ada.get("/org"); !strings.Contains(page, `id="workspaces"`) || strings.Contains(page, `name="workspace" aria-label="Workspace they join"`) {
			t.Fatalf("an admin sees the workspaces, and with one, the invitation does not ask which:\n%s", page)
		}
		f.invitedAndJoined(ada, "bob@acme.com")
		if main, _ := spaceIn(f.roster(), "main"); !slices.Equal(main.Members, []string{"ada@acme.com", "bob@acme.com"}) {
			t.Fatalf("a person who joins is in the first workspace: %+v", main)
		}
	})
}

func TestAnAdminMakesAWorkspaceAndInvitesAPersonStraightIntoIt(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f, ada := acme(t)
		if status, _, _ := ada.post("/org/workspaces", url.Values{"name": {"  Business  team "}}); status != http.StatusSeeOther {
			t.Fatalf("making a workspace: %d", status)
		}
		business, found := spaceIn(f.roster(), "business-team")
		if !found || business.Name != "Business team" || !slices.Equal(business.Members, []string{"ada@acme.com"}) {
			t.Fatalf("the new workspace, with its maker in it: %+v", f.roster().Workspaces)
		}
		if status, _, _ := ada.post("/org/workspaces", url.Values{"name": {"Business team"}}); status != http.StatusSeeOther {
			t.Fatalf("a second workspace by the same name: %d", status)
		}
		if _, found := spaceIn(f.roster(), "business-team-2"); !found {
			t.Fatalf("a second workspace by one name gets an id of its own: %+v", f.roster().Workspaces)
		}
		if status, _, _ := ada.post("/org/invite", url.Values{"email": {"sam@acme.com"}, "workspace": {"business-team"}}); status != http.StatusSeeOther {
			t.Fatalf("inviting into a workspace: %d", status)
		}
		sam := f.browser()
		if status, _, _ := sam.post("/invite", url.Values{"t": {tokenOf(f.linkTo("sam@acme.com", "/invite?t="))}}); status != http.StatusSeeOther {
			t.Fatalf("joining: %d", status)
		}
		list := f.roster()
		if spaces := list.SpacesOf("sam@acme.com"); len(spaces) != 1 || spaces[0].ID != "business-team" {
			t.Fatalf("sam joins only the workspace he was invited to: %+v", spaces)
		}
		if status, _, _ := ada.post("/org/workspaces/rename", url.Values{"workspace": {"business-team"}, "name": {"Sales"}}); status != http.StatusSeeOther {
			t.Fatalf("renaming: %d", status)
		}
		if renamed, _ := spaceIn(f.roster(), "business-team"); renamed.Name != "Sales" {
			t.Fatalf("a renamed workspace keeps its id: %+v", f.roster().Workspaces)
		}
		kinds := map[string]bool{}
		for _, event := range f.events() {
			kinds[event.Item.Kind] = true
		}
		if !kinds[changelog.WorkspaceMade] || !kinds[changelog.WorkspaceRenamed] {
			t.Fatalf("the organization's record is told of the workspaces: %v", kinds)
		}
	})
}

func TestARunnerCannotChangeWorkspacesNorSeeThemWithOnlyOne(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f, ada := acme(t)
		bob := f.invitedAndJoined(ada, "bob@acme.com")
		if _, page := bob.get("/org"); strings.Contains(page, `id="workspaces"`) {
			t.Fatal("a runner in one workspace is shown the workspaces")
		}
		if status, _, _ := bob.post("/org/workspaces", url.Values{"name": {"Mine"}}); status != http.StatusForbidden {
			t.Fatalf("a runner made a workspace: %d", status)
		}
		if len(f.roster().Workspaces) != 1 {
			t.Fatalf("a runner's refused workspace was kept: %+v", f.roster().Workspaces)
		}
	})
}

func TestLeavingAWorkspaceRemovesThatComputerOnlyAndAPersonKeepsOneWorkspace(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f, ada := acme(t)
		f.invitedAndJoined(ada, "bob@acme.com")
		ada.post("/org/workspaces", url.Values{"name": {"Engineering"}})
		if status, _, _ := ada.post("/org/workspaces/add", url.Values{"workspace": {"engineering"}, "email": {"bob@acme.com"}}); status != http.StatusSeeOther {
			t.Fatalf("adding bob: %d", status)
		}
		registry, err := f.store.Computers("acme-com")
		if err != nil {
			t.Fatal(err)
		}
		first, _, _ := registry.Ensure("bob@acme.com", "", "local")
		engineering, _, _ := registry.Ensure("bob@acme.com", "engineering", "local")
		if status, _, _ := ada.post("/org/workspaces/remove", url.Values{"workspace": {"engineering"}, "email": {"bob@acme.com"}}); status != http.StatusSeeOther {
			t.Fatalf("taking bob out of engineering: %d", status)
		}
		retiring, _ := registry.Retiring()
		if len(retiring) != 1 || retiring[0].ID != engineering.ID || retiring[0].ID == first.ID {
			t.Fatalf("leaving engineering marks only that computer: %+v", retiring)
		}
		if status, page, _ := ada.post("/org/workspaces/remove", url.Values{"workspace": {"main"}, "email": {"bob@acme.com"}}); status != http.StatusConflict || !strings.Contains(page, "at least one workspace") {
			t.Fatalf("taking bob out of his last workspace: %d", status)
		}
		if spaces := f.roster().SpacesOf("bob@acme.com"); len(spaces) != 1 || spaces[0].ID != "main" {
			t.Fatalf("bob is still in his last workspace: %+v", spaces)
		}
	})
}

func TestRemovingAPersonFromTheOrganizationTakesThemOutOfEveryWorkspace(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f, ada := acme(t)
		f.invitedAndJoined(ada, "bob@acme.com")
		ada.post("/org/workspaces", url.Values{"name": {"Engineering"}})
		ada.post("/org/workspaces/add", url.Values{"workspace": {"engineering"}, "email": {"bob@acme.com"}})
		if status, _, _ := ada.post("/org/people/remove", url.Values{"email": {"bob@acme.com"}}); status != http.StatusSeeOther {
			t.Fatalf("removing bob: %d", status)
		}
		if spaces := f.roster().SpacesOf("bob@acme.com"); len(spaces) != 0 {
			t.Fatalf("a removed person is in no workspace: %+v", spaces)
		}
	})
}

func TestEveryOrganizationAndPersonWithNoWorkspaceIsGivenTheFirst(t *testing.T) {
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
		if err := f.store.db.InTx(ctx, func(tx dbkit.Tx) error { return fillFirstWorkspaces(ctx, tx) }); err != nil {
			t.Fatal(err)
		}
		spaces, err := f.store.Workspaces(ctx, "acme-com")
		if err != nil || len(spaces) != 1 || spaces[0].ID != "main" || spaces[0].Name != "Acme" || !slices.Equal(spaces[0].Members, []string{"ada@acme.com", "bob@acme.com"}) {
			t.Fatalf("an organization kept from before workspaces: %+v, %v", spaces, err)
		}
	})
}
