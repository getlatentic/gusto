package web

import (
	"net/url"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/usage"
)

// A room's agent runs on its owner's computer and spending, whoever mentions it: bob asks alice's Scout, and the turn counts
// against alice while it still says bob asked.
func TestATurnOfARoomsAgentCountsAgainstItsOwnerWhoeverAsks(t *testing.T) {
	work := &roomWork{}
	site, id, thread := aRoom(t, work, [2]string{"Scout", "fixture"})
	site.as(t, alice).ask(t, "POST", "/rooms/"+id+"/members", url.Values{"who": {bob}})
	site.as(t, bob).ask(t, "POST", thread+"/say", url.Values{"said": {"@Scout what next?"}})
	work.waitTurns(t, site, 1)

	var turn jobs.Record
	for _, record := range site.server.index.List() {
		if record.Instance == id && record.Agent != "" {
			turn = record
		}
	}
	if !strings.EqualFold(turn.Asked, bob) || !strings.EqualFold(turn.PaidBy, alice) || !strings.EqualFold(turn.Payer(), alice) {
		t.Fatalf("the turn was asked by %q and paid by %q", turn.Asked, turn.PaidBy)
	}
	who := usage.WhoOfTask(turn, func(string) string { return "" }, func(string, string) (string, bool) { return "", false })
	if !strings.EqualFold(who.Person, alice) {
		t.Errorf("its calls count against %q", who.Person)
	}
	if named, _ := site.server.UsageWho(turn.ID); named.Agent != "Scout" {
		t.Errorf("usage names the room's agent %q, not Scout", named.Agent)
	}
	if got := askerOf(turn.Payer()); !strings.EqualFold(got, alice) {
		t.Errorf("the month's spending by person puts it under %q", got)
	}
}
