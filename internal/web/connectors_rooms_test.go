package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/setup"
	"github.com/getlatentic/gusto/pkg/gateway"
	"github.com/getlatentic/gusto/pkg/job"
)

// A room's agent uses the apps of whoever asked it, and only once that person has let that agent do so on its Apps tab: never
// another member's, never before they chose, and not after they stop it. The room's message box asks nothing, the turn says whose
// apps it used, the agent is told why it has none, and the record has each choice.
func TestARoomTurnUsesItsAskersAppsOnlyOnceTheyLetTheAgent(t *testing.T) {
	here, log, app := connected(t)
	here.ask(t, "POST", "/admin/connectors/tasks/on", nil)
	ada, bea := here.as(t, alice), here.as(t, bob)
	signInAt(t, ada, app)
	room, err := here.server.instances.StartRoom("Launch", alice)
	if err != nil {
		t.Fatal(err)
	}
	here.server.instances.SetMember(room.ID, bob, instance.RoleMember)
	nova := aCrewAgent(t, here, alice, "Nova", "opencode")
	inRoom, err := here.server.instances.AddAgent(room.ID, instance.Agent{Name: nova.Name, Harness: nova.Harness, Member: nova.ID})
	if err != nil {
		t.Fatal(err)
	}
	turn := func(id, who string) *job.Setup {
		spec, _ := job.Read(placed(t, here, id, jobs.Record{Asked: who, Agent: inRoom.ID, Instance: room.ID}, "opencode"))
		return spec.Setup
	}
	brief := func(id string) string {
		spec, _ := job.Read(job.Dir(filepath.Join(here.index.Root(), id)))
		return spec.Brief
	}
	thread := "/instances/" + room.ID + "/threads/" + here.server.instances.Threads(room.ID)[0].ID
	appsTab := "/members/agents/" + nova.ID + "?tab=apps"

	if got := turn("before", alice); got != nil {
		t.Fatalf("a room turn used the asker's apps before they let the agent: %+v", got)
	}
	if got := brief("before"); !strings.Contains(got, "has not let you use their Tasks") || !strings.Contains(got, "on your Apps page") {
		t.Errorf("the agent is not told the asker has not let it use their apps:\n%s", got)
	}
	if page := ada.ask(t, "GET", thread, nil).Body.String(); strings.Contains(page, "use your Tasks") {
		t.Error("the room's message box still asks about apps")
	}
	if page := ada.ask(t, "GET", appsTab, nil).Body.String(); !strings.Contains(page, "Your Tasks") || !strings.Contains(page, "Let Nova use them") {
		t.Fatalf("the agent's Apps tab does not offer the person's app:\n%s", page)
	}
	if page := bea.ask(t, "GET", appsTab, nil).Body.String(); !strings.Contains(page, "You have connected no apps.") {
		t.Error("the Apps tab offers apps to a person who connected nothing")
	}

	let := url.Values{"let": {"on"}}
	if got := ada.ask(t, "POST", "/members/agents/"+nova.ID+"/apps", let); got.Code != http.StatusSeeOther || got.Header().Get("Location") != appsTab {
		t.Fatalf("letting answered %d %q", got.Code, got.Header().Get("Location"))
	}
	if got := turn("after", alice); got == nil || !slices.Equal(got.Connectors, []string{"tasks"}) {
		t.Fatalf("the asker's turn was not given their app once they let the agent: %+v", got)
	}
	var behind []gateway.Server
	text, _ := os.ReadFile(filepath.Join(job.Dir(filepath.Join(here.index.Root(), "after")).SetupDir(), setup.GatewayFile))
	if json.Unmarshal(text, &behind) != nil || len(behind) != 1 || behind[0].Person != alice {
		t.Errorf("the gateway is not given the app as the asker: %s", text)
	}
	if record, _ := here.index.Get("after"); here.server.fullTurn(record, nil).Apps != nameFromAddress(alice)+"'s Tasks" {
		t.Errorf("the turn does not say whose app it could use: %q", here.server.fullTurn(record, nil).Apps)
	}
	if got := turn("bob", bob); got != nil {
		t.Errorf("another member's turn was given the asker's app: %+v", got)
	}
	if got := brief("after"); !strings.Contains(got, "you have "+nameFromAddress(alice)+"'s connected apps: Tasks.") {
		t.Errorf("the agent is not told whose apps it has for the message:\n%s", got)
	}
	if got := brief("bob"); !strings.Contains(got, "you have no connected apps") || !strings.Contains(got, "do not look for them") ||
		!strings.Contains(got, "the person whose app it is can ask for it themselves") {
		t.Errorf("the agent is not told it has no apps for another member's message:\n%s", got)
	}
	if page := ada.ask(t, "GET", appsTab, nil).Body.String(); !strings.Contains(page, "Nova may use them for what you ask it.") {
		t.Error("the Apps tab does not say the agent uses the person's apps")
	}

	ada.ask(t, "POST", "/members/agents/"+nova.ID+"/apps", url.Values{"let": {"off"}})
	if got := turn("stopped", alice); got != nil {
		t.Errorf("a turn after stopping was given the app: %+v", got)
	}
	if got := here.ask(t, "POST", "/members/agents/"+nova.ID+"/apps", let); got.Code != http.StatusNotFound {
		t.Errorf("the shared sign-in, which is nobody, answered %d", got.Code)
	}
	var said []string
	for _, event := range written(t, log) {
		if event.Kind == "connector.agent" {
			said = append(said, event.Who+" "+event.What["agent"]+" "+event.What["now"])
		}
	}
	if strings.Join(said, ", ") != alice+" Nova on, "+alice+" Nova off" {
		t.Errorf("the record has %v", said)
	}
}
