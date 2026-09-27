package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/getlatentic/haas/internal/catalog"
	"github.com/getlatentic/haas/internal/harness"

	"github.com/getlatentic/haas/internal/instance"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/signing"
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
	return openAt(t, t.TempDir())
}

// openAt is a server over a jobs directory that may already hold jobs, which is
// how a test shows that what ran before still reads.
func openAt(t *testing.T, root string) *site {
	t.Helper()
	return openKeyed(t, root, signing.Keyring{})
}

// openKeyed is openAt with the keyring the job page names signers from.
func openKeyed(t *testing.T, root string, keyring signing.Keyring) *site {
	t.Helper()
	index, err := jobs.Open(root)
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
	instances, err := instance.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(index, instances, queue, password, &Identity{}, offered(), []string{"platform/test-model"})
	if err != nil {
		t.Fatal(err)
	}
	server.UseKeyring(keyring)
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
	posted := site.ask(t, "POST", "/instances/anything/tasks", url.Values{"brief": {"x"}, "gate": {"true"}})
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

// The front door is the catalog: what this deployment can start, and picking
// one is the only thing on it to do.
func TestSomebodySignedInSeesWhatTheyCanStart(t *testing.T) {
	site := open(t)
	site.signIn(t)
	answer := site.ask(t, "GET", "/", nil)
	body := answer.Body.String()
	if answer.Code != http.StatusOK {
		t.Fatalf("code %d", answer.Code)
	}
	for _, want := range []string{"fixture", "/instances/new?harness=fixture"} {
		if !strings.Contains(body, want) {
			t.Errorf("the catalog does not offer %s", want)
		}
	}
}

func TestTheStreamReplaysFromTheStartAndEndsWithTheVerdict(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id := aTask(t, site, url.Values{"brief": {"watch me"}, "gate": {"true"}})
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
	id := aTask(t, site, url.Values{"brief": {"files"}, "gate": {"true"}})
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
	return []catalog.Entry{{
		Name: "fixture", What: "what a fixture is", Program: "sh",
		Needs: harness.NeedsNothing, Ready: true,
	}}
}

// The page and the fragment must agree: a turn swapped in should be the same
// markup the whole page would have rendered for it.
func TestAFinishedTurnIsSwappedInRatherThanReloaded(t *testing.T) {
	site := open(t)
	site.signIn(t)
	where := startedThread(t, site)
	if said := site.ask(t, "POST", where+"/say", url.Values{"said": {"hello there"}}); said.Code != http.StatusSeeOther {
		t.Fatalf("saying something gave %d", said.Code)
	}
	select {
	case <-site.ran:
	case <-time.After(5 * time.Second):
		t.Fatal("the turn never ran")
	}

	whole := site.ask(t, "GET", where, nil).Body.String()
	fragment := site.ask(t, "GET", where+"/turns", nil).Body.String()
	if !strings.Contains(fragment, "hello there") {
		t.Fatalf("the fragment does not carry the turn:\n%s", fragment)
	}
	if !strings.Contains(whole, "hello there") {
		t.Fatal("the whole page does not carry the turn")
	}
	if strings.Contains(fragment, "<html") || strings.Contains(fragment, "</body>") {
		t.Fatal("the fragment is a whole page, which would nest one inside the other when swapped")
	}
}

func TestWhatIsVendoredIsServedFromTheBinary(t *testing.T) {
	site := open(t)
	answer := site.ask(t, "GET", "/static/htmx.min.js", nil)
	if answer.Code != http.StatusOK {
		t.Fatalf("code %d", answer.Code)
	}
	if answer.Body.Len() < 10_000 {
		t.Fatalf("%d bytes came back, which is not htmx", answer.Body.Len())
	}
	// It is in the binary, so it needs no sign-in: it is the same bytes for
	// everybody and a page cannot render without it.
	if strings.Contains(answer.Body.String(), "cdn.") {
		t.Fatal("the vendored copy reaches a content delivery network")
	}
}

// startedThread is a harness started from the catalog, and the conversation
// that starting it opens.
func startedThread(t *testing.T, site *site) string {
	t.Helper()
	started := site.ask(t, "POST", "/instances", url.Values{"harness": {"fixture"}})
	where := started.Header().Get("Location")
	if !strings.Contains(where, "/threads/") {
		t.Fatalf("starting a harness did not open a conversation: %d to %q", started.Code, where)
	}
	return where
}

// startedInstance is the instance a started conversation belongs to.
func startedInstance(t *testing.T, site *site) string {
	t.Helper()
	thread := startedThread(t, site)
	return thread[:strings.Index(thread, "/threads/")]
}

// Starting a harness is not reading about one. Whoever picked it wants to be
// talking to it, so the first conversation is already open.
func TestStartingAHarnessLandsInAConversationWithIt(t *testing.T) {
	site := open(t)
	site.signIn(t)
	where := startedThread(t, site)
	body := site.ask(t, "GET", where, nil).Body.String()
	if !strings.Contains(body, `name="said"`) {
		t.Error("the page you land on has nowhere to say anything")
	}
}

// Two threads on one instance are two conversations, and what is said in one
// does not appear in the other.
func TestTwoThreadsOnOneInstanceAreSeparateConversations(t *testing.T) {
	site := open(t)
	site.signIn(t)
	first := startedThread(t, site)
	where := first[:strings.Index(first, "/threads/")]
	opened := site.ask(t, "POST", where+"/threads", url.Values{"title": {"the second thing"}})
	second := opened.Header().Get("Location")
	if second == first {
		t.Fatalf("both threads are at %s", second)
	}
	if said := site.ask(t, "POST", first+"/say", url.Values{"said": {"only in the first"}}); said.Code != http.StatusSeeOther {
		t.Fatalf("saying something gave %d", said.Code)
	}
	select {
	case <-site.ran:
	case <-time.After(5 * time.Second):
		t.Fatal("the turn never ran")
	}
	if body := site.ask(t, "GET", first, nil).Body.String(); !strings.Contains(body, "only in the first") {
		t.Error("the first thread does not carry what was said in it")
	}
	if body := site.ask(t, "GET", second, nil).Body.String(); strings.Contains(body, "only in the first") {
		t.Error("the second thread carries what was said in the first")
	}
}

// The catalog is the front door, so every page has to name a way back to it.
// Without one the app looks like it is only about jobs.
func TestEveryPageOffersTheWayBackToTheCatalog(t *testing.T) {
	site := open(t)
	site.signIn(t)
	for _, page := range []string{"/", "/jobs", "/spend"} {
		body := site.ask(t, "GET", page, nil).Body.String()
		if !strings.Contains(body, `href="/"`) || !strings.Contains(body, "Harnesses") {
			t.Errorf("%s does not offer the way to the catalog", page)
		}
	}
}

// A task is work done on the instance's own folder. Not every task starts
// from a repository, and one that does not still has somewhere to work.
func TestATaskRunsInTheInstancesOwnFolder(t *testing.T) {
	site := open(t)
	site.signIn(t)
	where := startedInstance(t, site)
	answer := site.ask(t, "POST", where+"/tasks", url.Values{
		"brief": {"count the files"}, "gate": {"true"},
	})
	if answer.Code != http.StatusSeeOther {
		t.Fatalf("the task was refused: %d %q", answer.Code, answer.Body.String())
	}
	id := strings.TrimPrefix(answer.Header().Get("Location"), "/jobs/")
	record, found := site.index.Get(id)
	if !found {
		t.Fatalf("no job called %q", id)
	}
	if record.Instance != strings.TrimPrefix(where, "/instances/") {
		t.Errorf("the task does not name its instance: %+v", record)
	}
	if record.Thread != "" {
		t.Errorf("a task is not said to a thread, but it names %q", record.Thread)
	}
	// It belongs to the instance's page, which is where it was asked for.
	if body := site.ask(t, "GET", where, nil).Body.String(); !strings.Contains(body, "count the files") {
		t.Error("the instance does not list the task run on it")
	}
}

// A task needs somewhere to work and a way to know it worked, and nothing else.
func TestATaskWithNoGateIsRefused(t *testing.T) {
	site := open(t)
	site.signIn(t)
	where := startedInstance(t, site)
	for _, gate := range []string{"", "  "} {
		answer := site.ask(t, "POST", where+"/tasks", url.Values{"brief": {"do a thing"}, "gate": {gate}})
		if got := answer.Header().Get("Location"); !strings.Contains(got, "refused") {
			t.Errorf("a task with gate %q was accepted: %d to %q", gate, answer.Code, got)
		}
	}
	if len(site.index.List()) != 0 {
		t.Fatal("a refused task left a directory behind")
	}
}

// With a provider the shared password is the way in when the provider is not,
// so it is reachable and it is not the second thing on the sign-in page.
func TestTheSharedPasswordIsNotOfferedBesideAProvider(t *testing.T) {
	index, err := jobs.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	instances, err := instance.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(index, instances, nil, password, &Identity{provider: &oidc.Provider{}}, offered(), nil)
	if err != nil {
		t.Fatal(err)
	}
	site := &site{handler: server.Handler(), index: index}

	plain := site.ask(t, "GET", "/login", nil).Body.String()
	if strings.Contains(plain, `name="password"`) {
		t.Error("the sign-in page offers the shared password beside the provider")
	}
	if !strings.Contains(plain, "/login?shared") {
		t.Error("the shared password cannot be reached at all")
	}
	asked := site.ask(t, "GET", "/login?shared", nil).Body.String()
	if !strings.Contains(asked, `name="password"`) {
		t.Error("the shared password is not offered to somebody who asked for it")
	}
	// A wrong one keeps it open, or there is nowhere to try again.
	wrong := site.ask(t, "GET", "/login?wrong=1", nil).Body.String()
	if !strings.Contains(wrong, `name="password"`) {
		t.Error("a wrong password left nowhere to try again")
	}
}

// A cost is a pointer, because a cost of nothing and no cost reported are
// different answers. Handed to printf it prints an address, which is what the
// page showed the first time a harness reported one.
func TestACostIsShownAsMoneyAndNotAsAPointer(t *testing.T) {
	shown, ok := helpers["money"].(func(*float64) string)
	if !ok {
		t.Fatal("there is no money helper")
	}
	amount := 1.3161
	if got := shown(&amount); got != "$1.3161" {
		t.Errorf("a cost of %v is shown as %q", amount, got)
	}
	nothing := 0.0
	if got := shown(&nothing); got != "$0.0000" {
		t.Errorf("a cost of nothing is shown as %q, and it was reported", got)
	}
	if got := shown(nil); got != "" {
		t.Errorf("a cost nobody reported is shown as %q", got)
	}
}

// A catalog of names is not a choice. Each card says what the harness is, and
// carries the action that starts it.
func TestTheCatalogSaysWhatEachHarnessIsAndOffersToStartIt(t *testing.T) {
	site := open(t)
	site.signIn(t)
	body := site.ask(t, "GET", "/", nil).Body.String()
	if !strings.Contains(body, `action="/instances"`) || !strings.Contains(body, ">Start<") {
		t.Error("the catalog does not offer to start anything")
	}
	if !strings.Contains(body, "what a fixture is") {
		t.Error("the catalog does not say what the harness is")
	}
}
