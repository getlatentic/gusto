package web

import (
	"net/url"
	"strings"
	"testing"
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

	page := admin.ask(t, "GET", "/admin/records?tab=activity", nil).Body.String()
	if strings.Contains(page, `">the server started`) {
		t.Errorf("the server starting is shown among what people did:\n%s", page)
	}
	if !strings.Contains(page, "2 times the server started are left out") || !strings.Contains(page, "reviewer") {
		t.Errorf("the page does not count what it left out, or lost what people did:\n%s", page)
	}
	all := admin.ask(t, "GET", "/admin/records?tab=activity&server=1", nil).Body.String()
	if !strings.Contains(all, `">the server started`) || !strings.Contains(all, "Show only what people did") {
		t.Errorf("asking for the server starting does not show it:\n%s", all)
	}
}
