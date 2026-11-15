package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/crew"
	"github.com/getlatentic/gusto/pkg/harness"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/preset"
	"github.com/getlatentic/gusto/internal/setup"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/task"
)

const password = "a-shared-password"

type site struct {
	server  *Server
	handler http.Handler
	index   *tasks.Index
	cookie  *http.Cookie
	// extra is the cookies besides the session's that the browser sends, such as the workspace it chose.
	extra []*http.Cookie
	ran   chan string
}

// testWork replaces what the test server's queue does with a task, for the test that sets it.
var testWork tasks.StoppableWork

// testWorkers is how many tasks the test server runs at once, for the test that sets it; zero is one.
var testWorkers int

func open(t *testing.T) *site {
	t.Helper()
	return openAt(t, t.TempDir())
}

// openAt is a server over a tasks directory that may already hold tasks, which is
// how a test shows that what ran before still reads.
func openAt(t *testing.T, root string) *site {
	t.Helper()
	return openKeyed(t, root, signing.Keyring{})
}

// openKeyed is openAt with the keyring the task page names signers from.
func openKeyed(t *testing.T, root string, keyring signing.Keyring) *site {
	t.Helper()
	return build(t, root, keyring, offered())
}

// openOffering is a server whose catalog is the entries given.
func openOffering(t *testing.T, entries []catalog.Entry) *site {
	t.Helper()
	return build(t, t.TempDir(), signing.Keyring{}, entries)
}

func build(t *testing.T, root string, keyring signing.Keyring, entries []catalog.Entry) *site {
	t.Helper()
	return buildOffering(t, root, keyring, entries, Models([]string{"platform/test-model"}))
}

