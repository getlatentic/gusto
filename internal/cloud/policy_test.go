package cloud

import (
	"net/url"
	"strings"
	"testing"
)

// Signing in, signing up and joining are forms whose redirects end at an organization's or an app's address, and a
// browser holds every redirect of a form to the page's form-action.
func TestTheFrontDoorsFormsMayEndAtAnyOrganizationsAddress(t *testing.T) {
	f := withServers(t)
	_, _, header := f.browser().do("GET", "/signin", nil, nil)
	if policy := header.Get("Content-Security-Policy"); !strings.Contains(policy, "form-action 'self' http://*.cloud.test https://checkout.stripe.com") {
		t.Errorf("a sign-in's way into the organization is refused: %s", policy)
	}
	for address, want := range map[string]string{
		"https://gusto.example.com":   " https://*.gusto.example.com",
		"http://gusto.localhost:8051": " http://*.gusto.localhost:8051",
	} {
		base, _ := url.Parse(address)
		s := &Server{Config: Config{URL: base, Domain: base.Hostname()}}
		if got := s.orgsSource(); got != want {
			t.Errorf("%s: %q, want %q", address, got, want)
		}
	}
	if got := (&Server{Config: Config{URL: &url.URL{Scheme: "https", Host: "gusto.example.com"}}}).orgsSource(); got != "" {
		t.Errorf("a front door with no organizations' domain allows %q", got)
	}
}
