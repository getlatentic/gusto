package web

import (
	"strings"
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
	page := site.ask(t, "GET", "/admin/verification", nil)
	body := page.Body.String()
	if page.Code != 200 || !strings.Contains(body, `href="/admin/verification"`) || !strings.Contains(body, "Needs a look") {
		t.Fatalf("the section is missing: %d\n%s", page.Code, body)
	}
	if !strings.Contains(body, "unsigned-20260927-0100") && !strings.Contains(body, "/jobs/unsigned-20260927-0100") {
		t.Errorf("the unsigned run is not listed:\n%s", body)
	}
	if strings.Contains(body, "/jobs/acme-20260927-0100") {
		t.Errorf("a verified run is listed among those needing a look:\n%s", body)
	}
}
