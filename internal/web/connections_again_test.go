package web

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/task"
)

// A sign-in the app no longer takes is not shown as connected and not given to a task: Connections, the admin's page and the
// agent's Apps tab say it needs signing in again, and the agent in a room is told that and not that nothing is disconnected.
func TestASignInThatNeedsMakingAgainSaysSoWhereverAppsAreShown(t *testing.T) {
	here, _, app := connected(t)
	app.GivesNoRefresh()
	app.Lasts(time.Second)
	here.ask(t, "POST", "/admin/connectors/tasks/on", nil)
	ada := here.as(t, alice)
	signInAt(t, ada, app)
	time.Sleep(1100 * time.Millisecond)

	page := ada.ask(t, "GET", "/connections", nil).Body.String()
	if !strings.Contains(page, ">sign in again<") || !strings.Contains(page, `href="/connections/tasks/connect">Sign in again<`) {
		t.Errorf("Connections does not ask the person to sign in again:\n%s", page)
	}
	if strings.Contains(page, `action="/connections/tasks/agents"`) {
		t.Error("Connections offers a default for agents to an app that does not work")
	}
	if !strings.Contains(page, `action="/connections/tasks/disconnect"`) {
		t.Error("the person cannot end a sign-in that needs making again")
	}
	if admin := here.ask(t, "GET", "/admin/apps?tab=apps", nil).Body.String(); !strings.Contains(admin, "1 person needs to sign in again") {
		t.Errorf("the admin's page does not say who needs to sign in again:\n%s", admin)
	}
	if spec, _ := task.Read(placed(t, here, "own", tasks.Record{Asked: alice}, "opencode")); spec.Setup != nil {
		t.Errorf("a task was given an app whose sign-in no longer works: %+v", spec.Setup)
	}

	room, err := here.server.instances.StartRoom("Launch", alice, "")
	if err != nil {
		t.Fatal(err)
	}
	nova := aCrewAgent(t, here, alice, "Nova", "opencode")
	inRoom, err := here.server.instances.AddAgent(room.ID, instance.Agent{Name: nova.Name, Harness: nova.Harness, Member: nova.ID})
	if err != nil {
		t.Fatal(err)
	}
	if tab := ada.ask(t, "GET", "/members/agents/"+nova.ID+"?tab=apps", nil).Body.String(); !strings.Contains(tab, ">sign in again<") || strings.Contains(tab, `name="choice"`) {
		t.Errorf("the agent's Apps tab does not ask for a sign-in, or offers a choice for an app that does not work:\n%s", tab)
	}
	placed(t, here, "room", tasks.Record{Asked: alice, Agent: inRoom.ID, Instance: room.ID}, "opencode")
	brief, _ := task.Read(task.Dir(filepath.Join(here.index.Root(), "room")))
	if !strings.Contains(brief.Brief, "sign-in to Tasks needs signing in again") || strings.Contains(brief.Brief, "Nothing is disconnected, so never suggest reconnecting") {
		t.Errorf("the agent is not told the sign-in needs making again:\n%s", brief.Brief)
	}

}

// An admin turning an app off does not take a person's own sign-in out of their hands: Connections still lists the app, with
// only Disconnect, and ending the sign-in works, while connecting the app again stays refused until an admin turns it on.
func TestAPersonEndsTheirSignInToAnAppAnAdminTurnedOff(t *testing.T) {
	here, log, app := connected(t)
	here.ask(t, "POST", "/admin/connectors/tasks/on", nil)
	ada, bea := here.as(t, alice), here.as(t, bob)
	signInAt(t, ada, app)
	here.ask(t, "POST", "/admin/connectors/tasks/off", nil)

	page := ada.ask(t, "GET", "/connections", nil).Body.String()
	if !strings.Contains(page, ">turned off<") || !strings.Contains(page, `action="/connections/tasks/disconnect"`) ||
		strings.Contains(page, `action="/connections/tasks/agents"`) || strings.Contains(page, `href="/connections/tasks/connect"`) {
		t.Errorf("a turned-off app the person is signed in to is not shown with only Disconnect:\n%s", page)
	}
	if page := bea.ask(t, "GET", "/connections", nil).Body.String(); strings.Contains(page, "Tasks") {
		t.Errorf("a turned-off app is listed for somebody who never signed in to it:\n%s", page)
	}
	if got := ada.ask(t, "GET", "/connections/tasks/connect", nil).Code; got != http.StatusNotFound {
		t.Errorf("connecting an app that is off answered %d", got)
	}
	if got := ada.ask(t, "POST", "/connections/tasks/disconnect", nil).Code; got != http.StatusSeeOther {
		t.Fatalf("ending a sign-in to an app that is off answered %d", got)
	}
	if page := ada.ask(t, "GET", "/connections", nil).Body.String(); strings.Contains(page, "Tasks") {
		t.Errorf("the app is still listed after the sign-in ended:\n%s", page)
	}
	if ended := ofKind(written(t, log), changelog.ConnectorDisconnected); len(ended) != 1 || ended[0].Who != alice {
		t.Errorf("the change record holds %+v", ended)
	}
	if got := bea.ask(t, "POST", "/connections/nothing/disconnect", nil).Code; got != http.StatusNotFound {
		t.Errorf("disconnecting an app nobody knows answered %d", got)
	}
}
