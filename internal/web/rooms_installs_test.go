package web

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/crew"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/pkg/spend"
)

// aRoomWithBob is a room alice runs, with Scout, alice's agent, and bob as a member.
func aRoomWithBob(t *testing.T, work *roomWork) (*site, string, string) {
	t.Helper()
	site, id, thread := aRoom(t, work, [2]string{"Scout", "fixture"})
	site.as(t, alice).ask(t, "POST", "/rooms/"+id+"/members", url.Values{"who": {bob}})
	return site, id, thread
}

// bobsAgentIn is Atlas, bob's agent, proposed by bob and accepted by alice.
func bobsAgentIn(t *testing.T, site *site, id string) instance.Agent {
	t.Helper()
	atlas := aCrewAgent(t, site, bob, "Atlas", "fixture")
	site.as(t, bob).ask(t, "POST", "/rooms/"+id+"/members/add", url.Values{"agent": {atlas.ID}})
	held, _ := site.server.instances.Get(id)
	agent, found := held.AgentCalled("Atlas")
	if !found {
		t.Fatal("bob's proposal was not kept")
	}
	site.as(t, alice).ask(t, "POST", "/rooms/"+id+"/agents/"+agent.ID+"/accept", nil)
	return agent
}

func refusedTo(t *testing.T, answer string) string {
	t.Helper()
	at, err := url.Parse(answer)
	if err != nil {
		t.Fatal(err)
	}
	return at.Query().Get("refused")
}

func TestAMembersOwnAgentWaitsForTheRoomsAdminAndIsNotAskedUntilThen(t *testing.T) {
	work := &roomWork{}
	site, id, thread := aRoomWithBob(t, work)
	his, mine := site.as(t, bob), site.as(t, alice)
	atlas := aCrewAgent(t, site, bob, "Atlas", "fixture")
	proposed := his.ask(t, "POST", "/rooms/"+id+"/members/add", url.Values{"agent": {atlas.ID}, "allowance": {"5"}})
	if why := refusedTo(t, proposed.Header().Get("Location")); why != "" {
		t.Fatalf("bob could not propose his agent: %s", why)
	}
	held, _ := site.server.instances.Get(id)
	agent, _ := held.AgentCalled("Atlas")
	if !agent.Pending || agent.ProposedBy != bob || agent.AllowanceUSD == nil || *agent.AllowanceUSD != 5 {
		t.Fatalf("the proposal is %+v", agent)
	}
	his.ask(t, "POST", thread+"/say", url.Values{"said": {"@Atlas what next?"}})
	if got := site.as(t, bob).ask(t, "GET", "/rooms/"+id+"/members", nil); got.Code >= 400 {
		t.Fatalf("%d", got.Code)
	}
	time.Sleep(50 * time.Millisecond)
	if heard := work.turns(); len(heard) != 0 {
		t.Fatalf("an agent nobody accepted was asked, or another in its place: %+v", heard)
	}
	if notes := site.server.instances.Latest(id).Notes; len(notes) != 2 || notes[0].Text != "@Atlas what next?" || !strings.Contains(notes[1].Text, "Atlas is waiting for an admin") {
		t.Errorf("the room was told %+v", notes)
	}
	adminSees := mine.ask(t, "GET", "/rooms/"+id+"/members", nil).Header().Get("Location")
	page := mine.ask(t, "GET", adminSees, nil).Body.String()
	if !strings.Contains(page, `action="/rooms/`+id+`/agents/`+agent.ID+`/accept"`) || !strings.Contains(page, "$5.00 a month here") {
		t.Error("the room's admin is not offered the proposal to accept, with its allowance")
	}
	if page := his.ask(t, "GET", adminSees, nil).Body.String(); !strings.Contains(page, "Waiting for an admin") || strings.Contains(page, "/accept") {
		t.Error("the member who proposed it is not told it waits, or is offered to accept it")
	}
	if got := his.ask(t, "POST", "/rooms/"+id+"/agents/"+agent.ID+"/accept", nil); got.Code != 403 {
		t.Errorf("a member accepted an agent: %d", got.Code)
	}
	mine.ask(t, "POST", "/rooms/"+id+"/agents/"+agent.ID+"/accept", nil)
	his.ask(t, "POST", thread+"/say", url.Values{"said": {"@Atlas what next?"}})
	if heard := work.waitTurns(t, site, 1); heard[0].Agent != agent.ID {
		t.Fatalf("heard %+v", heard)
	}
}

