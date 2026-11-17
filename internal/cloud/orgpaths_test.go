package cloud

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/web"
)

// withServersAtPaths is gusto cloud with organizations at /workspace/<slug> on its own name, each served by the fake.
func withServersAtPaths(t *testing.T) *front {
	t.Helper()
	return newFront(t, Settings{PerDomain: 100}, func(c *Config) {
		c.Controller = controlled(t, c.Store)
		c.OrgPaths = true
	})
}

func whoamiAt(t *testing.T, b *browser, path string, header map[string]string) (int, map[string]any) {
	t.Helper()
	status, body, _ := b.do("GET", path, nil, header)
	var seen map[string]any
	json.Unmarshal([]byte(body), &seen)
	return status, seen
}

// Going in at a path: the organization shares gusto cloud's sign-in, so the browser is sent to its path with no token, and
// every request there says whose it is.
func TestAtAPathOpeningAnOrganizationNeedsNoSignInOfItsOwn(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := withServersAtPaths(t)
		b := f.signedUp("ada@acme.com")
		status, _, header := b.get2("/open?team=eng")
		if location := header.Get("Location"); status != http.StatusSeeOther || location != "http://cloud.test/workspace/acme-com/auth/entry?team=eng" {
			t.Fatalf("%d %q", status, location)
		}
		status, _, answer := b.do("GET", "/workspace/acme-com/auth/entry?team=eng", nil, nil)
		if status != http.StatusSeeOther || answer.Get("Location") != "/workspace/acme-com/" || answer.Get("Set-Cookie") != "" {
			t.Errorf("the server lets the person in with no cookie of its own, and its redirect is under the path: %d %v", status, answer)
		}
		if _, seen := whoamiAt(t, b, "/workspace/acme-com/whoami", nil); seen["person"] != "ada@acme.com" {
			t.Errorf("a request says whose it is: %v", seen)
		}
		if got := shownAt(t, b, "/org"); got.Address != "cloud.test/workspace/acme-com" {
			t.Errorf("the organization's address is its path: %q", got.Address)
		}
	})
}

// Whose a request is, is said by gusto cloud alone: a request with no sign-in says nobody, whatever it claims, and what is
// said for one organization is not good at another. Whether the person may do anything there is the organization's to
// decide.
func TestAtAPathOnlyGustoCloudSaysWhoseARequestIs(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := withServersAtPaths(t)
		ada := f.signedUp("ada@acme.com")
		f.signedUp("bob@globex.com")
		_, acme := whoamiAt(t, ada, "/workspace/acme-com/whoami", nil)
		forged := map[string]string{web.PersonHeader: acme["personHeader"].(string)}
		if _, seen := whoamiAt(t, f.browser(), "/workspace/acme-com/whoami", forged); seen["person"] != "" || seen["personHeader"] != "" {
			t.Errorf("a request with no sign-in carried somebody's: %v", seen)
		}
		if _, seen := whoamiAt(t, ada, "/workspace/globex-com/whoami", nil); seen["person"] != "ada@acme.com" {
			t.Errorf("a request to another organization says whose it is, for that organization: %v", seen)
		}
		if _, seen := whoamiAt(t, ada, "/workspace/globex-com/whoami", forged); seen["personHeader"] == forged[web.PersonHeader] {
			t.Error("what was said for one organization went to another")
		}
	})
}

// An organization's server is asked for the page below the organization's path and told the path. Its redirects go under
// the path, and its cookies, which it keeps to the path itself, come back as it set them; gusto cloud's own stay here.
func TestAtAPathTheOrganizationsServerIsAskedForThePageBelowIt(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := withServersAtPaths(t)
		b := f.signedUp("ada@acme.com")
		status, seen := whoamiAt(t, b, "/workspace/acme-com/whoami?x=1", nil)
		if status != http.StatusOK || seen["org"] != "acme-com" || seen["path"] != "/whoami?x=1" || seen["prefix"] != "/workspace/acme-com" {
			t.Errorf("%d %v", status, seen)
		}
		if status, _ := whoamiAt(t, b, "/workspace/nobody/whoami", nil); status != http.StatusNotFound {
			t.Errorf("no organization of that name: %d", status)
		}
		if status, _ := whoamiAt(t, b, "/workspace/acme-com/../globex-com/whoami", nil); status == http.StatusOK {
			t.Error("a path that climbs out of one organization reaches another")
		}
		status, _, answer := b.do("GET", "/workspace/acme-com/kept-cookie", nil, nil)
		if status != http.StatusSeeOther || answer.Get("Location") != "/workspace/acme-com/chat?x=1" {
			t.Errorf("%d %v", status, answer)
		}
		if set := answer.Get("Set-Cookie"); set != "__Secure-gusto-workspace=w; Path=/workspace/acme-com; HttpOnly; Secure" {
			t.Errorf("the server's cookie comes back as it set it: %q", set)
		}
		_, seen = whoamiAt(t, f.browser(), "/workspace/acme-com/whoami", map[string]string{"Cookie": "__Secure-gusto-workspace=w; gusto_cloud=x; other=1"})
		if seen["cookie"] != "__Secure-gusto-workspace=w; other=1" {
			t.Errorf("the server's own cookies go to it, and gusto cloud's stay here: %q", seen["cookie"])
		}
	})
}

// At paths, an organization's old host name sends a kept link to the same page at its path.
func TestAtAPathAnOrganizationsHostNameSendsALinkToItsPath(t *testing.T) {
	f := withServersAtPaths(t)
	f.signedUp("ada@acme.com")
	status, _, header := f.browser().at("acme-com.cloud.test", "GET", "/jobs/1?a=b", nil)
	if status != http.StatusPermanentRedirect || header.Get("Location") != "http://cloud.test/workspace/acme-com/jobs/1?a=b" {
		t.Errorf("%d %v", status, header)
	}
}

// An organization at a path shares gusto cloud's origin, so the browser lets only gusto's own pages read what gusto cloud
// says about the person, and the page names its organization, which the person must be in.
func TestAtAPathThePageThatAsksNamesItsOrganization(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := withServersAtPaths(t)
		b := f.signedUp("ada@acme.com")
		f.signedUp("bob@globex.com")
		status, body, header := b.do("GET", "/account.json?org=acme-com", nil, map[string]string{"Sec-Fetch-Site": "same-origin"})
		if status != http.StatusOK || !strings.Contains(body, `"email":"ada@acme.com"`) || header.Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("%d %s %v", status, body, header)
		}
		for name, path := range map[string]string{
			"no organization": "/account.json", "an organization they are not in": "/account.json?org=globex-com",
			"a name that is not one": "/account.json?org=..%2Fx",
		} {
			if status, _, _ := b.do("GET", path, nil, nil); status == http.StatusOK {
				t.Errorf("%s was answered", name)
			}
		}
		if status, _, _ := b.do("GET", "/account.json?org=acme-com", nil, map[string]string{"Sec-Fetch-Site": "cross-site"}); status == http.StatusOK {
			t.Error("a request another site made was answered")
		}
	})
}
