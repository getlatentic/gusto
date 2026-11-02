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

// A room's agent uses an app of whoever asked it only as that person chose, app by app: an app left on inherit follows their
// default on Connections, and a choice for the agent wins over it either way. Never another member's apps, and the agent is told
// which of the asker's apps it was not let use. Each choice is recorded.
func TestARoomTurnUsesEachOfItsAskersAppsAsTheyChoseOrTheirDefault(t *testing.T) {
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
	turn := func(id, who string) []string {
		spec, _ := job.Read(placed(t, here, id, jobs.Record{Asked: who, Agent: inRoom.ID, Instance: room.ID}, "opencode"))
		if spec.Setup == nil {
			return nil
		}
		return spec.Setup.Connectors
	}
	brief := func(id string) string {
		spec, _ := job.Read(job.Dir(filepath.Join(here.index.Root(), id)))
		return spec.Brief
	}
	thread := "/instances/" + room.ID + "/threads/" + here.server.instances.Threads(room.ID)[0].ID
	appsTab := "/members/agents/" + nova.ID + "?tab=apps"
	choose := func(choice string) {
		t.Helper()
		if got := ada.ask(t, "POST", "/members/agents/"+nova.ID+"/apps", url.Values{"app": {"tasks"}, "choice": {choice}}); got.Code != http.StatusSeeOther || got.Header().Get("Location") != appsTab {
			t.Fatalf("choosing %s answered %d %q", choice, got.Code, got.Header().Get("Location"))
		}
	}
	byDefault := func(on string) {
		t.Helper()
		if got := ada.ask(t, "POST", "/connections/tasks/agents", url.Values{"on": {on}}); got.Code != http.StatusSeeOther {
			t.Fatalf("setting the default %s answered %d", on, got.Code)
		}
	}

	if got := turn("before", alice); got != nil {
		t.Fatalf("a room turn used the asker's app before they let it: %v", got)
	}
	if got := brief("before"); !strings.Contains(got, "has not let you use their Tasks") || !strings.Contains(got, "on your Apps page") {
		t.Errorf("the agent is not told the asker has not let it use their app:\n%s", got)
	}
	if page := ada.ask(t, "GET", thread, nil).Body.String(); strings.Contains(page, "use your Tasks") {
		t.Error("the room's message box asks about apps")
	}
	if page := ada.ask(t, "GET", appsTab, nil).Body.String(); !strings.Contains(page, ">Tasks<") || !strings.Contains(page, "Inherit (off)") {
		t.Fatalf("the agent's Apps tab does not offer the person's app with their default:\n%s", page)
	}
	if page := bea.ask(t, "GET", appsTab, nil).Body.String(); !strings.Contains(page, "No apps connected.") {
		t.Error("the Apps tab offers apps to a person who connected nothing")
	}

	byDefault("on")
	if page := ada.ask(t, "GET", "/connections", nil).Body.String(); !strings.Contains(page, `<option value="on" selected>On</option>`) {
		t.Error("Connections does not show the default as on")
	}
	if got := turn("inherited", alice); !slices.Equal(got, []string{"tasks"}) {
		t.Fatalf("an agent left on inherit was not given the app the person's default allows: %v", got)
	}
	var behind []gateway.Server
	text, _ := os.ReadFile(filepath.Join(job.Dir(filepath.Join(here.index.Root(), "inherited")).SetupDir(), setup.GatewayFile))
	if json.Unmarshal(text, &behind) != nil || len(behind) != 1 || behind[0].Person != alice {
		t.Errorf("the gateway is not given the app as the asker: %s", text)
	}
	if record, _ := here.index.Get("inherited"); here.server.fullTurn(record, nil).Apps != nameFromAddress(alice)+"'s Tasks" {
		t.Errorf("the turn does not say whose app it could use: %q", here.server.fullTurn(record, nil).Apps)
	}
	if got := brief("inherited"); !strings.Contains(got, "you have "+nameFromAddress(alice)+"'s connected apps: Tasks.") {
		t.Errorf("the agent is not told whose apps it has for the message:\n%s", got)
	}

	choose("deny")
	if got := turn("denied", alice); got != nil {
		t.Errorf("an app the person kept from the agent was given though their default is on: %v", got)
	}
	choose("allow")
	byDefault("off")
	if got := turn("allowed", alice); !slices.Equal(got, []string{"tasks"}) {
		t.Errorf("an app the person allowed the agent was not given though their default is off: %v", got)
	}
	if page := ada.ask(t, "GET", appsTab, nil).Body.String(); !strings.Contains(page, `<option value="allow" selected>Allow</option>`) {
		t.Error("the Apps tab does not show the choice")
	}
	if got := turn("bob", bob); got != nil {
		t.Errorf("another member's turn was given the asker's app: %v", got)
	}
	if got := brief("bob"); !strings.Contains(got, "the person whose app it is can ask for it themselves") {
		t.Errorf("the agent is not told it has no apps for another member's message:\n%s", got)
	}
	choose("inherit")
	if got := turn("back", alice); got != nil {
		t.Errorf("back on inherit, with the default off, the app was given: %v", got)
	}

	if got := here.ask(t, "POST", "/members/agents/"+nova.ID+"/apps", url.Values{"app": {"tasks"}, "choice": {"allow"}}); got.Code != http.StatusNotFound {
		t.Errorf("the shared sign-in, which is nobody, chose for an agent: %d", got.Code)
	}
	if got := ada.ask(t, "POST", "/members/agents/"+nova.ID+"/apps", url.Values{"app": {"nothing"}, "choice": {"allow"}}); !strings.Contains(got.Header().Get("Location"), "refused") {
		t.Error("a choice for an app that is not offered was taken")
	}
	var said []string
	for _, event := range written(t, log) {
		switch event.Kind {
		case "connector.agent":
			said = append(said, event.What["agent"]+" "+event.What["now"])
		case "connector.default":
			said = append(said, "default "+event.What["now"])
		}
	}
	if strings.Join(said, ", ") != "default on, Nova deny, Nova allow, default off, Nova inherit" {
		t.Errorf("the record has %v", said)
	}
}
