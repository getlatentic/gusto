package web

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/registry"
	"github.com/getlatentic/gusto/internal/usage"
	"github.com/getlatentic/gusto/pkg/job"
)

func recordTask(t *testing.T, here *site, record jobs.Record) {
	t.Helper()
	if err := job.Dir(filepath.Join(here.index.Root(), record.ID)).Make(); err != nil {
		t.Fatal(err)
	}
	record.Queued = time.Now().Unix()
	if err := here.index.Add(record); err != nil {
		t.Fatal(err)
	}
}

func TestWhoMadeATurnIsTheInstanceAndThreadAndWhoMadeATaskIsTheTaskItself(t *testing.T) {
	here := open(t)
	where := startedInstance(t, here.as(t, alice))
	id := where[len("/instances/"):]
	recordTask(t, here, jobs.Record{ID: "turn-1", Harness: "fixture", Asked: alice, Team: "red", Repo: "acme/web", Instance: id, Thread: "1"})
	recordTask(t, here, jobs.Record{ID: "lone-task", Harness: "fixture", Asked: "token:ci"})

	turn, found := here.server.UsageWho("turn-1")
	if !found || turn.Session != id+":1" || turn.Person != alice || turn.Team != "red" || turn.Project != "acme/web" || turn.Agent == "" {
		t.Errorf("a turn was %+v", turn)
	}
	lone, _ := here.server.UsageWho("lone-task")
	if lone != (usage.Who{Session: "lone-task", Person: "token:ci", Engine: here.server.engineName("fixture")}) {
		t.Errorf("a task in no conversation was %+v", lone)
	}
	if _, found := here.server.UsageWho("no-such-task"); found {
		t.Error("a task that is not held has no one who made it")
	}
}

func TestTheProviderOfAModelIsTheRegistrysAndOtherwiseTheKeyThatServedIt(t *testing.T) {
	here := open(t)
	for _, c := range []struct{ model, credential, want string }{
		{"claude-sonnet", "team-key", "team-key"},
		{"openai/gpt-5", "", "openai"},
		{"gpt-5", "", ""},
	} {
		if got := here.server.UsageProvider(c.model, c.credential); got != c.want {
			t.Errorf("%q served by %q was attributed to %q, not %q", c.model, c.credential, got, c.want)
		}
	}
}

func TestAModelTheRegistryListsIsAnsweredByItsProvider(t *testing.T) {
	here, _, _ := registered(t, registry.Rules{})
	for _, c := range []struct{ model, credential, want string }{
		{"platform/qwen", "", "platform"},
		{"qwen", "platform", "platform"},
		{"qwen", "", ""},
		{"unlisted", "platform", "platform"},
	} {
		if got := here.server.UsageProvider(c.model, c.credential); got != c.want {
			t.Errorf("%q served by %q was attributed to %q, not %q", c.model, c.credential, got, c.want)
		}
	}
}
