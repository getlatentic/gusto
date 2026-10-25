package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/job"
)

// heardTurn is what one of a room's agents was given for a turn.
type heardTurn struct {
	Agent, Harness, Brief string
}

// roomWork answers each turn with the reply set for the agent that took it, and keeps what each was given.
type roomWork struct {
	mu      sync.Mutex
	heard   []heardTurn
	replies map[string]string
}

func (w *roomWork) do(_ context.Context, dir job.Dir) (int, error) {
	text, err := os.ReadFile(dir.Spec())
	if err != nil {
		return 1, err
	}
	var spec job.Spec
	if err := json.Unmarshal(text, &spec); err != nil {
		return 1, err
	}
	w.mu.Lock()
	w.heard = append(w.heard, heardTurn{spec.Agent, spec.Harness, spec.Brief})
	reply := w.replies[spec.Agent]
	w.mu.Unlock()
	if reply == "" {
		reply = "done"
	}
	if err := os.MkdirAll(dir.Out(), 0o755); err != nil {
		return 1, err
	}
	return 0, os.WriteFile(dir.OutFile("transcript.log"), []byte(reply+"\n"), 0o644)
}

func (w *roomWork) turns() []heardTurn {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]heardTurn(nil), w.heard...)
}

// waitTurns holds until the agents have taken n turns between them and the queue has nothing left to run.
func (w *roomWork) waitTurns(t *testing.T, site *site, n int) []heardTurn {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if heard := w.turns(); len(heard) >= n && site.allDone() {
			return heard
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the agents took %d turns, not %d", len(w.turns()), n)
	return nil
}

func (s *site) allDone() bool {
	for _, record := range s.index.List() {
		if !record.State.Done() {
			return false
		}
	}
	return true
}

func twoHarnesses() []catalog.Entry {
	return []catalog.Entry{
		{Name: "fixture", Title: "Fixture", What: "a fixture", Program: "sh", Needs: harness.NeedsNothing, Ready: true},
		{Name: "other", Title: "Other", What: "another fixture", Program: "sh", Needs: harness.NeedsNothing, Ready: true},
	}
}

// aRoom is a room alice made, with the agents named, each on the harness after its name.
func aRoom(t *testing.T, work *roomWork, agents ...[2]string) (*site, string, string) {
	t.Helper()
	testWork = work.do
	t.Cleanup(func() { testWork = nil })
	site := openOffering(t, twoHarnesses())
	mine := site.as(t, alice)
	made := mine.ask(t, "POST", "/rooms", url.Values{"name": {"Launch plan"}})
	if made.Code != http.StatusSeeOther {
		t.Fatalf("making a room gave %d: %s", made.Code, made.Body)
	}
	members := made.Header().Get("Location")
	id := strings.TrimSuffix(strings.TrimPrefix(members, "/rooms/"), "/members")
	for _, agent := range agents {
		added := mine.ask(t, "POST", "/rooms/"+id+"/agents", url.Values{"name": {agent[0]}, "harness": {agent[1]}, "job": {"does " + agent[0]}})
		if added.Code != http.StatusSeeOther || strings.Contains(added.Header().Get("Location"), "refused") {
			t.Fatalf("adding %s gave %d to %s", agent[0], added.Code, added.Header().Get("Location"))
		}
	}
	held, _ := site.server.instances.Get(id)
	return site, id, threadPath(site.server.instances.Latest(held.ID))
}

func TestMakingARoomListsItForItsMembersAndNobodyElse(t *testing.T) {
	work := &roomWork{}
	site, id, thread := aRoom(t, work)
	if page := site.as(t, alice).ask(t, "GET", "/chat", nil).Body.String(); !strings.Contains(page, "Launch plan") || !strings.Contains(page, thread) {
		t.Error("the side list does not carry the room to the person who made it")
	}
	if page := site.as(t, bob).ask(t, "GET", "/chat", nil).Body.String(); strings.Contains(page, "Launch plan") {
		t.Error("the side list carries a room to somebody who is not in it")
	}
	if got := site.as(t, bob).ask(t, "GET", thread, nil); got.Code != http.StatusNotFound {
		t.Errorf("the room answered %d to somebody not in it", got.Code)
	}
	if added := site.as(t, alice).ask(t, "POST", "/rooms/"+id+"/members", url.Values{"who": {bob}}); added.Code != http.StatusSeeOther {
		t.Fatalf("adding bob gave %d", added.Code)
	}
	if got := site.as(t, bob).ask(t, "GET", thread, nil); got.Code != http.StatusOK {
		t.Errorf("the room answered %d to a member", got.Code)
	}
	if page := site.as(t, bob).ask(t, "GET", "/chat", nil).Body.String(); !strings.Contains(page, "Launch plan") {
		t.Error("the side list does not carry the room to a member")
	}
}

func TestOnlyARoomsAdminChangesWhoIsInIt(t *testing.T) {
	work := &roomWork{}
	site, id, _ := aRoom(t, work, [2]string{"Scout", "fixture"})
	site.as(t, alice).ask(t, "POST", "/rooms/"+id+"/members", url.Values{"who": {bob}})
	his := site.as(t, bob)
	for _, change := range []struct {
		path string
		form url.Values
	}{
		{"/rooms/" + id + "/members", url.Values{"who": {"carol@example.com"}}},
		{"/rooms/" + id + "/members/remove", url.Values{"who": {alice}}},
		{"/rooms/" + id + "/agents", url.Values{"name": {"Draft"}, "harness": {"fixture"}}},
		{"/rooms/" + id + "/agents/a1/remove", nil},
	} {
		if got := his.ask(t, "POST", change.path, change.form); got.Code != http.StatusForbidden {
			t.Errorf("a member's POST %s answered %d", change.path, got.Code)
		}
	}
	if got := his.ask(t, "POST", "/instances/"+id+"/remove", url.Values{}); got.Code != http.StatusNotFound {
		t.Errorf("a member removing the room answered %d", got.Code)
	}
	held, _ := site.server.instances.Get(id)
	if len(held.Members) != 2 || len(held.Agents) != 1 {
		t.Errorf("a member changed the room: %+v", held)
	}
	if left := his.ask(t, "POST", "/rooms/"+id+"/members/remove", url.Values{"who": {bob}}); left.Code != http.StatusSeeOther {
		t.Errorf("leaving gave %d", left.Code)
	}
	if held, _ := site.server.instances.Get(id); held.RoleOf(bob) != "" {
		t.Error("a member could not leave")
	}
}

func TestAMentionGoesToTheAgentNamedOnItsOwnHarness(t *testing.T) {
	work := &roomWork{}
	site, id, thread := aRoom(t, work, [2]string{"Scout", "fixture"}, [2]string{"Draft", "other"})
	site.as(t, alice).ask(t, "POST", "/rooms/"+id+"/members", url.Values{"who": {bob}})
	if said := site.as(t, bob).ask(t, "POST", thread+"/say", url.Values{"said": {"@Draft write the page"}}); said.Code != http.StatusSeeOther {
		t.Fatalf("saying something gave %d", said.Code)
	}
	heard := work.waitTurns(t, site, 1)
	if len(heard) != 1 || heard[0].Agent != "a2" || heard[0].Harness != "other" {
		t.Fatalf("the turns went to %+v", heard)
	}
	for _, want := range []string{"You are Draft, for does Draft", "@Scout", bob + " says to you:\n@Draft write the page"} {
		if !strings.Contains(heard[0].Brief, want) {
			t.Errorf("the agent was not told %q:\n%s", want, heard[0].Brief)
		}
	}
	page := site.as(t, alice).ask(t, "GET", thread, nil).Body.String()
	for _, want := range []string{">bob<", `>@Draft</span> write the page`, ">Draft<", "does Draft"} {
		if !strings.Contains(page, want) {
			t.Errorf("the room does not show %q", want)
		}
	}
}

func TestBothAgentsMentionedAnswer(t *testing.T) {
	work := &roomWork{}
	site, _, thread := aRoom(t, work, [2]string{"Scout", "fixture"}, [2]string{"Draft", "other"})
	site.as(t, alice).ask(t, "POST", thread+"/say", url.Values{"said": {"@Scout and @Draft, and @Scout again"}})
	heard := work.waitTurns(t, site, 2)
	if len(heard) != 2 {
		t.Fatalf("the agents took %d turns, not one each", len(heard))
	}
	if page := site.as(t, alice).ask(t, "GET", thread, nil).Body.String(); strings.Count(page, "@Scout</span> again") != 1 {
		t.Error("the room shows one message once for each agent it mentions")
	}
}

func TestWhatIsSaidStaysInTheOrderItWasSaid(t *testing.T) {
	work := &roomWork{replies: map[string]string{"a1": "the reply"}}
	site, _, thread := aRoom(t, work, [2]string{"Scout", "fixture"}, [2]string{"Draft", "other"})
	mine := site.as(t, alice)
	mine.ask(t, "POST", thread+"/say", url.Values{"said": {"the first note"}})
	mine.ask(t, "POST", thread+"/say", url.Values{"said": {"@Scout the question"}})
	work.waitTurns(t, site, 1)
	mine.ask(t, "POST", thread+"/say", url.Values{"said": {"the last note"}})
	page := mine.ask(t, "GET", thread, nil).Body.String()
	order := []int{strings.Index(page, "the first note"), strings.Index(page, "the question"), strings.Index(page, "the reply"), strings.Index(page, "the last note")}
	for i := 1; i < len(order); i++ {
		if order[i-1] < 0 || order[i] <= order[i-1] {
			t.Fatalf("the room shows what was said out of order: %v", order)
		}
	}
}

func TestWithSeveralAgentsAndNoMentionNobodyAnswers(t *testing.T) {
	work := &roomWork{}
	site, _, thread := aRoom(t, work, [2]string{"Scout", "fixture"}, [2]string{"Draft", "other"})
	mine := site.as(t, alice)
	said := mine.ask(t, "POST", thread+"/say", url.Values{"said": {"lunch at noon"}})
	if said.Code != http.StatusSeeOther || !strings.Contains(said.Header().Get("Location"), "#n1") {
		t.Fatalf("a note gave %d to %s", said.Code, said.Header().Get("Location"))
	}
	if page := mine.ask(t, "GET", thread, nil).Body.String(); !strings.Contains(page, "lunch at noon") {
		t.Error("the room does not show what was said to nobody")
	}
	mine.ask(t, "POST", thread+"/say", url.Values{"said": {"@Scout what did I say?"}})
	heard := work.waitTurns(t, site, 1)
	if len(heard) != 1 || !strings.Contains(heard[0].Brief, alice+": lunch at noon") {
		t.Fatalf("the agent was not told what was said since it last spoke: %+v", heard)
	}
}

func TestARoomsOnlyAgentAnswersWithoutAMention(t *testing.T) {
	work := &roomWork{}
	site, _, thread := aRoom(t, work, [2]string{"Scout", "fixture"})
	site.as(t, alice).ask(t, "POST", thread+"/say", url.Values{"said": {"what is next?"}})
	if heard := work.waitTurns(t, site, 1); heard[0].Agent != "a1" {
		t.Fatalf("the turn went to %q", heard[0].Agent)
	}
}

func TestAnAgentHandsWorkOnByMentioningAnotherAndTheChainStops(t *testing.T) {
	work := &roomWork{replies: map[string]string{"a1": "@Draft over to you", "a2": "@Scout back to you"}}
	site, _, thread := aRoom(t, work, [2]string{"Scout", "fixture"}, [2]string{"Draft", "other"})
	site.as(t, alice).ask(t, "POST", thread+"/say", url.Values{"said": {"@Scout start"}})
	heard := work.waitTurns(t, site, 1+handOffs)
	time.Sleep(100 * time.Millisecond)
	if heard = work.turns(); len(heard) != 1+handOffs {
		t.Fatalf("the agents took %d turns, not %d", len(heard), 1+handOffs)
	}
	if !strings.Contains(heard[1].Brief, "@Scout says to you:\n@Draft over to you") {
		t.Errorf("the agent handed to was not told who handed it on:\n%s", heard[1].Brief)
	}
	if page := site.as(t, alice).ask(t, "GET", thread, nil).Body.String(); !strings.Contains(page, `asked by <span class="rounded-xs bg-accent-wash px-0.5 font-medium text-accent">@Scout</span>`) {
		t.Error("the room does not show which agent handed a turn on")
	}
}

func TestAnAgentKeepsItsOwnConversationInARoom(t *testing.T) {
	work := &roomWork{}
	site, id, thread := aRoom(t, work, [2]string{"Scout", "fixture"}, [2]string{"Draft", "other"})
	held, _ := site.server.instances.Get(id)
	threadID := site.server.instances.Latest(held.ID).ID
	if err := site.server.instances.Learned(id, threadID, "a1", "scout-session"); err != nil {
		t.Fatal(err)
	}
	site.as(t, alice).ask(t, "POST", thread+"/say", url.Values{"said": {"@Scout @Draft go"}})
	work.waitTurns(t, site, 2)
	var specs []job.Spec
	for _, record := range site.index.List() {
		text, err := os.ReadFile(record.Dir(site.index.Root()).Spec())
		if err != nil {
			t.Fatal(err)
		}
		var spec job.Spec
		json.Unmarshal(text, &spec)
		specs = append(specs, spec)
	}
	for _, spec := range specs {
		if want := map[string]string{"a1": "scout-session", "a2": ""}[spec.Agent]; spec.Remembered != want {
			t.Errorf("%s carried on %q, not %q", spec.Agent, spec.Remembered, want)
		}
	}
}

func TestAMessageShowsItsMentionsAndNothingElseAsMarkup(t *testing.T) {
	shown := string(mentions(`<b>hi</b> @Scout, mail me@example.com`))
	if strings.Contains(shown, "<b>") || !strings.Contains(shown, ">@Scout</span>,") || strings.Contains(shown, ">@example") {
		t.Errorf("the message shows as %s", shown)
	}
	if got := initials("mira.ionescu@example.com"); got != "MI" {
		t.Errorf("the initials are %q", got)
	}
}
