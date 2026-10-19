package cloud

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/web"
)

const someApp = "0123456789"

func TestAnAppHostNameIsSlugTwoHyphensAndAnAppIdUnderTheDomainAndNothingElseIs(t *testing.T) {
	f := withServers(t)
	for host, want := range map[string]string{
		"acme-com--" + someApp + ".cloud.test":      "acme-com/" + someApp,
		"ACME-COM--" + someApp + ".Cloud.Test:8443": "acme-com/" + someApp,
		"acme-com.cloud.test":                       "",
		"cloud.test":                                "",
		"acme-com--" + someApp + ".other.test":      "",
		"acme-com--zzzzzzzzzz.cloud.test":           "",
		"acme-com--" + someApp[:9] + ".cloud.test":  "",
		"acme-com--" + someApp + "0.cloud.test":     "",
		"x.acme-com--" + someApp + ".cloud.test":    "",
		"acme--com--" + someApp + ".cloud.test":     "",
		"--" + someApp + ".cloud.test":              "",
		"admin-" + "-" + someApp + ".cloud.test":    "",
		"acme-com--" + someApp + "--x.cloud.test":   "",
	} {
		slug, id, ok := f.server.appHost(host)
		got := ""
		if ok {
			got = slug + "/" + id
		}
		if got != want {
			t.Errorf("%s: got %q, wanted %q", host, got, want)
		}
	}
	// An organisation's own address is never read as an app's.
	if _, _, ok := f.server.appHost("acme-com.cloud.test"); ok {
		t.Error("an organisation's address is an app's")
	}
	if slug, ok := f.server.orgSlug("acme-com--" + someApp + ".cloud.test"); ok {
		t.Errorf("an app's address is an organisation's: %q", slug)
	}
}

// what a request looks like to the organisation's server, as the fake reports it.
func whoami(t *testing.T, b *browser, host string, header map[string]string) (status int, seen map[string]any) {
	t.Helper()
	status, body, _ := b.at(host, "GET", "/whoami", header)
	seen = map[string]any{}
	json.Unmarshal([]byte(body), &seen)
	return status, seen
}

func TestARequestForAnAppIsSignedAsForThatAppAndNoOtherRequestCarriesTheHeader(t *testing.T) {
	f := withServers(t)
	b := f.signedUp("ada@acme.com")
	key, err := f.server.Controller.EntryKey("acme-com")
	if err != nil {
		t.Fatal(err)
	}

	_, seen := whoami(t, b, "acme-com--"+someApp+".cloud.test", nil)
	if seen["app"] != web.SignApp(key, someApp) {
		t.Errorf("an app's request is signed for the app: %v", seen["app"])
	}
	if seen["host"] != "acme-com--"+someApp+".cloud.test" {
		t.Errorf("the app's host name is kept for the server to see: %v", seen["host"])
	}
	// A client that writes the header itself is not believed, on either kind of host name.
	forged := map[string]string{web.AppHeader: web.SignApp(key, "9999999999")}
	if _, seen := whoami(t, b, "acme-com.cloud.test", forged); seen["app"] != "" {
		t.Errorf("the organisation's own address took a client's header: %v", seen["app"])
	}
	if _, seen := whoami(t, b, "acme-com--"+someApp+".cloud.test", forged); seen["app"] != web.SignApp(key, someApp) {
		t.Errorf("an app's address took a client's header: %v", seen["app"])
	}
	// Without one, a request for the organisation carries none.
	if _, seen := whoami(t, b, "acme-com.cloud.test", nil); seen["app"] != "" {
		t.Errorf("%v", seen["app"])
	}
}

func TestAnAppOfAnOrganisationThatIsNotThereIsNotFound(t *testing.T) {
	f := withServers(t)
	b := f.browser()
	if status, _, _ := b.at("nobody-com--"+someApp+".cloud.test", "GET", "/", nil); status != http.StatusNotFound {
		t.Errorf("%d", status)
	}
}

func TestOpeningAnAppSignsThePersonInOnTheAppsOwnHostNameWithATokenForTheirOrganisation(t *testing.T) {
	f := withServers(t)
	b := f.signedUp("ada@acme.com")
	status, _, header := b.get2("/open?app=" + someApp)
	location := header.Get("Location")
	if status != http.StatusSeeOther || !strings.HasPrefix(location, "http://acme-com--"+someApp+".cloud.test/auth/entry?t=") {
		t.Fatalf("%d %q", status, location)
	}
	// The fake organisation's server checks the token as the real one does, on the app's host name.
	parsed, _ := url.Parse(location)
	status, _, header = b.at(parsed.Host, "GET", parsed.RequestURI(), nil)
	if status != http.StatusSeeOther || header.Get("Location") != "/" || !strings.Contains(header.Get("Set-Cookie"), "gusto=ada@acme.com|admin") {
		t.Errorf("the app's host signed the person in: %d %v", status, header)
	}
	for _, bad := range []string{"", "zzzz", "../x", someApp + "0"} {
		if bad == "" {
			continue
		}
		if status, _, _ := b.get2("/open?app=" + url.QueryEscape(bad)); status != http.StatusNotFound {
			t.Errorf("app=%q gave %d", bad, status)
		}
	}
	stranger := f.browser()
	if status, _, header := stranger.get2("/open?app=" + someApp); status != http.StatusSeeOther || header.Get("Location") != "/signin" {
		t.Errorf("a stranger was sent on to an app: %d %q", status, header.Get("Location"))
	}
}
