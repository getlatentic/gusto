package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/job"
)

// The conversation can be asked what its latest turn has done so far, which is what the page shows while it waits.
func TestAConversationSaysWhatItsLatestTurnHasDoneSoFar(t *testing.T) {
	site := open(t)
	site.signIn(t)
	where := startedThread(t, site)
	site.ask(t, "POST", where+"/say", url.Values{"said": {"make a page"}})
	answer := site.ask(t, "GET", where+"/live", nil)
	var live liveTurn
	if answer.Code != http.StatusOK || json.Unmarshal(answer.Body.Bytes(), &live) != nil || live.Items == nil {
		t.Fatalf("the live status answered %d %s", answer.Code, answer.Body.String())
	}
	if none := site.ask(t, "GET", "/instances/nobody/threads/1/live", nil); none.Code != http.StatusNotFound {
		t.Errorf("a conversation that is not there answered %d", none.Code)
	}
}

// A project in the side list has its own button for a new chat, and the chat that opens is already in its folder.
func TestAProjectInTheSideListStartsAChatInItsFolder(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id := strings.TrimPrefix(startedInstance(t, site), "/instances/")
	site.ask(t, "POST", "/instances/"+id+"/threads/1/say", url.Values{"said": {"hello"}})
	list := site.ask(t, "GET", "/jobs", nil).Body.String()
	if !strings.Contains(list, `href="/chat?at=`+id+`"`) {
		t.Errorf("the project has no button for a new chat: %s", list)
	}
	page := site.ask(t, "GET", "/chat?at="+id, nil).Body.String()
	if !strings.Contains(page, `data-at="`+id+`"`) {
		t.Errorf("the chat does not open in the project's folder")
	}
	for _, bad := range []string{"nobody", id + "/..", id + "/../.."} {
		if page := site.ask(t, "GET", "/chat?at="+url.QueryEscape(bad), nil).Body.String(); strings.Contains(page, "data-at=") {
			t.Errorf("%q opened a chat in a folder", bad)
		}
	}
}

// While a turn runs the box offers Stop where it offers Send, and not a second Stop beside the reply.
func TestWhileATurnRunsTheBoxOffersStop(t *testing.T) {
	site := open(t)
	site.signIn(t)
	where := startedThread(t, site)
	body := site.ask(t, "GET", where, nil).Body.String()
	if strings.Contains(body, `id="stop"`) {
		t.Error("a conversation with nothing running offers Stop")
	}
	if !strings.Contains(body, `id="send"`) {
		t.Error("the box has no Send")
	}
}

// A turn's words and its steps come back in the order they happened, whole, with the words between the steps where they were said.
func TestALiveTurnGivesWhatWasSaidAndRunInOrder(t *testing.T) {
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	said := "I will look at the file first. Then I will fix it. All done."
	first, second := strings.Index(said, "Then"), strings.Index(said, "All")
	if err := os.WriteFile(dir.OutFile("transcript.log"), []byte(said), 0o644); err != nil {
		t.Fatal(err)
	}
	steps := fmt.Sprintf("{\"tool\":\"read\",\"at\":%d,\"input\":{\"filePath\":\"a.txt\"}}\n{\"tool\":\"edit\",\"at\":%d,\"input\":{\"filePath\":\"a.txt\"}}\n", first, second)
	if err := os.WriteFile(dir.OutFile("commands.jsonl"), []byte(steps), 0o644); err != nil {
		t.Fatal(err)
	}
	got := timelineOf(dir)
	var order []string
	for _, each := range got.Items {
		order = append(order, each.Kind)
	}
	if strings.Join(order, ",") != "said,step,said,step,said" {
		t.Fatalf("the order was %v: %+v", order, got.Items)
	}
	if got.Items[0].Text != "I will look at the file first." || got.Items[2].Text != "Then I will fix it." || got.Items[4].Text != "All done." {
		t.Errorf("the words were cut or misplaced: %+v", got.Items)
	}
	if got.Items[1].What != "Read" || got.Steps != 2 {
		t.Errorf("the steps: %+v", got)
	}
}

// A run from before steps said where they happened gives all its steps, and then what was said.
func TestALiveTurnWithoutPlacesGivesItsStepsThenItsWords(t *testing.T) {
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(dir.OutFile("transcript.log"), []byte("done"), 0o644)
	os.WriteFile(dir.OutFile("commands.jsonl"), []byte("{\"tool\":\"read\",\"input\":{\"filePath\":\"a.txt\"}}\n"), 0o644)
	got := timelineOf(dir)
	if len(got.Items) != 2 || got.Items[0].Kind != "step" || got.Items[1].Text != "done" {
		t.Errorf("%+v", got.Items)
	}
}

// What is left of a very long transcript starts at a word, never in one.
func TestALongTranscriptIsCutBetweenWords(t *testing.T) {
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(dir.OutFile("transcript.log"), []byte(strings.Repeat("analytics ", liveRead/5)), 0o644)
	got := timelineOf(dir)
	if len(got.Items) != 1 || !got.Cut || !strings.HasPrefix(got.Items[0].Text, "analytics") {
		t.Errorf("the start of what is left: %.40q (cut %v)", got.Items[0].Text, got.Cut)
	}
}
