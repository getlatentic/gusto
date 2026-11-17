package cloud

import (
	"net/http"
	"strings"
	"testing"
)

// withAppsApart is gusto cloud with organizations at paths and shared apps on a domain of their own.
func withAppsApart(t *testing.T) *front {
	t.Helper()
	return newFront(t, Settings{PerDomain: 100}, func(c *Config) {
		c.Controller = controlled(t, c.Store)
		c.Controller.AppsURL = "https://apps.test"
		c.OrgPaths = true
		c.AppsDomain = "apps.test"
	})
}

// On a domain of their own, an app is reached there and nowhere under gusto cloud's own name, opening one goes there, and
// a form may end there.
func TestSharedAppsAreServedOnTheirOwnDomain(t *testing.T) {
	f := withAppsApart(t)
	b := f.signedUp("ada@acme.com")
	if slug, id, ok := f.server.appHost("acme-com--" + someApp + ".apps.test"); !ok || slug != "acme-com" || id != someApp {
		t.Errorf("an app's host name on the apps' domain: %q %q %v", slug, id, ok)
	}
	if _, _, ok := f.server.appHost("acme-com--" + someApp + ".cloud.test"); ok {
		t.Error("an app is reached under gusto cloud's own name")
	}
	if status, seen := whoami(t, b, "acme-com--"+someApp+".apps.test", nil); status != http.StatusOK || seen["app"] == "" {
		t.Errorf("an app's request goes to its organization's server, signed for the app: %d %v", status, seen)
	}
	status, _, header := b.get2("/open?app=" + someApp)
	if location := header.Get("Location"); status != http.StatusSeeOther || !strings.HasPrefix(location, "http://acme-com--"+someApp+".apps.test/auth/entry?t=") {
		t.Errorf("opening an app goes to its own domain: %d %q", status, location)
	}
	if policy := f.server.orgsSource(); policy != " http://*.cloud.test http://*.apps.test" {
		t.Errorf("a form may end at an organization's or an app's address: %q", policy)
	}
	if _, seen := whoamiAt(t, b, "/workspace/acme-com/whoami", nil); seen["apps"] != "https://apps.test" {
		t.Errorf("the organization's server is told where its apps are: %v", seen["apps"])
	}
}
