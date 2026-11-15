package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
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

func TestAnRecordFileIsAPageWithTheWayBackForABrowserAndTheBytesForAnyoneElse(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id := aTask(t, site, url.Values{"brief": {"files"}, "gate": {"true"}})
	<-site.ran
	file := "/jobs/" + id + "/files/transcript.log"

	page := askAs(t, site, file, "text/html,application/xhtml+xml")
	if body := page.Body.String(); page.Code != http.StatusOK || !strings.Contains(body, `<div id="root">`) {
		t.Errorf("a browser is not given the file's page: %d\n%s", page.Code, body)
	}
	var shown fileJSON
	site.data(t, file, &shown)
	crumbs := []pageCrumb{{"Activity", "/jobs"}, {"Details", "/jobs/" + id}}
	if shown.Back != "/jobs/"+id || shown.BackWord != "Back to the record" || !reflect.DeepEqual(shown.Crumbs, crumbs) || shown.File.Raw != file+"?raw=1" ||
		shown.File.Download != file+"?raw=1&download=1" || shown.File.Kind != kindText || shown.Save != "" {
		t.Errorf("the file's page is not under its record, with the way back: %+v", shown)
	}
	if got := site.ask(t, "GET", "/app/api/jobs/"+id+"/files/nothing.log", nil); got.Code != http.StatusNotFound {
		t.Errorf("a file the record does not have answered %d", got.Code)
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
