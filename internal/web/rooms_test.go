package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/catalog"
	"github.com/getlatentic/gusto/internal/crew"
	"github.com/getlatentic/gusto/internal/instance"
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
	// patch is the change every turn reports making to the folder.
	patch string
	// hold, when set, keeps every turn running until it is closed or the turn is stopped.
	hold chan struct{}
}

func (w *roomWork) do(ctx context.Context, dir job.Dir) (int, error) {
	if w.hold != nil {
		select {
		case <-w.hold:
		case <-ctx.Done():
			return 1, ctx.Err()
		}
	}
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
	if w.patch != "" {
		if err := os.WriteFile(dir.OutFile("diff.patch"), []byte(w.patch), 0o644); err != nil {
			return 1, err
		}
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
	if testWork == nil {
		testWork = work.do
	}
	t.Cleanup(func() { testWork = nil })
	site := openOffering(t, twoHarnesses())
	mine := site.as(t, alice)
	made := mine.ask(t, "POST", "/rooms", url.Values{"name": {"Launch plan"}})
	if made.Code != http.StatusSeeOther {
		t.Fatalf("making a room gave %d: %s", made.Code, made.Body)
	}
	opened := made.Header().Get("Location")
	if !strings.HasSuffix(opened, "?add=1") {
		t.Fatalf("a new room opens at %s, not with its add dialog", opened)
	}
	id := strings.TrimPrefix(opened[:strings.Index(opened, "/threads/")], "/instances/")
	for _, agent := range agents {
		member := aCrewAgent(t, site, alice, agent[0], agent[1])
		added := mine.ask(t, "POST", "/rooms/"+id+"/members/add", url.Values{"agent": {member.ID}})
		if added.Code != http.StatusSeeOther || strings.Contains(added.Header().Get("Location"), "refused") {
			t.Fatalf("adding %s gave %d to %s", agent[0], added.Code, added.Header().Get("Location"))
		}
	}
	held, _ := site.server.instances.Get(id)
	return site, id, threadPath(site.server.instances.Latest(held.ID))
}

// aCrewAgent is a workspace agent somebody made, whose specialty is "does" and its name.
func aCrewAgent(t *testing.T, site *site, owner, name, harness string) crew.Agent {
	t.Helper()
	made, err := site.server.crew.Make(crew.Agent{Name: name, Harness: harness, Specialty: "does " + name, Owner: owner})
	if err != nil {
		t.Fatal(err)
	}
	return made
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
		{"/rooms/" + id + "/members/add", url.Values{"who": {"carol@example.com"}}},
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
	for _, want := range []string{">Bob<", `>@Draft</span> write the page`, ">Draft<", "does Draft"} {
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
	if page := site.as(t, alice).ask(t, "GET", thread, nil).Body.String(); !strings.Contains(page, `asked by <span class="text-accent">@Scout</span>`) {
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
	names := []string{"Scout", "Ada", "Ada Lovelace", "Tosin's Claude"}
	shown := string(mentionsIn(`<b>hi</b> @Scout, mail me@example.com, @Ada Lovelace and @tosin's claude, not @Scouting`, names))
	for _, want := range []string{`>@Scout</span>,`, `>@Ada Lovelace</span> and`, `>@tosin&#39;s claude</span>,`} {
		if !strings.Contains(shown, want) {
			t.Errorf("the message does not show %s: %s", want, shown)
		}
	}
	if strings.Contains(shown, "<b>") || strings.Contains(shown, ">@example") || strings.Contains(shown, ">@Scouting") || strings.Contains(shown, ">@Ada</span>") {
		t.Errorf("the message shows as %s", shown)
	}
	if got := initialsOf(nameFromAddress("mira.ionescu@example.com")); got != "MI" {
		t.Errorf("the initials are %q", got)
	}
}

func TestARoomCountsWhatAPersonHasNotReadAndWhatMentionsThem(t *testing.T) {
	work := &roomWork{}
	here, id, thread := aRoom(t, work, [2]string{"Scout", "fixture"}, [2]string{"Draft", "other"})
	mine, his := here.as(t, alice), here.as(t, bob)
	mine.ask(t, "POST", "/rooms/"+id+"/members", url.Values{"who": {bob}})
	his.ask(t, "GET", thread, nil)
	time.Sleep(1100 * time.Millisecond)
	mine.ask(t, "POST", thread+"/say", url.Values{"said": {"@bob can you look?"}})
	mine.ask(t, "POST", thread+"/say", url.Values{"said": {"and lunch at noon"}})

	tabs := func(who *site) (unread, forYou string) {
		page := who.ask(t, "GET", "/chat", nil).Body.String()
		return between(page, `data-side-tab="unread">Unread`, "</button>"), between(page, `data-side-tab="me">@me`, "</button>")
	}
	if unread, forYou := tabs(his); !strings.Contains(unread, ">1<") || !strings.Contains(forYou, ">1<") {
		t.Errorf("bob's tabs read %q and %q, not one room unread that mentions him", unread, forYou)
	}
	if unread, _ := tabs(mine); strings.Contains(unread, "<span") {
		t.Errorf("what alice said herself counts as unread for her: %q", unread)
	}
	his.ask(t, "GET", thread, nil)
	if unread, forYou := tabs(his); strings.Contains(unread, "<span") || strings.Contains(forYou, "<span") {
		t.Errorf("after bob looked his tabs still read %q and %q", unread, forYou)
	}
}

func between(text, start, end string) string {
	at := strings.Index(text, start)
	if at < 0 {
		return ""
	}
	text = text[at+len(start):]
	return text[:max(strings.Index(text, end), 0)]
}

func TestAWorkspaceAgentAndAPersonAreAddedToARoomTogether(t *testing.T) {
	work := &roomWork{}
	here, id, thread := aRoom(t, work)
	member := aCrewAgent(t, here, alice, "Tosin's Claude", "other")
	theirs := aCrewAgent(t, here, bob, "Bob's Codex", "fixture")
	mine := here.as(t, alice)
	page := mine.ask(t, "GET", thread, nil).Body.String()
	if !strings.Contains(page, `value="`+member.ID+`"`) || strings.Contains(page, `value="`+theirs.ID+`"`) {
		t.Fatal("the add dialog does not offer exactly the agents the person may work with")
	}
	added := mine.ask(t, "POST", "/rooms/"+id+"/members/add", url.Values{"agent": {member.ID}, "who": {bob + ", carol@example.com"}})
	if added.Code != http.StatusSeeOther || strings.Contains(added.Header().Get("Location"), "refused") {
		t.Fatalf("adding gave %d to %s", added.Code, added.Header().Get("Location"))
	}
	held, _ := here.server.instances.Get(id)
	if agent, found := held.AgentCalled("Tosin's Claude"); !found || agent.Member != member.ID {
		t.Errorf("the agent became %+v", held.Agents)
	}
	if held.RoleOf(bob) != instance.RoleMember || held.RoleOf("carol@example.com") != instance.RoleMember {
		t.Errorf("the people were not added: %+v", held.Members)
	}
	for _, refused := range []string{member.ID, theirs.ID} {
		if got := mine.ask(t, "POST", "/rooms/"+id+"/members/add", url.Values{"agent": {refused}}); !strings.Contains(got.Header().Get("Location"), "refused") {
			t.Errorf("adding %s again or somebody else's agent was not refused", refused)
		}
	}
	mine.ask(t, "POST", "/rooms/"+id+"/members/role", url.Values{"who": {bob}, "role": {"admin"}})
	mine.ask(t, "POST", "/rooms/"+id+"/members/role", url.Values{"who": {"carol@example.com"}, "role": {"remove"}})
	if held, _ := here.server.instances.Get(id); held.RoleOf(bob) != instance.RoleAdmin || held.RoleOf("carol@example.com") != "" {
		t.Errorf("roles did not change: %+v", held.Members)
	}
	if got := here.as(t, "carol@example.com").ask(t, "POST", "/rooms/"+id+"/members/add", url.Values{"who": {"dave@example.com"}}); got.Code != http.StatusNotFound {
		t.Errorf("somebody taken out of the room could still add to it: %d", got.Code)
	}
}

func TestARoomUsesItsAgentsAsTheWorkspaceHasThemNow(t *testing.T) {
	work := &roomWork{}
	here, id, thread := aRoom(t, work, [2]string{"Scout", "fixture"})
	held, _ := here.server.instances.Get(id)
	member, _ := here.server.crew.Get(held.Agents[0].Member)
	if _, err := here.server.crew.Change(member.ID, func(agent *crew.Agent) error {
		agent.Name, agent.Model, agent.Persona = "Scout Prime", "platform/test-model", "You answer in one line."
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	here.as(t, alice).ask(t, "POST", thread+"/say", url.Values{"said": {"@Scout Prime what next?"}})
	heard := work.waitTurns(t, here, 1)
	for _, want := range []string{"You are Scout Prime", "You answer in one line.", "@Scout Prime what next?"} {
		if !strings.Contains(heard[0].Brief, want) {
			t.Errorf("the agent was not told %q:\n%s", want, heard[0].Brief)
		}
	}
	if page := here.as(t, alice).ask(t, "GET", thread, nil).Body.String(); !strings.Contains(page, ">@Scout Prime</span>") {
		t.Error("the room does not show the renamed agent's mention")
	}
}
func TestARoomListsItsFilesByWhoMadeThemAndTakesUploadsFromItsPeople(t *testing.T) {
	work := &roomWork{patch: "diff --git a/brief.md b/brief.md\nnew file mode 100644\n--- /dev/null\n+++ b/brief.md\n@@ -0,0 +1 @@\n+the brief\n"}
	here, id, thread := aRoom(t, work, [2]string{"Scout", "fixture"})
	mine := here.as(t, alice)
	mine.ask(t, "POST", "/rooms/"+id+"/members/add", url.Values{"who": {bob}})
	mine.ask(t, "POST", thread+"/say", url.Values{"said": {"write the brief"}})
	work.waitTurns(t, here, 1)
	if err := os.WriteFile(filepath.Join(here.server.instances.Workspace(id), "brief.md"), []byte("the brief\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	upload := func(who *site, name, body string) int {
		var sent bytes.Buffer
		form := multipart.NewWriter(&sent)
		part, _ := form.CreateFormFile("file", name)
		part.Write([]byte(body))
		form.Close()
		request := httptest.NewRequest("POST", "/rooms/"+id+"/files", &sent)
		request.Header.Set("Content-Type", form.FormDataContentType())
		request.AddCookie(who.cookie)
		answer := httptest.NewRecorder()
		who.handler.ServeHTTP(answer, request)
		return answer.Code
	}
	his := here.as(t, bob)
	if got := upload(his, "notes.txt", "from bob"); got != http.StatusNoContent {
		t.Fatalf("a member's upload answered %d", got)
	}
	if got := upload(his, "notes.txt", "again"); got != http.StatusNoContent {
		t.Fatalf("a second upload of the same name answered %d", got)
	}
	if kept, _ := os.ReadFile(filepath.Join(here.server.instances.Workspace(id), "notes (2).txt")); string(kept) != "again" {
		t.Errorf("a second file of the same name was kept as %q", kept)
	}
	if got := upload(here.as(t, "carol@example.com"), "x.txt", "no"); got != http.StatusNotFound {
		t.Errorf("somebody outside the room uploaded, answered %d", got)
	}

	panel := his.ask(t, "GET", "/rooms/"+id+"/files?panel=1", nil).Body.String()
	agents, people := between(panel, `data-files-list="agents"`, `data-files-list="people"`), panel[strings.Index(panel, `data-files-list="people"`):]
	if !strings.Contains(agents, "brief.md") || strings.Contains(agents, "notes.txt") {
		t.Errorf("the agents' list is wrong:\n%s", agents)
	}
	if !strings.Contains(people, "notes.txt") || !strings.Contains(people, "notes (2).txt") || strings.Contains(people, "brief.md") {
		t.Errorf("the people's list is wrong:\n%s", people)
	}
}

func TestARoomSaysWhetherItChangedAndIsWrittenOnlyWhenItDid(t *testing.T) {
	work := &roomWork{replies: map[string]string{"a1": "done, @bob have a look"}}
	here, id, thread := aRoom(t, work, [2]string{"Scout", "fixture"})
	mine, his := here.as(t, alice), here.as(t, bob)
	mine.ask(t, "POST", "/rooms/"+id+"/members/add", url.Values{"who": {bob}})
	version := func() string { return his.ask(t, "GET", thread+"/version", nil).Body.String() }

	before := version()
	if again := version(); again != before {
		t.Errorf("the version moved with nothing said: %s then %s", before, again)
	}
	his.ask(t, "GET", thread, nil)
	file := filepath.Join(here.server.instances.Dir(id), "threads")
	stamp := func() map[string]int64 {
		stamps := map[string]int64{}
		entries, _ := os.ReadDir(file)
		for _, entry := range entries {
			info, _ := entry.Info()
			stamps[entry.Name()] = info.ModTime().UnixNano()
		}
		return stamps
	}
	looked := stamp()
	// Seen times are whole seconds, so the looks are a second apart, as they would be when a room's page asks again.
	time.Sleep(1100 * time.Millisecond)
	his.ask(t, "GET", thread+"/turns", nil)
	his.ask(t, "GET", thread, nil)
	if after := stamp(); fmt.Sprint(after) != fmt.Sprint(looked) {
		t.Error("looking at a room that had not changed wrote it")
	}

	mine.ask(t, "POST", thread+"/say", url.Values{"said": {"check the numbers"}})
	work.waitTurns(t, here, 1)
	if version() == before {
		t.Error("the version did not move when a turn was said and answered")
	}
	unreadTab := between(his.ask(t, "GET", "/chat", nil).Body.String(), `data-side-tab="me">@me`, "</button>")
	if !strings.Contains(unreadTab, ">1<") {
		t.Errorf("a reply that mentions bob is not counted for him: %q", unreadTab)
	}
	if got := here.as(t, "carol@example.com").ask(t, "GET", thread+"/version", nil); got.Code != http.StatusNotFound {
		t.Errorf("somebody outside the room read its version: %d", got.Code)
	}
}

func TestOnlyTheTextAfterAMentionIsKeptOfAReply(t *testing.T) {
	kept := mentionsOf("mail me@example.com, then @Ada Lovelace " + strings.Repeat("x", 500) + " and @bob")
	if strings.Contains(kept, "@example") || !strings.HasPrefix(kept, "@Ada Lovelace") || !strings.Contains(kept, "\n@bob\n") || len(kept) > 2*mentionReach {
		t.Errorf("kept %q", kept)
	}
	if !mentionsPerson(kept, nameFromAddress("bob@example.com")) || mentionsPerson(kept, nameFromAddress("carol@example.com")) {
		t.Error("the kept text does not answer who is mentioned")
	}
}

// A new room's add dialog, for somebody with no agents yet, says to make one rather than that every agent is already in.
func TestANewRoomSaysToMakeAnAgentWhenThereIsNone(t *testing.T) {
	site, _, where := aRoom(t, &roomWork{})
	page := site.as(t, alice).ask(t, "GET", where+"?add=1", nil).Body.String()
	if !strings.Contains(page, "You have no agents yet.") || strings.Contains(page, "is in this room already") {
		t.Error("the add dialog of a room with no agents does not say to make one")
	}
}

// Adding somebody who is in the room already says so, and changes nobody's role: the admin who adds themselves stays the admin.
func TestAddingSomebodyInTheRoomAlreadySaysSoAndKeepsTheirRole(t *testing.T) {
	here, id, _ := aRoom(t, &roomWork{})
	mine := here.as(t, alice)
	for who, want := range map[string]string{alice: "you are in this room already", "Alice@Example.com": "you are in this room already"} {
		added := mine.ask(t, "POST", "/rooms/"+id+"/members/add", url.Values{"who": {who}})
		if where, _ := url.QueryUnescape(added.Header().Get("Location")); !strings.Contains(where, want) {
			t.Errorf("adding %s went to %q", who, where)
		}
	}
	mine.ask(t, "POST", "/rooms/"+id+"/members/add", url.Values{"who": {bob}})
	added := mine.ask(t, "POST", "/rooms/"+id+"/members/add", url.Values{"who": {bob}})
	if where, _ := url.QueryUnescape(added.Header().Get("Location")); !strings.Contains(where, "is in this room already") {
		t.Errorf("adding bob again went to %q", where)
	}
	if held, _ := here.server.instances.Get(id); held.RoleOf(alice) != instance.RoleAdmin {
		t.Errorf("the admin is now %q", held.RoleOf(alice))
	}
}
