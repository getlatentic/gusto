package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func askAs(t *testing.T, s *site, path, accept string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest("GET", path, nil)
	request.Header.Set("Accept", accept)
	if s.cookie != nil {
		request.AddCookie(s.cookie)
	}
	recorder := httptest.NewRecorder()
	s.handler.ServeHTTP(recorder, request)
	return recorder
}

func TestAnEvidenceFileIsAPageWithTheWayBackForABrowserAndTheBytesForAnyoneElse(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id := aTask(t, site, url.Values{"brief": {"files"}, "gate": {"true"}})
	<-site.ran
	file := "/jobs/" + id + "/files/transcript.log"

	page := askAs(t, site, file, "text/html,application/xhtml+xml")
	body := page.Body.String()
	for _, want := range []string{"<html", `href="/jobs/` + id + `"`, "Back to the record", "?raw=1"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page lacks %q:\n%s", want, body)
		}
	}

	panel := askAs(t, site, file+"?panel=1", "text/html")
	if strings.Contains(panel.Body.String(), "<html") || panel.Code != http.StatusOK {
		t.Errorf("the tab is not a fragment: %d\n%s", panel.Code, panel.Body.String())
	}

	for name, accept := range map[string]string{"a program": "", "a program that takes anything": "*/*"} {
		raw := askAs(t, site, file, accept)
		if !strings.HasPrefix(raw.Header().Get("Content-Type"), "text/plain") || strings.Contains(raw.Body.String(), "<html") {
			t.Errorf("%s was given a page: %s", name, raw.Header().Get("Content-Type"))
		}
	}
	if raw := askAs(t, site, file+"?raw=1", "text/html"); !strings.HasPrefix(raw.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("?raw=1 was given a page")
	}
}
