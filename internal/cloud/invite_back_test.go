package cloud

import (
	"net/http"
	"net/url"
	"testing"
)

// An organization's Members page sends the invitation from its own address, and gusto cloud takes it from there and not from
// another organization's or another site's (getlatentic/planning#835).
func TestAnInvitationIsTakenFromTheOrganizationsOwnPageOnly(t *testing.T) {
	f, ada := acme(t)
	invite := func(origin string) (int, http.Header) {
		t.Helper()
		form := url.Values{"email": {"bob@acme.com"}, "back": {"https://acme-com.cloud.test/admin/people"}}
		status, _, header := ada.do("POST", "/org/invite?org=acme-com", form, map[string]string{"Origin": origin})
		return status, header
	}
	for _, origin := range []string{"http://globex-com.cloud.test", "http://evil.example"} {
		if status, _ := invite(origin); status != http.StatusForbidden {
			t.Errorf("an invitation was sent from %s: %d", origin, status)
		}
	}
	if status, header := invite("http://acme-com.cloud.test"); status != http.StatusSeeOther || header.Get("Location") != "/org?sent=1" {
		t.Errorf("from the organization's page: %d to %q, and an http gusto cloud sends nobody to an http page", status, header.Get("Location"))
	}
	if invited, _ := f.store.Invitations(t.Context(), "acme-com"); len(invited) != 1 {
		t.Errorf("%d invitations, want the one from the organization's own page", len(invited))
	}
}

func TestAnInvitationGoesBackToTheMembersPageItWasSentFromWithAWordToSayItWent(t *testing.T) {
	front, _ := url.Parse("https://cloud.test")
	s := &Server{Config: Config{URL: front, Domain: "cloud.test", Controller: &Controller{}}}
	ada := Person{Email: "ada@acme.com", Org: "acme-com"}
	for back, want := range map[string]string{
		"https://acme-com.cloud.test/admin/people":        "https://acme-com.cloud.test/admin/people?sent=1",
		"https://acme-com.cloud.test/admin/people?a=b":    "https://acme-com.cloud.test/admin/people?a=b&sent=1",
		"https://acme-com.cloud.test/admin/people?sent=0": "https://acme-com.cloud.test/admin/people?sent=1",
		"https://globex-com.cloud.test/admin/people":      "/org?sent=1",
		"https://evil.example/admin/people":               "/org?sent=1",
		"http://acme-com.cloud.test/admin/people":         "/org?sent=1",
		"": "/org?sent=1",
	} {
		if got := s.sentTo(back, ada); got != want {
			t.Errorf("back=%q went to %q, want %q", back, got, want)
		}
	}
}
