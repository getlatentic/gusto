package web

import (
	"net/url"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/changelog"
)

// People's actions come first on the Activity page: the server starting, which every deploy does, is left out and counted, and
// shown when asked for.
func TestActivityLeavesOutTheServerStartingUntilAskedFor(t *testing.T) {
	here, _ := recording(t)
	here.server.UseAdmins([]string{"admin@example.com"})
	admin := here.as(t, "admin@example.com")
	admin.ask(t, "POST", "/admin/skill-sets", url.Values{"name": {"reviewer"}})
	here.server.UseSettings(map[string]string{"GUSTO_ADMINS": "admin@example.com"})
	here.server.UseSettings(map[string]string{"GUSTO_ADMINS": "admin@example.com"})

	starts := func(a *activityJSON) int {
		n := 0
		for _, each := range a.Rows {
			if each.Kind == changelog.ServerStarted {
				n += each.Times
			}
		}
		return n
	}
	page := recordsPage(t, admin, "?tab=activity").Activity
	if page == nil || starts(page) != 0 {
		t.Fatalf("the server starting is shown among what people did: %+v", page)
	}
	if page.LeftOut != 2 || page.WithServer || !strings.Contains(page.said(), "reviewer") {
		t.Errorf("the page does not count what it left out, or lost what people did: %+v", page)
	}
	all := recordsPage(t, admin, "?tab=activity&server=1").Activity
	if all == nil || starts(all) == 0 || !all.WithServer {
		t.Errorf("asking for the server starting does not show it: %+v", all)
	}
}
