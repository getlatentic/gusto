package web

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/pkg/job"
)

// holding is work that runs until it is told to stop, and then writes what a run that was stopped leaves.
func holding(t *testing.T) (started chan string) {
	t.Helper()
	started = make(chan string, 8)
	testWork = func(ctx context.Context, dir job.Dir) (int, error) {
		os.MkdirAll(dir.Out(), 0o755)
		os.WriteFile(dir.OutFile("transcript.log"), []byte("working on it"), 0o644)
		started <- dir.Path()
		<-ctx.Done()
		return -1, ctx.Err()
	}
	t.Cleanup(func() { testWork = nil })
	return started
}

func TestAPersonStopsTheirRunningTurnByTheAPIAndItEndsInterruptedByThem(t *testing.T) {
	started := holding(t)
	here := withTokens(t)
	_, thread := started2(t, here, alice)
	ada := signInADevice(t, here, alice)
	_, said := here.say("/api"+thread+"/say", ada, "a long piece of work", "go")
	id := said["job"].(string)
	<-started

	stopped := here.postAs(ada, "/api/jobs/"+id+"/stop", "")
	if stopped.Code != http.StatusOK || !strings.Contains(stopped.Body.String(), `"wasRunning":true`) {
		t.Fatalf("stopping gave %d %s", stopped.Code, stopped.Body)
	}
	record := turnDone(t, here, id)
	if record.State != jobs.Interrupted || record.StoppedBy != alice {
		t.Fatalf("the stopped turn is %+v", record)
	}
	if note, err := os.ReadFile(record.Dir(here.index.Root()).Path() + "/stop.json"); err != nil || !strings.Contains(string(note), alice) {
		t.Errorf("who stopped it was not written where the run reads it: %q %v", note, err)
	}
	if again := here.postAs(ada, "/api/jobs/"+id+"/stop", ""); again.Code != http.StatusConflict {
		t.Errorf("stopping a turn that was over gave %d", again.Code)
	}
}

func TestAQueuedTurnThatIsStoppedNeverRunsAndSomebodyElsesCannotBeStopped(t *testing.T) {
	started := holding(t)
	here := withTokens(t)
	_, thread := started2(t, here, alice)
	ada := signInADevice(t, here, alice)
	_, first := here.say("/api"+thread+"/say", ada, "holds the worker", "one")
	<-started
	_, second := here.say("/api"+thread+"/say", ada, "waits behind it", "two")
	id := second["job"].(string)

	if got := here.postAs(signInADevice(t, here, bob), "/api/jobs/"+id+"/stop", ""); got.Code != http.StatusNotFound {
		t.Errorf("bob stopped alice's turn: %d", got.Code)
	}
	if got := here.postAs(here.secrets["everyone"], "/api/jobs/"+id+"/stop", ""); got.Code == http.StatusOK {
		t.Errorf("an organization's token stopped a person's turn")
	}
	if got := here.postAs(ada, "/api/jobs/"+id+"/stop", ""); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"wasRunning":false`) {
		t.Fatalf("stopping a queued turn gave %d %s", got.Code, got.Body)
	}
	record, _ := here.index.Get(id)
	if record.State != jobs.Interrupted || record.StoppedBy != alice {
		t.Errorf("the queued turn is %+v", record)
	}
	here.postAs(ada, "/api/jobs/"+first["job"].(string)+"/stop", "")
	time.Sleep(200 * time.Millisecond)
	select {
	case path := <-started:
		t.Errorf("the turn that was stopped while queued ran: %s", path)
	default:
	}
}

func TestThePageStopsATurnAndShowsWhoStoppedIt(t *testing.T) {
	started := holding(t)
	here := withTokens(t)
	ada := here.as(t, alice)
	_, thread := started2(t, here, alice)
	ada.ask(t, "POST", thread+"/say", url.Values{"said": {"a long piece of work"}})
	<-started
	page := ada.ask(t, "GET", thread, nil).Body.String()
	if !strings.Contains(page, `/stop"`) || !strings.Contains(page, ">Stop<") {
		t.Fatalf("a running turn has no stop button:\n%s", page)
	}
	id := regexpFirst(t, page, `/jobs/([0-9a-z-]+)/stop`)
	if got := here.as(t, bob).ask(t, "POST", "/jobs/"+id+"/stop", nil); got.Code != http.StatusNotFound {
		t.Errorf("bob stopped alice's turn from the page: %d", got.Code)
	}
	got := ada.ask(t, "POST", "/jobs/"+id+"/stop", url.Values{"back": {thread}})
	if got.Code != http.StatusSeeOther || got.Header().Get("Location") != thread {
		t.Fatalf("the button answered %d to %q", got.Code, got.Header().Get("Location"))
	}
	turnDone(t, here, id)
	if after := ada.ask(t, "GET", thread, nil).Body.String(); !strings.Contains(after, "Stopped by "+alice) {
		t.Errorf("the page does not say who stopped it:\n%s", after)
	}
	if got := ada.ask(t, "POST", "/jobs/"+id+"/stop", url.Values{"back": {"//evil.example"}}); got.Header().Get("Location") != "/jobs/"+id {
		t.Errorf("the button went to %q", got.Header().Get("Location"))
	}
}

