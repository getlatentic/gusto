package web

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// A browser asking for a page that is not there is shown a page with a way out, and anything else still gets the plain line.
func TestAPageThatIsNotThereHasAWayOut(t *testing.T) {
	site := open(t)
	site.signIn(t)
	ask := func(accept string) *httptest.ResponseRecorder {
		request := httptest.NewRequest("GET", "/instances/nobody/threads/9", nil)
		request.AddCookie(site.cookie)
		if accept != "" {
			request.Header.Set("Accept", accept)
		}
		recorder := httptest.NewRecorder()
		site.handler.ServeHTTP(recorder, request)
		return recorder
	}
	page := ask("text/html,application/xhtml+xml")
	if page.Code != 404 || !strings.Contains(page.Body.String(), `id="back"`) || !strings.Contains(page.Header().Get("Content-Type"), "text/html") {
		t.Errorf("a browser got %d %q %q", page.Code, page.Header().Get("Content-Type"), page.Body.String())
	}
	if strings.Count(page.Body.String(), "<!doctype html>") != 1 || strings.Contains(page.Body.String(), "404 page not found") {
		t.Errorf("the page was written twice or with the line after it: %q", page.Body.String())
	}
	plain := ask("application/json")
	if plain.Code != 404 || !strings.Contains(plain.Body.String(), "404 page not found") {
		t.Errorf("a program got %d %q", plain.Code, plain.Body.String())
	}
}
