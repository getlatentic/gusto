package cloud

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestADemoVisitorBringsAColleagueWhoLandsInTheDemo(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := demoFront(t)
		visitor := startDemoAs(t, f, "Ada Lovelace", "ada@acme.com", "Acme Research")
		f.server.Wait()
		if status, _, _ := visitor.do("GET", "/demo/wait", nil, nil); status != http.StatusSeeOther {
			t.Fatalf("the demo did not open: %d", status)
		}
		if status, page := visitor.get("/demo/colleague"); status != http.StatusOK || shown(t, page).Page != "demo-colleague" || !shown(t, page).IsAdmin || shown(t, page).Link != "" {
			t.Fatalf("the visitor is offered the invitation: %d %+v", status, shown(t, page))
		}
		status, page, _ := visitor.post("/demo/colleague", url.Values{"email": {"Grace@Acme.com"}})
		if status != http.StatusOK || shown(t, page).Email != "grace@acme.com" {
			t.Fatalf("%d %+v", status, shown(t, page))
		}
		link := f.linkTo("grace@acme.com", "/invite?t=")
		if got := shown(t, page).Link; got != "http://cloud.test"+link {
			t.Errorf("the page shows the link %q, not the link the mail carries, %s", got, link)
		}
		colleague := f.browser()
		status, _, header := colleague.post("/invite", url.Values{"t": {tokenOf(link)}})
		if status != http.StatusSeeOther || header.Get("Location") != "/open" {
			t.Errorf("a colleague who joins a demo goes straight in: %d %v", status, header)
		}
		if status, page := colleague.get("/demo/colleague"); status != http.StatusOK || shown(t, page).IsAdmin {
			t.Errorf("a colleague is not offered the invitation: %d %+v", status, shown(t, page))
		}
	})
}

func TestADemoAllowsOneColleagueAndNoMore(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := demoFront(t)
		visitor := startDemoAs(t, f, "Ada Lovelace", "ada@acme.com", "Acme Research")
		if status, _, _ := visitor.post("/demo/colleague", url.Values{"email": {"grace@acme.com"}}); status != http.StatusOK {
			t.Fatalf("first colleague: %d", status)
		}
		if status, page, _ := visitor.post("/demo/colleague", url.Values{"email": {"alan@acme.com"}}); status == http.StatusOK || !strings.Contains(shown(t, page).Error, "as many people as it may") {
			t.Errorf("a second colleague was let in: %d %q", status, shown(t, page).Error)
		}
	})
}

func TestAnOrdinaryOrganizationHasNoColleagueInvitationPage(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := newFront(t, Settings{}, nil)
		admin := f.verifiedAdmin("ada@acme.com", "Acme")
		if status, _, header := admin.do("GET", "/demo/colleague", nil, nil); status != http.StatusSeeOther || header.Get("Location") != "/org" {
			t.Errorf("%d %v", status, header)
		}
	})
}
