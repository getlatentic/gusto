package cloud

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/entry"
)

// A person who signs out in their organization's server is sent here with that server's word for it, and is signed
// out here too. A link without that word, which any site could make, signs nobody out.
func TestSigningOutInAnOrganizationsServerSignsThePersonOutOfTheFrontDoor(t *testing.T) {
	f := withServers(t)
	ada := f.signedUp("ada@acme.com")
	key, err := f.server.Controller.EntryKey("acme-com")
	if err != nil {
		t.Fatal(err)
	}
	inside := func(b *browser) bool {
		t.Helper()
		_, _, header := b.do("GET", "/", nil, nil)
		return header.Get("Location") == "/open"
	}
	exit, _ := entry.MintExit(key, "acme-com", "ada@acme.com", time.Now())
	enter, _ := entry.Mint(key, "acme-com", "ada@acme.com", "admin", time.Now())
	stale, _ := entry.MintExit(key, "acme-com", "ada@acme.com", time.Now().Add(-2*entry.Life))
	foreign, _ := entry.MintExit([]byte("another key another key another!"), "acme-com", "ada@acme.com", time.Now())

	for name, query := range map[string]url.Values{
		"no token":                        {"org": {"acme-com"}},
		"a token to enter with":           {"org": {"acme-com"}, "t": {enter}},
		"an old token":                    {"org": {"acme-com"}, "t": {stale}},
		"another key's token":             {"org": {"acme-com"}, "t": {foreign}},
		"another organization":            {"org": {"globex-com"}, "t": {exit}},
		"an organization that is not one": {"org": {"../acme-com"}, "t": {exit}},
	} {
		status, _, header := ada.do("GET", "/signout?"+query.Encode(), nil, nil)
		if status != http.StatusSeeOther || header.Get("Location") != "/" || !inside(ada) {
			t.Errorf("%s signed ada out: %d %v", name, status, header)
		}
	}

	status, _, header := ada.do("GET", "/signout?"+url.Values{"org": {"acme-com"}, "t": {exit}}.Encode(), nil, nil)
	if status != http.StatusSeeOther || header.Get("Location") != "/signin" {
		t.Fatalf("%d %v", status, header)
	}
	if inside(ada) {
		t.Error("ada is still signed in to the front door")
	}
}
