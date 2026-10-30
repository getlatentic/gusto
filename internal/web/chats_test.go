package web

import (
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/quota"
)

// Chat is the home page: the address of the site lands on it, and the list of chats is the side list it carries.
func TestTheHomePageIsChatAndItsSideListLeadsBackIntoAChat(t *testing.T) {
	site := open(t)
	site.signIn(t)
	where := startedThread(t, site)
	site.ask(t, "POST", where+"/say", url.Values{"said": {"hello"}})
	project := strings.TrimPrefix(where[:strings.Index(where, "/threads/")], "/instances/")
	landed := site.ask(t, "GET", "/", nil)
	if landed.Code != http.StatusSeeOther || landed.Header().Get("Location") != "/chat" {
		t.Fatalf("the home address answered %d to %q", landed.Code, landed.Header().Get("Location"))
	}
	if body := site.ask(t, "GET", "/chat", nil).Body.String(); !strings.Contains(body, `href="/chat?at=`+project+`"`) {
		t.Errorf("the chat page's side list does not carry the project of %s", where)
	}
}

// Removing a harness takes it off the list and leaves its jobs in the record.
func TestARemovedHarnessLeavesTheListAndKeepsItsJobs(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id := aTask(t, site, url.Values{"brief": {"count the files"}, "gate": {"true"}})
	site.wait(t, id)
	record, _ := site.index.Get(id)
	answer := site.ask(t, "POST", "/instances/"+record.Instance+"/remove", nil)
	if answer.Code != http.StatusSeeOther || answer.Header().Get("Location") != "/" {
		t.Fatalf("removing answered %d to %q", answer.Code, answer.Header().Get("Location"))
	}
	if home := site.ask(t, "GET", "/chat", nil).Body.String(); strings.Contains(home, record.Instance) {
		t.Error("the removed harness is still listed")
	}
	if page := site.ask(t, "GET", "/jobs/"+id, nil); page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "removed") {
		t.Errorf("its job no longer reads as the record of a removed harness: %d", page.Code)
	}
}

// The counts across the record are its filters, and a filter keeps to itself
// what it lets through.
func TestTheRecordFiltersByState(t *testing.T) {
	site := open(t)
	site.signIn(t)
	site.leave(t, "went-well", leftBehind{state: jobs.Passed, out: map[string]string{}, gate: passed})
	site.leave(t, "went-badly", leftBehind{state: jobs.Failed, out: map[string]string{}, gate: passed})
	all := site.ask(t, "GET", "/jobs", nil).Body.String()
	if !strings.Contains(all, `href="/jobs?state=failed"`) {
		t.Fatal("the failed count is not a filter")
	}
	failed := site.ask(t, "GET", "/jobs?state=failed", nil).Body.String()
	if !strings.Contains(failed, "/jobs/went-badly") || strings.Contains(failed, "/jobs/went-well") {
		t.Error("filtering by failed did not keep only the failure")
	}
}

// A turn in a conversation names no gate. Its page has no gate to report
// missing, and it leads back to the conversation it was part of.
func TestATurnsPageLeadsBackToItsThread(t *testing.T) {
	site := open(t)
	site.signIn(t)
	where := startedThread(t, site)
	said := site.ask(t, "POST", where+"/say", url.Values{"said": {"hello"}})
	id := strings.TrimPrefix(said.Header().Get("Location"), where+"#")
	site.wait(t, id)
	page := site.ask(t, "GET", "/jobs/"+id, nil).Body.String()
	if !strings.Contains(page, `href="`+where+`"`) {
		t.Error("the turn's page does not lead back to its thread")
	}
	if strings.Contains(page, "never answered") || strings.Contains(page, "before its gate") {
		t.Error("a turn with no gate is shown as missing a verdict")
	}
}

// wait holds until a job has run and the record says it finished, because the
// runner signals before the queue writes down how the job ended.
func (s *site) wait(t *testing.T, id string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if record, found := s.index.Get(id); found && record.State.Done() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never finished", id)
}

// A reply is drawn as Markdown, and nothing in it acts in the reader's
// browser: its HTML reads as text, a javascript: link is dropped, and an image
// is named rather than fetched.
func TestAReplyIsMarkdownThatCannotActInTheBrowser(t *testing.T) {
	drawn := string(markdown("Fixed `adder.sh`.\n\n<img src=x onerror=alert(1)>\n\n" +
		"[click](javascript:alert(2)) ![pixel](https://example.com/p.png)"))
	for _, want := range []string{"<code>adder.sh</code>", "&lt;img src=x onerror=alert(1)&gt;", "[image: https://example.com/p.png]"} {
		if !strings.Contains(drawn, want) {
			t.Errorf("want %q in %s", want, drawn)
		}
	}
	for _, never := range []string{"<img", "javascript:"} {
		if strings.Contains(drawn, never) {
			t.Errorf("%q reached the page: %s", never, drawn)
		}
	}
}

// The presets page was at /new, and an old link to it still arrives there.
func TestTheOldAddressOfThePresetsPageLeadsToThePresetsPage(t *testing.T) {
	site := open(t)
	site.signIn(t)
	old := site.ask(t, "GET", "/new", nil)
	if old.Code != http.StatusMovedPermanently || old.Header().Get("Location") != "/presets" {
		t.Fatalf("/new answered %d to %q", old.Code, old.Header().Get("Location"))
	}
	if page := site.ask(t, "GET", "/presets", nil); page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Presets") {
		t.Errorf("/presets answered %d", page.Code)
	}
}

// A message that was refused says why above the box it was typed in, and a refusal of the members dialog stays in the dialog.
func TestARefusedMessageSaysWhyAboveTheBox(t *testing.T) {
	site := open(t)
	site.signIn(t)
	where := startedThread(t, site)
	path := filepath.Join(t.TempDir(), "quota.json")
	if err := quota.Write(path, quota.Quota{Jobs: 1, Concurrent: 1, Note: "Confirm your email address to raise this limit."}); err != nil {
		t.Fatal(err)
	}
	site.index.UseQuota(quota.Open(path))

	site.ask(t, "POST", where+"/say", url.Values{"said": {"the one allowed"}})
	said := site.ask(t, "POST", where+"/say", url.Values{"said": {"one more"}})
	back := said.Header().Get("Location")
	page := site.ask(t, "GET", back, nil).Body.String()
	shown := regexp.MustCompile(`<p [^>]*id="unsent" role="alert">([^<]*)</p>`).FindStringSubmatch(page)
	if shown == nil || !strings.Contains(shown[1], "1 task in all") {
		t.Fatalf("the refusal from %q is not shown above the box", back)
	}
	members := site.ask(t, "GET", back+"&members=1", nil).Body.String()
	if !strings.Contains(members, `id="unsent" hidden>`) {
		t.Error("a refusal of the members dialog is shown above the box as well")
	}
}
