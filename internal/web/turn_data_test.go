package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// While a turn runs, the conversation's data says so and names it, so the page can say what it is doing and offer to stop it.
func TestARunningTurnIsNamedInTheConversationsDataSoItCanBeStopped(t *testing.T) {
	started := holding(t)
	site := open(t)
	site.signIn(t)
	where := startedThread(t, site)
	site.ask(t, "POST", where+"/say", url.Values{"said": {"a long piece of work"}})
	<-started
	page := site.conversation(t, where)
	if !page.Waiting || page.Latest == "" || len(page.Turns) != 1 || page.Turns[0].ID != page.Latest || page.Turns[0].Done {
		t.Errorf("the data does not say a turn is running: waiting %v, latest %q, %+v", page.Waiting, page.Latest, page.Turns)
	}
	if stopped := site.ask(t, "POST", "/jobs/"+page.Latest+"/stop", nil); stopped.Code != http.StatusSeeOther {
		t.Errorf("stopping the running turn answered %d", stopped.Code)
	}
}

// A chat that could not start carries why, for the page to say at the box.
func TestARefusedChatSaysWhyAtTheBox(t *testing.T) {
	site := open(t)
	site.signIn(t)
	sealed := strings.TrimPrefix(site.server.withMessage("/chat", "refused", "That folder is not yours."), "/chat")
	if chat := chatOf(t, site, sealed); chat.Refused != "That folder is not yours." || !chat.Ready {
		t.Errorf("the chat does not carry the reason with a box to say it at: %+v", chat)
	}
}
