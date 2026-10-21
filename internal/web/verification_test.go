package web

import (
	"strings"
	"testing"
)

// An administrator has a section that says, run by run, whether the files still match what was recorded and who signed them.
func TestTheConsoleHasAVerificationSectionForTheRuns(t *testing.T) {
	root, trusted := t.TempDir(), t.TempDir()
	claiming(t, root, "unsigned-20260927-0100")
	sealWith(t, claiming(t, root, "acme-20260927-0100"), trusted, "acme")
	site := openKeyed(t, root, keyringAt(t, trusted))
	site.signIn(t)
	page := site.ask(t, "GET", "/admin/verification", nil)
	body := page.Body.String()
	if page.Code != 200 || !strings.Contains(body, "Verification") || !strings.Contains(body, `href="/admin/verification"`) {
		t.Fatalf("the section is missing: %d\n%s", page.Code, body)
	}
	for _, want := range []string{"verified", "not signed"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not say %q:\n%s", want, body)
		}
	}
}