// started2 starts an agent for a person and gives its path and its first thread's.
func started2(t *testing.T, here *tokened, who string) (instance, thread string) {
	t.Helper()
	return started(t, here.as(t, who))
}

// A turn whose model the free credit refused says so, and what to do: add the organization's own key, which an admin is linked to.
func TestATurnTheCreditStoppedSaysHowToCarryOn(t *testing.T) {
	testWork = func(ctx context.Context, dir job.Dir) (int, error) {
		os.MkdirAll(dir.Out(), 0o755)
		line := `{"path":"/v1/chat/completions","status":402,"atUnix":` + strconv.FormatInt(time.Now().Unix(), 10) +
			`,"model":"m","inputTokens":0,"outputTokens":0,"usd":0,"refused":{"cap":"credit","limitUsd":5,"spentUsd":5.01,"reason":"the free credit is used"}}`
		os.WriteFile(dir.OutFile("proxy.jsonl"), []byte(line+"\n"), 0o644)
		return 1, nil
	}
	t.Cleanup(func() { testWork = nil })
	here := withTokens(t)
	ada := here.as(t, alice)
	_, thread := started2(t, here, alice)
	_, said := here.say("/api"+thread+"/say", signInADevice(t, here, alice), "one more", "go")
	turnDone(t, here, said["job"].(string))
	page := ada.ask(t, "GET", thread, nil).Body.String()
	if !strings.Contains(page, "The free credit for the models gusto provides is used: $5.01 of $5.00.") ||
		!(strings.Contains(page, `href="/admin/models">Add your organization&#39;s own model key</a>`) || strings.Contains(page, "Ask an admin of your organization")) {
		t.Errorf("the turn does not say the credit stopped it and what to do:\n%s", page)
	}
}

// A model the credit cannot count is refused for that, not because the credit is used, and the turn says the real reason.
func TestATurnRefusedForAnUnpricedModelSaysWhyAndNotThatTheCreditIsUsed(t *testing.T) {
	testWork = func(ctx context.Context, dir job.Dir) (int, error) {
		os.MkdirAll(dir.Out(), 0o755)
		line := `{"path":"/v1/chat/completions","status":402,"atUnix":` + strconv.FormatInt(time.Now().Unix(), 10) +
			`,"model":"m","inputTokens":0,"outputTokens":0,"usd":0,"refused":{"cap":"credit","limitUsd":5,"spentUsd":0,"reason":"m has no price in the spend policy, so the credit cap cannot count it"}}`
		os.WriteFile(dir.OutFile("proxy.jsonl"), []byte(line+"\n"), 0o644)
		return 1, nil
	}
	t.Cleanup(func() { testWork = nil })
	here := withTokens(t)
	ada := here.as(t, alice)
	_, thread := started2(t, here, alice)
	_, said := here.say("/api"+thread+"/say", signInADevice(t, here, alice), "one more", "go")
	turnDone(t, here, said["job"].(string))
	page := ada.ask(t, "GET", thread, nil).Body.String()
	if strings.Contains(page, "free credit for the models gusto provides is used") || !strings.Contains(page, "The model was not asked: m has no price in the spend policy") {
		t.Errorf("the turn does not give the real reason its model was refused")
	}
}
