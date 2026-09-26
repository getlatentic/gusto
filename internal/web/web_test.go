package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/haas/internal/catalog"
	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/talk"
)

const password = "a-shared-password"

type site struct {
	handler http.Handler
	index   *jobs.Index
	cookie  *http.Cookie
	ran     chan string
}

func open(t *testing.T) *site {
	t.Helper()
	index, err := jobs.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ran := make(chan string, 4)
	queue := jobs.Start(index, func(dir job.Dir) (int, error) {
		if err := os.MkdirAll(dir.Out(), 0o755); err != nil {
			return 0, err
		}
		if err := os.WriteFile(dir.OutFile("transcript.log"), []byte("first line\nsecond line\n"), 0o644); err != nil {
			return 0, err
		}
		ran <- dir.Path()
		return 0, nil
	}, 1)
	t.Cleanup(queue.Stop)
	sessions, err := talk.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(index, sessions, queue, password, offered(), []string{"platform/test-model"})
	if err != nil {
		t.Fatal(err)
	}
	return &site{handler: server.Handler(), index: index, ran: ran}
}

func (s *site) ask(t *testing.T, method, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	var request *http.Request
	if form == nil {
		request = httptest.NewRequest(method, path, nil)
	} else {
		request = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if s.cookie != nil {
		request.AddCookie(s.cookie)
	}
	recorder := httptest.NewRecorder()
	s.handler.ServeHTTP(recorder, request)
	return recorder
}

func (s *site) signIn(t *testing.T) {
	t.Helper()
	answer := s.ask(t, "POST", "/login", url.Values{"password": {password}})
	for _, cookie := range answer.Result().Cookies() {
		if cookie.Name == cookieName {
			s.cookie = cookie
			return
		}
	}
	t.Fatal("signing in gave no cookie")
}

func TestAStrangerIsSentToSignInAndNotToTheThingTheyAskedFor(t *testing.T) {
	site := open(t)
	for _, path := range []string{
		"/", "/jobs/anything", "/watch/anything",
		"/jobs/anything/files/manifest.json", "/jobs/anything/bundle",
	} {
		answer := site.ask(t, "GET", path, nil)
		if answer.Code != http.StatusSeeOther || answer.Header().Get("Location") != "/login" {
			t.Errorf("%s gave %d to %q", path, answer.Code, answer.Header().Get("Location"))
		}
	}
	posted := site.ask(t, "POST", "/jobs", url.Values{"brief": {"x"}, "repo": {"y"}, "gate": {"true"}})
	if posted.Header().Get("Location") != "/login" {
		t.Error("a stranger must not be able to start a job")
	}
	if len(site.index.List()) != 0 {
		t.Fatal("a job was created by somebody who had not signed in")
	}
}

func TestAWrongPasswordAdmitsNobody(t *testing.T) {
	site := open(t)
	answer := site.ask(t, "POST", "/login", url.Values{"password": {"not it"}})
	for _, cookie := range answer.Result().Cookies() {
		if cookie.Name == cookieName && cookie.Value != "" {
			t.Fatal("a wrong password issued a cookie")
		}
	}
	if !strings.Contains(answer.Header().Get("Location"), "wrong") {
		t.Fatalf("expected to be sent back, got %q", answer.Header().Get("Location"))
	}
}

func TestSomebodySignedInSeesTheList(t *testing.T) {
	site := open(t)
	site.signIn(t)
	answer := site.ask(t, "GET", "/", nil)
	if answer.Code != http.StatusOK || !strings.Contains(answer.Body.String(), "Nothing has run here yet") {
		t.Fatalf("code %d", answer.Code)
	}
}

func TestAJobSubmittedFromThePageIsTheSameJobTheCommandLineMakes(t *testing.T) {
	site := open(t)
	site.signIn(t)
	answer := site.ask(t, "POST", "/jobs", url.Values{
		"repo":    {"/somewhere/repo"},
		"brief":   {"Make the adder add"},
		"gate":    {`sh -c "test 2 = 2"`},
		"harness": {"fixture"},
	})
	if answer.Code != http.StatusSeeOther {
		t.Fatalf("code %d", answer.Code)
	}
	id := strings.TrimPrefix(answer.Header().Get("Location"), "/jobs/")
	record, found := site.index.Get(id)
	if !found {
		t.Fatalf("no job called %q", id)
	}
	spec, err := job.Read(record.Dir(site.index.Root()))
	if err != nil {
		t.Fatal(err)
	}
	if spec.Brief != "Make the adder add" || spec.Repo != "/somewhere/repo" {
		t.Fatalf("the spec is %+v", spec)
	}
	if len(spec.Gate) != 3 || spec.Gate[2] != "test 2 = 2" {
		t.Fatalf("a quoted gate must survive the form: %#v", spec.Gate)
	}
	select {
	case <-site.ran:
	case <-time.After(5 * time.Second):
		t.Fatal("the job was never picked up")
	}
}

func TestAJobWithoutAGateIsRefusedAndNothingIsCreated(t *testing.T) {
	site := open(t)
	site.signIn(t)
	answer := site.ask(t, "POST", "/jobs", url.Values{"repo": {"/repo"}, "brief": {"do it"}, "gate": {"  "}})
	if !strings.Contains(answer.Header().Get("Location"), "refused") {
		t.Fatalf("sent to %q", answer.Header().Get("Location"))
	}
	if len(site.index.List()) != 0 {
		t.Fatal("a refused job left a directory behind")
	}
}

func TestTheStreamReplaysFromTheStartAndEndsWithTheVerdict(t *testing.T) {
	site := open(t)
	site.signIn(t)
	answer := site.ask(t, "POST", "/jobs", url.Values{
		"repo": {"/repo"}, "brief": {"watch me"}, "gate": {"true"}, "harness": {"fixture"},
	})
	id := strings.TrimPrefix(answer.Header().Get("Location"), "/jobs/")
	select {
	case <-site.ran:
	case <-time.After(5 * time.Second):
		t.Fatal("the job never ran")
	}
	watched := site.ask(t, "GET", "/watch/"+id, nil)
	body := watched.Body.String()
	for _, want := range []string{"data: first line", "data: second line", "event: ended"} {
		if !strings.Contains(body, want) {
			t.Errorf("the stream is missing %q:\n%s", want, body)
		}
	}
}

func TestAFileOutsideTheEvidenceCannotBeAskedFor(t *testing.T) {
	site := open(t)
	site.signIn(t)
	answer := site.ask(t, "POST", "/jobs", url.Values{
		"repo": {"/repo"}, "brief": {"files"}, "gate": {"true"}, "harness": {"fixture"},
	})
	id := strings.TrimPrefix(answer.Header().Get("Location"), "/jobs/")
	<-site.ran
	if got := site.ask(t, "GET", "/jobs/"+id+"/files/transcript.log", nil); got.Code != http.StatusOK {
		t.Fatalf("an evidence file should be readable, got %d", got.Code)
	}
	for _, name := range []string{"..%2f..%2fjob.json", "job.json", "state.json"} {
		if got := site.ask(t, "GET", "/jobs/"+id+"/files/"+name, nil); got.Code == http.StatusOK {
			t.Errorf("%q was served and should not have been", name)
		}
	}
}

// offered is the catalog a test server shows: one harness that needs nobody's
// account, which is what a form may offer.
func offered() []catalog.Entry {
	return []catalog.Entry{{Name: "fixture", Program: "sh", Needs: harness.NeedsNothing, Ready: true}}
}
