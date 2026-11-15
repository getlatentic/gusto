package web

import (
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/registry"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/internal/usage"
	"github.com/getlatentic/gusto/pkg/task"
)

func recordTask(t *testing.T, here *site, record tasks.Record) {
	t.Helper()
	if err := task.Dir(filepath.Join(here.index.Root(), record.ID)).Make(); err != nil {
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
	recordTask(t, here, tasks.Record{ID: "turn-1", Harness: "fixture", Asked: alice, Team: "red", Repo: "acme/web", Instance: id, Thread: "1"})
	recordTask(t, here, tasks.Record{ID: "lone-task", Harness: "fixture", Asked: "token:ci"})

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

// A chat's title is the folder it works in, which says nothing of who answers: usage names its agent by the agent type, and keeps a
// role's agent by the role.
func TestUsageNamesAChatsAgentByItsTypeAndNotByItsFolder(t *testing.T) {
	here := open(t)
	chat, err := here.server.instances.Start("development", instance.Instance{Title: "Development", Harness: "fixture", Who: alice})
	if err != nil {
		t.Fatal(err)
	}
	role, err := here.server.instances.Start("analyst", instance.Instance{Title: "Finance analyst", Setup: "finance-analyst", Harness: "fixture", Who: alice})
	if err != nil {
		t.Fatal(err)
	}
	recordTask(t, here, tasks.Record{ID: "chat-turn", Harness: "fixture", Asked: alice, Instance: chat.ID, Thread: "1"})
	recordTask(t, here, tasks.Record{ID: "role-turn", Harness: "fixture", Asked: alice, Instance: role.ID, Thread: "1"})
	if who, _ := here.server.UsageWho("chat-turn"); who.Agent != here.server.engineName("fixture") {
		t.Errorf("a chat in the folder Development has the agent %q, not its agent type %q", who.Agent, here.server.engineName("fixture"))
	}
	if who, _ := here.server.UsageWho("role-turn"); who.Agent != "Finance analyst" {
		t.Errorf("a role's agent is %q", who.Agent)
	}
}

// Each total names the period it counts, so it can be read beside the month's figures on the Overview.
func TestUsageTotalsNameTheirPeriod(t *testing.T) {
	for query, want := range map[string]string{
		"":                              "spent, all time",
		"from=2026-10-29":               "spent, since 29 Oct 2026",
		"to=2026-11-03":                 "spent, up to 3 Nov 2026",
		"from=2026-11-01&to=2026-11-03": "spent, 1 Nov 2026 to 3 Nov 2026",
		"from=nonsense":                 "spent, all time",
	} {
		values, _ := url.ParseQuery(query)
		if _, got := usageSpent(usage.Totals{Calls: 2, USD: 0.1726}, usagePeriod(values)); got != want {
			t.Errorf("%q is captioned %q, want %q", query, got, want)
		}
	}
	if _, got := usageSpent(usage.Totals{Calls: 1, Unmeasured: 1}, "all time"); got != "no price set for these models, all time" {
		t.Errorf("an unpriced total is captioned %q", got)
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
