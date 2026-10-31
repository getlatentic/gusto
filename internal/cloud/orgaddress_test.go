package cloud

import (
	"net/url"
	"path/filepath"
	"testing"
)

// An organization's server is told the address it is reached at, which an app a person signs in to sends them back to, and
// where it keeps their sign-ins, in its own data.
func TestAnOrganizationsServerKnowsItsOwnAddressAndWhereItsSignInsAre(t *testing.T) {
	store := open(t, Settings{})
	public, _ := url.Parse("https://gusto.example:8443")
	c := &Controller{Store: store, Cell: &LocalCell{}, FrontDoor: "https://gusto.example:8443",
		OrgURL: func(slug string) string { return OrgAddress(public, "gusto.example", slug) }}
	env, err := c.environment("acme", Runtime{Port: 1, ProxyPort: 2}, c.Cell)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := settingOf(env, "GUSTO_PUBLIC_URL"); got != "https://acme.gusto.example:8443" {
		t.Errorf("the address is %q", got)
	}
	if got, _ := settingOf(env, "GUSTO_CONNECTORS"); got != filepath.Join(store.DataDir("acme"), "connectors") {
		t.Errorf("the sign-ins are kept at %q", got)
	}
	if _, count := settingOf(environmentFor(t, nil, "acme"), "GUSTO_CONNECTORS"); count != 0 {
		t.Error("a server with no address of its own is offered sign-ins to apps")
	}
}
