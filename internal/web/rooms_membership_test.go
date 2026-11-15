package web

import (
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/tasks"
)

func TestDeletingAnAgentStopsItsTurnsInTheRoomsItWasIn(t *testing.T) {
	work := &roomWork{hold: make(chan struct{})}
	defer close(work.hold)
	site, id, thread := aRoomWithBob(t, work)
	his := site.as(t, bob)
	agent := bobsAgentIn(t, site, id)
	his.ask(t, "POST", thread+"/say", url.Values{"said": {"@Atlas take a long time"}})
	lane := tasks.Record{Instance: id, Agent: agent.ID}.Lane()
	if len(site.server.index.Unfinished(lane)) != 1 {
		t.Fatal("the turn was not started")
	}

	if got := his.ask(t, "POST", "/members/agents/"+agent.Member+"/remove", nil); got.Code != http.StatusSeeOther {
		t.Fatalf("deleting the agent gave %d", got.Code)
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(site.server.index.Unfinished(lane)) > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if left := site.server.index.Unfinished(lane); len(left) != 0 {
		t.Errorf("a deleted agent's turn kept going: %+v", left)
	}
}

func TestAnAddThatIsRefusedAddsNobody(t *testing.T) {
	const carol = "carol@example.com"
	site, id, _ := aRoom(t, &roomWork{}, [2]string{"Scout", "fixture"})
	added := site.as(t, alice).ask(t, "POST", "/rooms/"+id+"/members/add", url.Values{"who": {carol}, "agent": {"no-such-agent"}})
	if why := refusedTo(t, added.Header().Get("Location")); why == "" {
		t.Fatalf("an add with an agent nobody may work with was not refused: %s", added.Header().Get("Location"))
	}
	if held, _ := site.server.instances.Get(id); held.RoleOf(carol) != "" {
		t.Error("the person of a refused add was added")
	}
}

func TestMessagingAnAgentTwiceAtOnceOpensOneRoom(t *testing.T) {
	site := openOffering(t, twoHarnesses())
	member := aCrewAgent(t, site, alice, "Atlas", "other")
	mine := site.as(t, alice)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mine.ask(t, "POST", "/members/agents/"+member.ID+"/message", nil)
		}()
	}
	wg.Wait()
	direct := 0
	for _, held := range site.server.instances.RoomsOf(alice) {
		if held.Direct {
			direct++
		}
	}
	if direct != 1 {
		t.Errorf("messaging an agent eight times at once made %d rooms with it", direct)
	}
}
