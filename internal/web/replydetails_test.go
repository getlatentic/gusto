package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// A reply's own page is for an administrator, who reaches it from the admin pages. Somebody else asking for it is taken back
// to the conversation, and no reply links to it.
func TestAReplysDetailsPageIsForAdministrators(t *testing.T) {
	site := open(t)
	mine := site.as(t, alice)
	where := startedThread(t, mine)
	said := mine.ask(t, "POST", where+"/say", url.Values{"said": {"hello there"}})
	if said.Code != http.StatusSeeOther {
		t.Fatalf("saying something answered %d", said.Code)
	}
	_, turn, _ := strings.Cut(said.Header().Get("Location"), "#")
	if turn == "" {
		t.Fatalf("the reply gave no turn to open: %q", said.Header().Get("Location"))
	}
	page := mine.ask(t, "GET", "/jobs/"+turn, nil)
	if page.Code != http.StatusSeeOther || !strings.HasPrefix(page.Header().Get("Location"), where) {
		t.Errorf("a person's own reply page answered %d to %q, want the conversation", page.Code, page.Header().Get("Location"))
	}
	if body := mine.ask(t, "GET", where, nil).Body.String(); strings.Contains(body, `href="/jobs/`+turn+`"`) {
		t.Error("the reply links to a page that is not theirs to open")
	}
	site.signIn(t)
	if page := site.ask(t, "GET", "/jobs/"+turn, nil); page.Code != http.StatusOK {
		t.Errorf("an administrator's reply page answered %d", page.Code)
	}
	if body := site.ask(t, "GET", where, nil).Body.String(); strings.Contains(body, `href="/jobs/`+turn+`"`) {
		t.Error("the conversation links an administrator to the reply page; it is reached from the admin pages")
	}
}
