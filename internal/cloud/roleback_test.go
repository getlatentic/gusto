package cloud

import (
	"net/http"
	"net/url"
	"testing"
)

// An organization's People page changes a role here from its own address, and nowhere else may.
func TestARoleIsChangedFromTheOrganizationsOwnAddressOnly(t *testing.T) {
	f, ada := acme(t)
	f.invitedAndJoined(ada, "bob@acme.com")
	role := func(origin, now string) (int, http.Header) {
		t.Helper()
		form := url.Values{"email": {"bob@acme.com"}, "role": {now}, "back": {"https://acme-com.cloud.test/admin/people"}}
		status, _, header := ada.do("POST", "/org/people/role?org=acme-com", form, map[string]string{"Origin": origin})
		return status, header
	}
	for _, origin := range []string{"http://globex-com.cloud.test", "http://evil.example", "http://acme-com--0123456789.cloud.test"} {
		if status, _ := role(origin, "admin"); status != http.StatusForbidden {
			t.Errorf("a role was changed from %s: %d", origin, status)
		}
	}
	if got, _ := member(f.roster(), "bob@acme.com"); got.Role != "runner" {
		t.Fatalf("a refused change was made: %+v", got)
	}
	status, header := role("http://acme-com.cloud.test", "admin")
	if status != http.StatusSeeOther || header.Get("Location") != "/org" {
		t.Errorf("from the organization's address: %d %s, and an http front door sends nobody to an http page", status, header.Get("Location"))
	}
	if got, _ := member(f.roster(), "bob@acme.com"); got.Role != "admin" {
		t.Errorf("the change from the organization's own address was not made: %+v", got)
	}
}

func TestAChangeGoesBackOnlyToAnHTTPSPageOfTheOrganizationItWasMadeIn(t *testing.T) {
	front, _ := url.Parse("https://cloud.test")
	s := &Server{Config: Config{URL: front, Domain: "cloud.test", Controller: &Controller{}}}
	ada := Person{Email: "ada@acme.com", Org: "acme-com"}
	for back, want := range map[string]string{
		"https://acme-com.cloud.test/admin/people":     "https://acme-com.cloud.test/admin/people",
		"https://acme-com.cloud.test/admin/people?x=1": "https://acme-com.cloud.test/admin/people?x=1",
		"http://acme-com.cloud.test/admin/people":      "/org",
		"https://globex-com.cloud.test/admin/people":   "/org",
		"https://acme-com.cloud.test:8443/admin":       "/org",
		"https://evil.example/admin/people":            "/org",
		"https://acme-com.cloud.test.evil.example/":    "/org",
		"https://acme-com--0123456789.cloud.test/":     "/org",
		"https://ada@acme-com.cloud.test/":             "/org",
		"//acme-com.cloud.test/admin/people":           "/org",
		"/admin/people":                                "/org",
		"javascript:alert(1)":                          "/org",
		"":                                             "/org",
	} {
		if got := s.backTo(back, ada); got != want {
			t.Errorf("back=%q went to %q, want %q", back, got, want)
		}
	}
}
