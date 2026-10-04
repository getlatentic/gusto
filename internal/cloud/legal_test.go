package cloud

import (
	"net/http"
	"strings"
	"testing"
)

func TestThePrivacyNoticeNamesWhatTheFrontDoorCollectsAndWhoHandlesIt(t *testing.T) {
	f := demoFront(t)
	status, page, _ := f.browser().get2("/privacy")
	if status != http.StatusOK {
		t.Fatalf("%d", status)
	}
	for _, want := range []string{"name, work email address and company", "Cloudflare", "Amazon Web Services", "DeepSeek", "Google", "removed when the demo ends", "one cookie", "Do not put confidential"} {
		if !strings.Contains(page, want) {
			t.Errorf("the privacy notice does not say %q", want)
		}
	}
	if strings.Contains(strings.ToLower(page), "haas") {
		t.Error("the privacy notice calls the product haas")
	}
}

func TestTheTermsSayHowLongADemoLastsAndWhatMayNotBePutIntoIt(t *testing.T) {
	f := demoFront(t)
	status, page, _ := f.browser().get2("/terms")
	if status != http.StatusOK || !strings.Contains(page, "for 30 minutes") || !strings.Contains(page, "Do not put confidential, personal or regulated information") {
		t.Errorf("%d\n%s", status, page)
	}
}

func TestTheLandingPageAndTheDemoFormLinkToThePrivacyNoticeAndTheTerms(t *testing.T) {
	f := demoFront(t)
	for _, path := range []string{"/", "/demo"} {
		_, page, _ := f.browser().get2(path)
		for _, link := range []string{`href="/privacy"`, `href="/terms"`} {
			if !strings.Contains(page, link) {
				t.Errorf("%s does not link to %s", path, link)
			}
		}
	}
}

func TestTheLegalPagesAreNotListedInSearch(t *testing.T) {
	f := demoFront(t)
	for _, path := range []string{"/privacy", "/terms"} {
		_, page, _ := f.browser().get2(path)
		if !strings.Contains(page, `content="noindex,nofollow"`) {
			t.Errorf("%s may be listed", path)
		}
	}
}
