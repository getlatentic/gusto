package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/instance"
)

func TestHomeOpensTheRoomOfSomebodyWhoHasNoChatYet(t *testing.T) {
	site := openOffering(t, twoHarnesses())
	mine, his := site.as(t, alice), site.as(t, bob)
	if got := his.ask(t, "GET", "/", nil).Header().Get("Location"); got != "/chat" {
		t.Fatalf("a person with no room went to %q", got)
	}
	made := mine.ask(t, "POST", "/rooms", url.Values{"name": {"Launch plan"}})
	room := strings.TrimPrefix(strings.SplitN(made.Header().Get("Location"), "/threads/", 2)[0], "/instances/")
	mine.ask(t, "POST", "/rooms/"+room+"/members", url.Values{"who": {bob}})
	if got := his.ask(t, "GET", "/", nil).Header().Get("Location"); !strings.HasPrefix(got, "/instances/"+room+"/threads/") {
		t.Errorf("a colleague added to a room went to %q", got)
	}
}

func TestHomeStaysOnTheChatOnceThereIsOneOfTheirOwn(t *testing.T) {
	site := openOffering(t, twoHarnesses())
	mine := site.as(t, alice)
	mine.ask(t, "POST", "/rooms", url.Values{"name": {"Launch plan"}})
	if _, err := site.server.instances.Start("chat", instance.Instance{Harness: "other", Who: alice}); err != nil {
		t.Fatal(err)
	}
	if got := mine.ask(t, "GET", "/", nil).Header().Get("Location"); got != "/chat" {
		t.Errorf("a person with a chat of their own went to %q", got)
	}
}

func TestAnAgentMadeWithNoModelStartsOnAModelTheServerOffers(t *testing.T) {
	site := openOffering(t, twoHarnesses())
	mine := site.as(t, alice)
	made := mine.ask(t, "POST", "/members/agents", url.Values{"name": {"Atlas"}, "harness": {"other"}})
	member, found := site.server.crew.Get(strings.TrimPrefix(made.Header().Get("Location"), "/members/agents/"))
	if made.Code != http.StatusSeeOther || !found || member.Model != "platform/test-model" {
		t.Fatalf("made %+v (%d)", member, made.Code)
	}
}
