package web

import (
	"net/http"
	"slices"
	"testing"
)

// An administrator has a section that counts the runs by whether their files still match what was recorded, and lists
// only the ones that need a look.
func TestTheConsoleHasAVerificationSectionThatListsOnlyWhatNeedsALook(t *testing.T) {
	root, trusted := t.TempDir(), t.TempDir()
	claiming(t, root, "unsigned-20260927-0100")
	sealWith(t, claiming(t, root, "acme-20260927-0100"), trusted, "acme")
	site := openKeyed(t, root, keyringAt(t, trusted))
	site.signIn(t)
	if got := site.ask(t, "GET", "/admin/records?tab=checks", nil); got.Code != http.StatusOK {
		t.Fatalf("the section is missing: %d", got.Code)
	}
	page := recordsPage(t, site, "?tab=checks")
	if page.Checks == nil || page.Tab != "checks" {
		t.Fatalf("the task checks are not given: %+v", page)
	}
	var listed []string
	for _, each := range page.Checks.Rows {
		listed = append(listed, each.Href)
	}
	if !slices.Contains(listed, "/jobs/unsigned-20260927-0100") {
		t.Errorf("the unsigned run is not listed: %v", listed)
	}
	if slices.Contains(listed, "/jobs/acme-20260927-0100") {
		t.Errorf("a verified run is listed among those needing a look: %v", listed)
	}
	if page.Health.Broken || page.Health.Runs != "the last 2 tasks check out" {
		t.Errorf("the top line does not say the runs check out: %+v", page.Health)
	}
}
