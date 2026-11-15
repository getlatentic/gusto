package web

import (
	"net/url"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/tasks"
)

// The Failed view lists a conversation whose latest turn failed or was interrupted, and not one that failed and then went on.
func TestTheFailedViewListsOnlyConversationsWhoseLatestTurnFailed(t *testing.T) {
	here, _ := withUsage(t, aCall("broke", 0, "broke", alice, 0), aCall("stopped", 0, "stopped", alice, 1),
		aCall("fine", 0, "fine", bob, 2), aCall("turn-1", 0, "room:1", alice, 3), aCall("turn-2", 0, "room:1", alice, 4))
	recordTask(t, here.site, tasks.Record{ID: "broke", Harness: "fixture", Asked: alice, State: tasks.Failed})
	recordTask(t, here.site, tasks.Record{ID: "stopped", Harness: "fixture", Asked: alice, State: tasks.Interrupted})
	recordTask(t, here.site, tasks.Record{ID: "fine", Harness: "fixture", Asked: bob, State: tasks.Passed})
	recordTask(t, here.site, tasks.Record{ID: "turn-1", Harness: "fixture", Asked: alice, Instance: "room", Thread: "1", State: tasks.Failed})
	recordTask(t, here.site, tasks.Record{ID: "turn-2", Harness: "fixture", Asked: alice, Instance: "room", Thread: "1", State: tasks.Passed})

	all := usagePageOf(t, here, "")
	if states := all.Usage.States; len(states) != 2 || states[0].Href != "/admin/usage" || !states[0].Current || states[1].Href != "/admin/usage?state=failed" || states[1].Count != "2 this week" {
		t.Errorf("the views are not offered with the week's count: %+v", states)
	}
	failed := usagePageOf(t, here, "?state=failed")
	if got := strings.Join(failed.sessionsOf(), " "); got != "stopped broke" {
		t.Errorf("the failed view lists %q, not only the conversations whose latest turn failed", got)
	}
	if failed.Usage.Values["state"][0] != "failed" || failed.Usage.Export != "/api/usage.csv?state=failed" {
		t.Errorf("the filters and the download do not keep the failed view: %v %q", failed.Usage.Values, failed.Usage.Export)
	}

	narrowed := usagePageOf(t, here, "?state=failed&q=stopped")
	if got := strings.Join(narrowed.sessionsOf(), " "); got != "stopped" || narrowed.Usage.Clear != "/admin/usage?state=failed" {
		t.Errorf("a filter does not narrow the failed view (%q), or its Clear leaves the view (%q)", got, narrowed.Usage.Clear)
	}

	csv := here.ask(t, "GET", "/api/usage.csv?state=failed", nil).Body.String()
	if !strings.Contains(csv, ",broke,") || !strings.Contains(csv, ",stopped,") || strings.Contains(csv, ",fine,") || strings.Contains(csv, ",room:1,") {
		t.Errorf("the download of the failed view holds:\n%s", csv)
	}
	if got := here.ask(t, "GET", "/app/api/admin/usage?state=sideways", nil); got.Code != 400 {
		t.Errorf("an unknown state answered %d", got.Code)
	}
}

func TestTheFailedViewSaysWhenNothingFailed(t *testing.T) {
	here, _ := withUsage(t, aCall("fine", 0, "fine", bob, 2))
	recordTask(t, here.site, tasks.Record{ID: "fine", Harness: "fixture", Asked: bob, State: tasks.Passed})
	if page := usagePageOf(t, here, "?state=failed"); len(page.Usage.Rows) != 0 || page.Usage.Filtered {
		t.Errorf("an empty failed view: %+v", page.Usage)
	}
}

// A row names who asked and, apart, whose agent answered: a room's agent is paid for by its owner, whoever mentions it.
func TestARowSaysWhoAskedAndWhoseAgentAnswered(t *testing.T) {
	call := aCall("turn", 0, "room:1", alice, 0)
	call.Agent = "Nova"
	here, _ := withUsage(t, call)
	recordTask(t, here.site, tasks.Record{ID: "turn", Harness: "fixture", Asked: bob, PaidBy: alice, Instance: "room", Thread: "1", State: tasks.Passed})
	page := usagePageOf(t, here, "")
	owner, asker := here.server.personName(alice), here.server.personName(bob)
	var headings []string
	for _, column := range page.Usage.Columns {
		headings = append(headings, column.Label)
	}
	if got := strings.Join(headings, ", "); got != "Last request, Conversation, Asked by, Agent · owner, Cost, Model" {
		t.Errorf("the headings are %s", got)
	}
	if len(page.Usage.Rows) != 1 || page.Usage.Rows[0].Asked != asker || page.Usage.Rows[0].Agent != "Nova" || page.Usage.Rows[0].Owner != owner {
		t.Errorf("the row does not say %s asked of %s's Nova: %+v", asker, owner, page.Usage.Rows)
	}
}

// Another page links to the usage of one person or one agent type, and the type may be named by its id.
func TestALinkToAPersonOrAnAgentTypeChoosesItsFilter(t *testing.T) {
	other := aCall("task-c", 0, "codex-session", alice, 5)
	other.Engine = "Codex"
	here, _ := withUsage(t, append(bothPeople(), other)...)
	here.server.UseLiveCatalog(func() []catalog.Entry {
		return []catalog.Entry{{Name: "claude", Title: "Claude Code"}, {Name: "codex", Title: "Codex"}}
	})

	byType := usagePageOf(t, here, "?agent_type=codex")
	if got := strings.Join(byType.sessionsOf(), " "); got != "codex-session" {
		t.Errorf("an agent type by its id narrows the table to %q", got)
	}
	if byType.Usage.Values["agent_type"][0] != "Codex" || !byType.Usage.Filtered {
		t.Errorf("an agent type by its id is not shown chosen: %v", byType.Usage.Values)
	}

	byPerson := usagePageOf(t, here, "?person="+url.QueryEscape(bob))
	if got := strings.Join(byPerson.sessionsOf(), " "); got != "bob-session" {
		t.Errorf("a person's link narrows the table to %q", got)
	}
	if byPerson.Usage.Values["person"][0] != bob {
		t.Errorf("a person's link is not shown chosen: %v", byPerson.Usage.Values)
	}
}