// buildOffering is a server that offers the models given, which are what an agent may think with.
func buildOffering(t *testing.T, root string, keyring signing.Keyring, entries []catalog.Entry, models Models) *site {
	t.Helper()
	index, err := tasks.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	ran := make(chan string, 4)
	work := func(_ context.Context, dir task.Dir) (int, error) {
		if err := os.MkdirAll(dir.Out(), 0o755); err != nil {
			return 0, err
		}
		if err := os.WriteFile(dir.OutFile("transcript.log"), []byte("first line\nsecond line\n"), 0o644); err != nil {
			return 0, err
		}
		ran <- dir.Path()
		return 0, nil
	}
	if testWork != nil {
		work = testWork
	}
	queue := tasks.StartStoppable(index, work, max(testWorkers, 1))
	t.Cleanup(queue.Stop)
	instances, err := instance.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server, err := New(index, instances, queue, password, &Identity{}, entries, models)
	if err != nil {
		t.Fatal(err)
	}
	server.UseKeyring(keyring)
	presets, err := preset.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server.UsePresets(presets)
	agents, err := crew.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server.UseCrew(agents)
	setups, err := setup.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server.UseSetups(setups, setup.Servers{"memory": {Name: "memory", Command: "mcp-server-memory", Env: map[string]string{"TOKEN": "s3cret"}}})
	return &site{handler: server.Handler(), index: index, ran: ran, server: server}
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
	for _, cookie := range s.extra {
		request.AddCookie(cookie)
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

// The new agent page offers the kinds of work an agent comes ready for.
func TestSomebodySignedInSeesWhatTheyCanStart(t *testing.T) {
	site := open(t)
	site.signIn(t)
	answer := site.ask(t, "GET", "/presets", nil)
	if answer.Code != http.StatusOK || presetsOf(t, site).role(t, "education").Title == "" {
		t.Fatalf("the new agent page does not offer templates: %d", answer.Code)
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

func TestAFileOutsideTheRecordCannotBeAskedFor(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id := aTask(t, site, url.Values{"brief": {"files"}, "gate": {"true"}})
	<-site.ran
	if got := site.ask(t, "GET", "/jobs/"+id+"/files/transcript.log", nil); got.Code != http.StatusOK {
		t.Fatalf("a record file should be readable, got %d", got.Code)
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
		Needs: harness.NeedsNothing, Ready: true, Skills: true, MCP: true,
	}}
}

// The page and the snapshot of its turns must agree: a turn swapped in is the turn the whole page's data had for it.
func TestAFinishedTurnInTheSnapshotIsTheTurnThePageHas(t *testing.T) {
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
	eventually(t, 5*time.Second, "the turn to finish", func() bool { return site.allDone() })

	whole := site.conversation(t, where)
	snapshot, header := site.turnsAt(t, where)
	if len(whole.Turns) != 1 || whole.Turns[0].Said != "hello there" || !whole.Turns[0].Done {
		t.Fatalf("the page does not carry the finished turn: %+v", whole.Turns)
	}
	if fmt.Sprint(snapshot.Turns) != fmt.Sprint(whole.Turns) {
		t.Errorf("the snapshot's turns differ from the page's:\n%+v\n%+v", snapshot.Turns, whole.Turns)
	}
	if header.Get("X-Room-Settings") == "" || snapshot.Settings != header.Get("X-Room-Settings") || header.Get("X-Room-Version") != "" {
		t.Errorf("a conversation that is not a room says %q %q %q", snapshot.Settings, header.Get("X-Room-Settings"), header.Get("X-Room-Version"))
	}
	if missing := site.ask(t, "GET", "/app/api/instances/nobody/threads/1/turns", nil); missing.Code != http.StatusNotFound {
		t.Errorf("the turns of a conversation that is not there answered %d", missing.Code)
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
	page := site.ask(t, "GET", where, nil)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "/static/look.js") {
		t.Errorf("the page you land on is not the app: %d", page.Code)
	}
	if data := site.conversation(t, where); data.Path != where || data.Agent.Name == "" || data.Hint != "Message "+data.Agent.Name {
		t.Errorf("the conversation you land on has nowhere to say anything: %+v", data)
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
	if !site.conversation(t, first).shows("only in the first") {
		t.Error("the first thread does not carry what was said in it")
	}
	if snapshot, _ := site.turnsAt(t, second); len(snapshot.Turns) != 0 {
		t.Error("the second thread carries what was said in the first")
	}
}

// The workspace's agents are its members, so every page names the way to the members. The members page's way on to the roles and
// presets is checked with that page, in web/src/pages/members.
func TestEveryPageOffersTheWayToTheMembers(t *testing.T) {
	site := open(t)
	site.signIn(t)
	for _, page := range []string{"/jobs"} {
		body := site.ask(t, "GET", page, nil).Body.String()
		if !strings.Contains(body, `href="/members"`) || !strings.Contains(body, "Members") {
			t.Errorf("%s does not offer the way to the members", page)
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
func TestATaskWithNoCheckIsRefused(t *testing.T) {
	site := open(t)
	site.signIn(t)
	where := startedInstance(t, site)
	for _, check := range []string{"", "  "} {
		answer := site.ask(t, "POST", where+"/tasks", url.Values{"brief": {"do a thing"}, "gate": {check}})
		if got := answer.Header().Get("Location"); !strings.Contains(got, "refused") {
			t.Errorf("a task with check %q was accepted: %d to %q", check, answer.Code, got)
		}
	}
	if len(site.index.List()) != 0 {
		t.Fatal("a refused task left a directory behind")
	}
}

// With a provider the shared password is the way in when the provider is not,
// so it is reachable and it is not the second thing on the sign-in page.
func TestTheSharedPasswordIsNotOfferedBesideAProvider(t *testing.T) {
	index, err := tasks.Open(t.TempDir())
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

	plain := signInOf(t, site, "")
	if plain.Shared {
		t.Error("the sign-in page offers the shared password beside the provider")
	}
	// The page links to /login?shared for whoever wants the shared password, when there is a provider and no form.
	if !plain.Identity || plain.Desktop {
		t.Error("the shared password cannot be reached at all")
	}
	if asked := signInOf(t, site, "?shared"); !asked.Shared {
		t.Error("the shared password is not offered to somebody who asked for it")
	}
	// A wrong one keeps it open, or there is nowhere to try again.
	if wrong := signInOf(t, site, "?wrong=1"); !wrong.Shared || !wrong.Wrong {
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

// A blank agent is one button. The engine is there for whoever wants
// to choose it, by a name a person knows, not by its program's.
func TestABlankAgentStartsWithOneButton(t *testing.T) {
	site := open(t)
	site.signIn(t)
	if page := blankOf(t, site); !page.engine("fixture") {
		t.Error("the blank page does not offer to start anything")
	}
	started := site.ask(t, "POST", "/instances", url.Values{"title": {"Research helper"}})
	if where := started.Header().Get("Location"); !strings.Contains(where, "/threads/") {
		t.Fatalf("starting with no engine chosen went to %q", where)
	}
	if page := site.conversation(t, started.Header().Get("Location")); page.Agent.Name != "Research helper" || page.Instance.Title != "Research helper" {
		t.Errorf("the agent is not named by the name it was given: %+v", page.Agent)
	}
}

// The words of the pages a person starts an agent from never say harness; the page's own words are checked where it is built.
func TestThePagesAPersonStartsAnAgentFromNeverSayHarness(t *testing.T) {
	site := open(t)
	site.signIn(t)
	for _, path := range []string{"/presets", "/new/blank", "/instances/new?harness=fixture", "/setups", "/templates/health"} {
		var page any
		site.data(t, path, &page)
		for _, words := range wordsIn(page) {
			if strings.Contains(strings.ToLower(strings.ReplaceAll(words, "harness=", "")), "harness") {
				t.Errorf("%s says harness to a person: %q", path, words)
			}
		}
	}
}

// wordsIn is every text a page's data holds, without the names of its fields.
func wordsIn(data any) []string {
	switch held := data.(type) {
	case string:
		return []string{held}
	case []any:
		var words []string
		for _, each := range held {
			words = append(words, wordsIn(each)...)
		}
		return words
	case map[string]any:
		var words []string
		for _, each := range held {
			words = append(words, wordsIn(each)...)
		}
		return words
	}
	return nil
}

func TestThePresetsPageOffersPresetsToStartFrom(t *testing.T) {
	site := open(t)
	site.signIn(t)
	if page := presetsOf(t, site); page.role(t, "health").Blocked != "" {
		t.Errorf("/presets offers no preset to start from: %+v", page.Roles)
	}
}