func TestAMemberProposesOnlyTheirOwnAgentAndAnAgentsOwnerMustBeInTheRoom(t *testing.T) {
	site, id, _ := aRoomWithBob(t, &roomWork{})
	carols := aCrewAgent(t, site, "carol@example.com", "Echo", "fixture")
	if _, err := site.server.crew.Change(carols.ID, func(held *crew.Agent) error { held.Access = []string{alice, bob}; return nil }); err != nil {
		t.Fatal(err)
	}
	asked := site.as(t, bob).ask(t, "POST", "/rooms/"+id+"/members/add", url.Values{"agent": {carols.ID}})
	if why := refusedTo(t, asked.Header().Get("Location")); !strings.Contains(why, "only an agent of your own") {
		t.Errorf("bob proposing carol's agent was told %q", why)
	}
	added := site.as(t, alice).ask(t, "POST", "/rooms/"+id+"/members/add", url.Values{"agent": {carols.ID}})
	if why := refusedTo(t, added.Header().Get("Location")); !strings.Contains(why, "carol is not in this room") {
		t.Errorf("the admin adding the agent of somebody not in the room was told %q", why)
	}
}

func TestAPersonWhoLeavesTakesTheirAgentsAndTheirTurnsStop(t *testing.T) {
	work := &roomWork{hold: make(chan struct{})}
	defer close(work.hold)
	site, id, thread := aRoomWithBob(t, work)
	his := site.as(t, bob)
	agent := bobsAgentIn(t, site, id)
	his.ask(t, "POST", thread+"/say", url.Values{"said": {"@Atlas take a long time"}})
	lane := jobs.Record{Instance: id, Agent: agent.ID}.Lane()
	if len(site.server.index.Unfinished(lane)) != 1 {
		t.Fatal("the turn was not started")
	}

	his.ask(t, "POST", "/rooms/"+id+"/members/remove", url.Values{"who": {bob}})

	held, _ := site.server.instances.Get(id)
	if _, still := held.AgentCalled("Atlas"); still {
		t.Error("bob's agent stayed in a room bob left")
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(site.server.index.Unfinished(lane)) > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if left := site.server.index.Unfinished(lane); len(left) != 0 {
		t.Errorf("the agent's turn kept going: %+v", left)
	}
}

func TestAnAgentsOwnerMayTakeItOutOfARoomTheyDoNotRun(t *testing.T) {
	site, id, _ := aRoomWithBob(t, &roomWork{})
	agent := bobsAgentIn(t, site, id)
	if got := site.as(t, bob).ask(t, "POST", "/rooms/"+id+"/agents/"+agent.ID+"/remove", nil); got.Code != 303 {
		t.Fatalf("bob taking his agent out gave %d", got.Code)
	}
	if got := site.as(t, bob).ask(t, "POST", "/rooms/"+id+"/agents/a1/remove", nil); got.Code != 403 {
		t.Errorf("bob took alice's agent out: %d", got.Code)
	}
}

func TestAnAgentPastItsAllowanceIsNotAskedAndTheRoomIsTold(t *testing.T) {
	work := &roomWork{}
	site, id, thread := aRoomWithBob(t, work)
	ledger := spend.NewLedger(spend.Policy{})
	site.server.UseLedger(ledger)
	atlas := aCrewAgent(t, site, alice, "Atlas", "fixture")
	site.as(t, alice).ask(t, "POST", "/rooms/"+id+"/members/add", url.Values{"agent": {atlas.ID}, "allowance": {"1"}})
	held, _ := site.server.instances.Get(id)
	agent, _ := held.AgentCalled("Atlas")
	if agent.AllowanceUSD == nil {
		t.Fatal("the allowance was not kept")
	}
	dollar := 1.0
	ledger.Begin("earlier", spend.Terms{Allowance: spend.AllowanceOf(id, agent.ID)})
	ledger.Count("earlier", spend.Call{Path: "/v1/chat/completions", Model: "m", AtUnix: time.Now().Unix(), Status: 200, USD: &dollar})

	site.as(t, bob).ask(t, "POST", thread+"/say", url.Values{"said": {"@Atlas and @Scout, go"}})

	heard := work.waitTurns(t, site, 1)
	if len(heard) != 1 || heard[0].Agent != "a1" {
		t.Fatalf("heard %+v", heard)
	}
	notes := site.server.instances.Latest(id).Notes
	if len(notes) == 0 || !strings.Contains(notes[len(notes)-1].Text, "Atlas has spent $1.00 of its $1.00 allowance") {
		t.Errorf("the room was told %+v", notes)
	}
	if spec := work.turns(); len(spec) != 1 {
		t.Errorf("turns %+v", spec)
	}
}

func TestAnAllowanceIsDollarsAMonth(t *testing.T) {
	for _, bad := range []string{"-1", "lots", "1e9"} {
		if _, err := allowanceOf(bad); err == nil {
			t.Errorf("%q was taken as an allowance", bad)
		}
	}
	if got, err := allowanceOf(" $12.50 "); err != nil || *got != 12.5 {
		t.Errorf("$12.50 read as %v %v", got, err)
	}
	if got, err := allowanceOf(""); err != nil || got != nil {
		t.Errorf("no allowance read as %v %v", got, err)
	}
}
