package web

import (
	"net/url"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/jobs"
)

// The Failed view lists a conversation whose latest turn failed or was interrupted, and not one that failed and then went on.
func TestTheFailedViewListsOnlyConversationsWhoseLatestTurnFailed(t *testing.T) {
	here, _ := withUsage(t, aCall("broke", 0, "broke", alice, 0), aCall("stopped", 0, "stopped", alice, 1),
		aCall("fine", 0, "fine", bob, 2), aCall("turn-1", 0, "room:1", alice, 3), aCall("turn-2", 0, "room:1", alice, 4))
	recordTask(t, here.site, jobs.Record{ID: "broke", Harness: "fixture", Asked: alice, State: jobs.Failed})
	recordTask(t, here.site, jobs.Record{ID: "stopped", Harness: "fixture", Asked: alice, State: jobs.Interrupted})
	recordTask(t, here.site, jobs.Record{ID: "fine", Harness: "fixture", Asked: bob, State: jobs.Passed})
	recordTask(t, here.site, jobs.Record{ID: "turn-1", Harness: "fixture", Asked: alice, Instance: "room", Thread: "1", State: jobs.Failed})
	recordTask(t, here.site, jobs.Record{ID: "turn-2", Harness: "fixture", Asked: alice, Instance: "room", Thread: "1", State: jobs.Passed})

	all := here.ask(t, "GET", "/admin/usage", nil).Body.String()
	if !strings.Contains(all, `href="/admin/usage" aria-current="page">All</a>`) || !strings.Contains(all, `href="/admin/usage?state=failed">Failed <span class="font-normal text-subtle">2 this week</span></a>`) {
		t.Errorf("the views are not offered with the week's count:\n%s", all)
	}
	failed := here.ask(t, "GET", "/admin/usage?state=failed", nil).Body.String()
	for _, want := range []string{"/admin/usage/broke", "/admin/usage/stopped"} {
		if !strings.Contains(failed, want) {
			t.Errorf("the failed view leaves out %s", want)
		}
	}
	for _, kept := range []string{"/admin/usage/fine", "/admin/usage/room:1"} {
		if strings.Contains(failed, kept) {
			t.Errorf("the failed view lists %s, whose latest turn did not fail", kept)
		}
	}
	if !strings.Contains(failed, `name="state" value="failed"`) || !strings.Contains(failed, "/api/usage.csv?state=failed") {
		t.Error("the filters and the download do not keep the failed view")
	}

	narrowed := here.ask(t, "GET", "/admin/usage?state=failed&q=stopped", nil).Body.String()
	if strings.Contains(narrowed, "/admin/usage/broke") || !strings.Contains(narrowed, "/admin/usage/stopped") || !strings.Contains(narrowed, `href="/admin/usage?state=failed">Clear</a>`) {
		t.Errorf("a filter does not narrow the failed view, or its Clear leaves the view:\n%s", narrowed)
	}

	csv := here.ask(t, "GET", "/api/usage.csv?state=failed", nil).Body.String()
	if !strings.Contains(csv, ",broke,") || !strings.Contains(csv, ",stopped,") || strings.Contains(csv, ",fine,") || strings.Contains(csv, ",room:1,") {
		t.Errorf("the download of the failed view holds:\n%s", csv)
	}
	if got := here.ask(t, "GET", "/admin/usage?state=sideways", nil); got.Code != 400 {
		t.Errorf("an unknown state answered %d", got.Code)
	}
}

func TestTheFailedViewSaysWhenNothingFailed(t *testing.T) {
	here, _ := withUsage(t, aCall("fine", 0, "fine", bob, 2))
	recordTask(t, here.site, jobs.Record{ID: "fine", Harness: "fixture", Asked: bob, State: jobs.Passed})
	body := here.ask(t, "GET", "/admin/usage?state=failed", nil).Body.String()
	if !strings.Contains(body, "No conversation has failed.") || strings.Contains(body, "/admin/usage/fine") {
		t.Errorf("an empty failed view:\n%s", body)
	}
}

// A row names who asked and, apart, whose agent answered: a room's agent is paid for by its owner, whoever mentions it.
func TestARowSaysWhoAskedAndWhoseAgentAnswered(t *testing.T) {
	call := aCall("turn", 0, "room:1", alice, 0)
	call.Agent = "Nova"
	here, _ := withUsage(t, call)
	recordTask(t, here.site, jobs.Record{ID: "turn", Harness: "fixture", Asked: bob, PaidBy: alice, Instance: "room", Thread: "1", State: jobs.Passed})
	body := here.ask(t, "GET", "/admin/usage", nil).Body.String()
	owner, asker := here.server.personName(alice), here.server.personName(bob)
	if !strings.Contains(body, ">Asked by<") || !strings.Contains(body, ">Agent · owner<") {
		t.Errorf("the headings are not Asked by and Agent · owner:\n%s", body)
	}
	if !strings.Contains(body, `<td class="max-w-40 truncate">`+asker+`</td>`) {
		t.Errorf("the row does not say %s asked", asker)
	}
	if !strings.Contains(body, `Nova <span class="text-subtle">· `+owner+`'s</span>`) {
		t.Errorf("the row does not say the agent is %s's", owner)
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

	byType := here.ask(t, "GET", "/admin/usage?agent_type=codex", nil).Body.String()
	if !strings.Contains(byType, "/admin/usage/codex-session") || strings.Contains(byType, "/admin/usage/alice-session") {
		t.Errorf("an agent type by its id does not narrow the table:\n%s", byType)
	}
	if !strings.Contains(byType, "<option selected>Codex</option>") || !strings.Contains(byType, "<details class=\"group\" open>") {
		t.Error("an agent type by its id is not shown chosen")
	}

	byPerson := here.ask(t, "GET", "/admin/usage?person="+url.QueryEscape(bob), nil).Body.String()
	if !strings.Contains(byPerson, "/admin/usage/bob-session") || strings.Contains(byPerson, "/admin/usage/alice-session") {
		t.Errorf("a person's link does not narrow the table:\n%s", byPerson)
	}
	if !strings.Contains(byPerson, "<option selected>"+bob+"</option>") {
		t.Error("a person's link is not shown chosen")
	}
}
