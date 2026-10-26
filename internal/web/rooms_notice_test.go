package web

import (
	"net/url"
	"strings"
	"testing"
)

func TestWhatTheRoomSaysOfItselfShowsAsANoticeAndNotAsAPersonsMessage(t *testing.T) {
	site, id, thread := aRoom(t, &roomWork{}, [2]string{"Scout", "fixture"}, [2]string{"Draft", "other"})
	threadID := site.server.instances.Latest(id).ID
	text := "Scout and Draft both changed report.md. report.md is Scout's version and report.draft.md is Draft's version."
	if _, err := site.server.instances.Notice(id, threadID, text); err != nil {
		t.Fatal(err)
	}
	held, _ := site.server.instances.Get(id)
	latest, _ := site.server.instances.Thread(id, threadID)
	if brief := site.server.roomBrief(held, latest, held.Agents[0], alice, "carry on", ""); !strings.Contains(brief, "The room says: Scout and Draft both changed report.md.") {
		t.Errorf("an agent is not told of the conflict:\n%s", brief)
	}
	site.as(t, alice).ask(t, "POST", thread+"/say", url.Values{"said": {"a word from a person"}})
	page := site.as(t, alice).ask(t, "GET", thread, nil).Body.String()
	for _, want := range []string{"both changed report.md", `role="status"`, "a word from a person"} {
		if !strings.Contains(page, want) {
			t.Errorf("the room does not show %q", want)
		}
	}
	at := strings.Index(page, "both changed report.md")
	if before := page[max(0, at-400):at]; strings.Contains(before, "rounded-full bg-sunken") {
		t.Errorf("the notice reads as a message from somebody: %s", before)
	}
}
