package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/spend"
)

func TestAConversationThatIsNotThereOrNotThePersonsIsA404AsAPageAndAsData(t *testing.T) {
	site := open(t)
	site.signIn(t)
	where := startedThread(t, site)
	instance := where[:strings.Index(where, "/threads/")]
	for _, path := range []string{"/instances/nobody/threads/1", instance + "/threads/99"} {
		for _, asked := range []string{path, "/app/api" + path, "/app/api" + path + "/turns"} {
			if got := site.ask(t, "GET", asked, nil); got.Code != http.StatusNotFound {
				t.Errorf("%s answered %d", asked, got.Code)
			}
		}
	}
	if got := site.as(t, bob).ask(t, "GET", "/app/api"+where, nil); got.Code != http.StatusNotFound {
		t.Errorf("somebody else's conversation's data answered %d", got.Code)
	}
	if got := site.ask(t, "GET", "/app/api/rooms/nobody/files", nil); got.Code != http.StatusNotFound {
		t.Errorf("the files of a room that is not there answered %d", got.Code)
	}
}

func TestAConversationsDataHasEveryFieldThePageReads(t *testing.T) {
	site := open(t)
	site.signIn(t)
	var fields map[string]json.RawMessage
	site.data(t, startedThread(t, site), &fields)
	for _, name := range []string{"Instance", "Thread", "Named", "Path", "Me", "Agent", "DirectAgent", "Turns", "Starters", "Several",
		"Models", "Sharing", "Owns", "Apps", "Starting", "Waiting", "Latest", "Holding", "Hint", "Mentions", "Names", "Stream",
		"Version", "Headcount", "Settings", "Members", "Guidelines", "MaxGuidelines", "Refused", "AddRefused", "GuidelinesRefused",
		"Unsent", "AttachMost", "AttachBytes"} {
		if _, found := fields[name]; !found {
			t.Errorf("the conversation's data has no %s", name)
		}
	}
	if _, found := fields["Title"]; found || len(fields) != 34 {
		t.Errorf("the conversation's data has %d fields, not the page's 34", len(fields))
	}
	for _, empty := range []string{"DirectAgent", "Stream", "Members"} {
		if string(fields[empty]) != "null" {
			t.Errorf("a conversation that is not a room gives %s as %s, not null", empty, fields[empty])
		}
	}
}

func TestATurnsTimeAndCostAreGivenOnlyWhenItIsDoneAndPriced(t *testing.T) {
	site := open(t)
	scene := turnScene{held: instance.Instance{ID: "one", Harness: "fixture"}, agent: agent{Name: "Helper", Engine: "Fixture"}}
	shown := turn{Record: tasks.Record{ID: "t1", State: tasks.Passed, Asked: alice, Started: 100, Ended: 112}, Said: "a <b>question</b>",
		Spent: spend.Tally{USD: 0.25}, Metered: true}
	view := site.server.turnViewOf(shown, scene)
	if view.Took != "12s" || view.Money != shown.Spent.Money() || !view.Done || view.By != "Helper" || view.Harness != "fixture" {
		t.Errorf("a finished, priced turn is %+v", view)
	}
	if view.Kind != "turn" || !view.ShowSaid || view.SaidHTML != "a &lt;b&gt;question&lt;/b&gt;" || view.Who != alice || view.WhoName == "" {
		t.Errorf("a person's message is %+v", view)
	}
	for name, change := range map[string]func(*turn){
		"not metered":     func(each *turn) { each.Metered = false },
		"partly unpriced": func(each *turn) { each.Spent.Unmeasured = 1 },
		"free":            func(each *turn) { each.Spent.USD = 0 },
		"still running":   func(each *turn) { each.State = tasks.Running },
	} {
		changed := shown
		change(&changed)
		if got := site.server.turnViewOf(changed, scene); got.Money != "" || (name == "still running" && (got.Took != "" || got.Done)) {
			t.Errorf("a turn %s is given %q for %q", name, got.Money, got.Took)
		}
	}
}

func TestARoomsNotesAndHandedOnTurnsAreToldApart(t *testing.T) {
	site := open(t)
	scene := turnScene{held: instance.Instance{ID: "room", Room: true}, names: []string{"Scout"}}
	cases := map[string]struct {
		shown    turn
		kind     string
		showSaid bool
	}{
		"a notice":         {turn{Said: "a file changed", roomTurn: roomTurn{Note: true, Notice: true, File: "plan.md"}}, "notice", true},
		"a note":           {turn{Said: "lunch", roomTurn: roomTurn{Note: true, Who: alice}}, "note", true},
		"a handed-on turn": {turn{Said: "@Scout go", roomTurn: roomTurn{By: "Draft", From: "Scout"}}, "turn", false},
		"an echo":          {turn{Said: "@Scout go", roomTurn: roomTurn{By: "Draft", Echo: true}}, "turn", false},
	}
	for name, each := range cases {
		view := site.server.turnViewOf(each.shown, scene)
		if view.Kind != each.kind || view.ShowSaid != each.showSaid {
			t.Errorf("%s is %q, showing what was said %v", name, view.Kind, view.ShowSaid)
		}
	}
	if note := site.server.turnViewOf(cases["a notice"].shown, scene); note.File != "plan.md" || !note.Done {
		t.Errorf("a notice is %+v", note)
	}
}
