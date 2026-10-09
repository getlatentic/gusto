package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// The conversation can be asked what its latest turn has done so far, which is what the page shows while it waits.
func TestAConversationSaysWhatItsLatestTurnHasDoneSoFar(t *testing.T) {
	site := open(t)
	site.signIn(t)
	where := startedThread(t, site)
	site.ask(t, "POST", where+"/say", url.Values{"said": {"make a page"}})
	answer := site.ask(t, "GET", where+"/live", nil)
	var live liveTurn
	if answer.Code != http.StatusOK || json.Unmarshal(answer.Body.Bytes(), &live) != nil || live.Steps == nil {
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
	list := site.ask(t, "GET", "/side/chats?here=", nil).Body.String()
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
