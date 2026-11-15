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

	"github.com/getlatentic/gusto/internal/changelog"
	"github.com/getlatentic/gusto/internal/connector"
	"github.com/getlatentic/gusto/internal/connector/connectortest"
	"github.com/getlatentic/gusto/internal/setup"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/gateway"
	"github.com/getlatentic/gusto/pkg/task"
)

// connected is a site whose admin may turn on one app, Tasks, which the fake app answers for.
func connected(t *testing.T) (*site, *changelog.Log, *connectortest.App) {
	t.Helper()
	here, log, _ := budgeted(t)
	app := connectortest.New(t)
	store, err := connector.Open(connector.Config{Dir: t.TempDir(), Base: "http://acme.test", HTTP: http.DefaultClient,
		Apps: []connector.Entry{{ID: "tasks", Name: "Tasks", What: "Your tasks", URL: app.MCP()}}})
	if err != nil {
		t.Fatal(err)
	}
	here.server.UseConnectors(store)
	return here, log, app
}

// connectionsJSON is a person's Connections page's data.
type connectionsJSON struct {
	Person, HomeReset bool
	Refused, Done     string
	Connectors        []connectionJSON
	Home              *struct {
		Counted, Full bool
		Used, Limit   string
		At            int64
	}
}

type connectionJSON struct {
	ID, Name, What                             string
	Offered, Connected, SignInAgain, ForAgents bool
	Since                                      int64
}

// connections is the Connections page's data as who reads it, at the address given.
func connections(t *testing.T, who *site, path string) connectionsJSON {
	t.Helper()
	var page connectionsJSON
	who.data(t, path, &page)
	return page
}

// app is the row of the app of that id, or nothing when the page does not list it.
func (c connectionsJSON) app(id string) (connectionJSON, bool) {
	for _, row := range c.Connectors {
		if row.ID == id {
			return row, true
		}
	}
	return connectionJSON{}, false
}

// offersToConnect is an app the person may connect and has not: the page links to the app's sign-in.
func (c connectionsJSON) offersToConnect(id string) bool {
	row, listed := c.app(id)
	return listed && row.Offered && !row.Connected && !row.SignInAgain
}

// signInAt is a person connecting an app: the link sends them to the app, they sign in there, and the app sends them back.
func signInAt(t *testing.T, who *site, app *connectortest.App) *http.Response {
	t.Helper()
	leaving := who.ask(t, "GET", "/connections/tasks/connect", nil).Result()
	if leaving.StatusCode != http.StatusSeeOther || !strings.HasPrefix(leaving.Header.Get("Location"), app.Server.URL+"/authorize?") {
		t.Fatalf("connecting did not go to the app: %d %q", leaving.StatusCode, leaving.Header.Get("Location"))
	}
	state, code := app.Approve(t, leaving.Header.Get("Location"))
	return who.ask(t, "GET", "/connections/callback?"+url.Values{"state": {state}, "code": {code}}.Encode(), nil).Result()
}

// An admin turns an app on, a person signs in to it with their own account, and the person's next task reaches it through the
// gateway as them; each step is in the record.
func TestAPersonConnectsAnAppTheAdminTurnedOnAndTheirTaskReachesIt(t *testing.T) {
	here, log, app := connected(t)
	ada := here.as(t, alice)
	if page := connections(t, ada, "/connections"); len(page.Connectors) != 0 || !page.Person {
		t.Fatalf("an app is offered before an admin turned it on: %+v", page)
	}
	if got := ada.ask(t, "GET", "/connections/tasks/connect", nil).Code; got != http.StatusNotFound {
		t.Errorf("an app that is off could be connected: %d", got)
	}
	turnedOn := here.ask(t, "POST", "/admin/connectors/tasks/on", nil).Header().Get("Location")
	if turnedOn != here.server.withMessage("/admin/apps?tab=apps", "on", "Tasks") {
		t.Fatalf("turning it on went to %q", turnedOn)
	}
	if page := appsPageOf(t, here, strings.TrimPrefix(turnedOn, appsPath)); !page.app(t, "tasks").On || page.TurnedOn != "Tasks" {
		t.Errorf("the admin's page does not show it on and that people connect it: %+v", page)
	}
	if chat := ada.dataText(t, "/chat"); strings.Contains(chat, "/connections/tasks/connect") {
		t.Errorf("the chat page asks the person to connect an app; they connect it from Connections")
	}
	if page := connections(t, ada, "/connections"); !page.offersToConnect("tasks") {
		t.Fatalf("the person is not offered it: %+v", page)
	}
	back := signInAt(t, ada, app)
	connected := here.server.withMessage("/connections", "connected", "Tasks")
	if back.StatusCode != http.StatusSeeOther || back.Header.Get("Location") != connected {
		t.Fatalf("coming back went to %d %q", back.StatusCode, back.Header.Get("Location"))
	}
	if page := connections(t, ada, connected); page.Done != "Tasks" {
		t.Errorf("the page does not say the person signed in: %+v", page)
	} else if row, _ := page.app("tasks"); !row.Connected || row.Since == 0 || row.Name != "Tasks" || row.What != "Your tasks" {
		t.Errorf("the page does not show the person signed in, and since when: %+v", row)
	}
	if forged := connections(t, ada, "/connections?connected=Evil"); forged.Done != "" {
		t.Errorf("a message this server did not seal is shown: %+v", forged)
	}

	dir := placed(t, here, "own", tasks.Record{Asked: alice}, "opencode")
	spec, err := task.Read(dir)
	if err != nil || spec.Setup == nil || !slices.Equal(spec.Setup.Connectors, []string{"tasks"}) || !slices.Equal(spec.Setup.Gateway, []string{"tasks"}) {
		t.Fatalf("the task was not given the app: %+v %v", spec.Setup, err)
	}
	var behind []gateway.Server
	text, _ := os.ReadFile(filepath.Join(dir.SetupDir(), setup.GatewayFile))
	if json.Unmarshal(text, &behind) != nil || len(behind) != 1 || behind[0].Connector != "tasks" || behind[0].URL != app.MCP() || behind[0].Person != alice {
		t.Errorf("the gateway is not given the app as the person: %s", text)
	}
	if strings.Contains(string(text), "access-") {
		t.Errorf("the task's files hold the person's token: %s", text)
	}

	var said []string
	for _, event := range written(t, log) {
		if strings.HasPrefix(event.Kind, "connector.") {
			said = append(said, event.Who+" "+event.Kind+" "+event.What["connector"])
		}
	}
	if strings.Join(said, ", ") != sharedSignIn+" connector.enabled Tasks, "+alice+" connector.connected Tasks" {
		t.Errorf("the record has %v", said)
	}
}

// A room's agent works for everyone in the room, an agent that cannot reach the gateway has no way to call the app, and a person
// who disconnected has given it back: none of them is given it.
func TestAnAppIsGivenOnlyToItsOwnersOwnTasksWhileTheyAreSignedIn(t *testing.T) {
	here, _, app := connected(t)
	here.ask(t, "POST", "/admin/connectors/tasks/on", nil)
	ada := here.as(t, alice)
	signInAt(t, ada, app)
	for name, record := range map[string]tasks.Record{
		"room":    {Asked: alice, Agent: "atlas", Instance: "room"},
		"paid":    {Asked: alice, PaidBy: bob},
		"someone": {Asked: bob},
	} {
		if spec, _ := task.Read(placed(t, here, name, record, "opencode")); spec.Setup != nil {
			t.Errorf("the %s task was given %+v", name, spec.Setup)
		}
	}
	if spec, _ := task.Read(placed(t, here, "pi", tasks.Record{Asked: alice}, "pi")); spec.Setup != nil {
		t.Errorf("an agent that reads its servers from its own file was given %+v", spec.Setup)
	}
	ada.ask(t, "POST", "/connections/tasks/disconnect", nil)
	if spec, _ := task.Read(placed(t, here, "after", tasks.Record{Asked: alice}, "opencode")); spec.Setup != nil {
		t.Errorf("a task after disconnecting was given %+v", spec.Setup)
	}
	here.ask(t, "POST", "/admin/connectors/tasks/off", nil)
	if got := ada.ask(t, "GET", "/connections/tasks/connect", nil).Code; got != http.StatusNotFound {
		t.Errorf("an app turned off could still be connected: %d", got)
	}
}

// The shared sign-in is nobody's own, and an app that needs a client the operator has not registered cannot be turned on.
func TestTheSharedSignInConnectsNothingAndAnAppWithoutItsClientStaysOff(t *testing.T) {
	here, _, _ := connected(t)
	here.ask(t, "POST", "/admin/connectors/tasks/on", nil)
	if got := here.ask(t, "GET", "/connections/tasks/connect", nil).Header().Get("Location"); !strings.Contains(got, "refused=") {
		t.Errorf("the shared sign-in set out to connect: %q", got)
	}
	store, err := connector.Open(connector.Config{Dir: t.TempDir(), Base: "http://acme.test",
		Apps: []connector.Entry{{ID: "crm", Name: "CRM", URL: "https://crm.example.com/mcp", NeedsClient: true}}})
	if err != nil {
		t.Fatal(err)
	}
	here.server.UseConnectors(store)
	if got := here.ask(t, "POST", "/admin/connectors/crm/on", nil).Header().Get("Location"); !strings.Contains(got, "refused=") {
		t.Errorf("an app with no client was turned on: %q", got)
	}
	if crm := appsPageOf(t, here, "").app(t, "crm"); crm.Usable || crm.On {
		t.Errorf("the admin is not told it needs setting up: %+v", crm)
	}
}

// The client document is public, since the app reads it with no sign-in here, and names where sign-ins come back to.
func TestTheClientDocumentIsServedToAnybody(t *testing.T) {
	here, _, _ := connected(t)
	stranger := *here
	stranger.cookie = nil
	answer := stranger.ask(t, "GET", "/connectors/client.json", nil)
	var document map[string]any
	if answer.Code != http.StatusOK || json.Unmarshal(answer.Body.Bytes(), &document) != nil ||
		document["client_id"] != "http://acme.test/connectors/client.json" {
		t.Errorf("the document: %d %s", answer.Code, answer.Body)
	}
}

// placed is a task placed as the record says, on the harness named, and where it was put.
func placed(t *testing.T, here *site, id string, record tasks.Record, harness string) task.Dir {
	t.Helper()
	record.ID = id
	if err := here.server.place(task.Spec{Brief: "list my tasks", Repo: "https://github.com/acme/repo", Harness: harness}, record); err != nil {
		t.Fatal(err)
	}
	return task.Dir(filepath.Join(here.index.Root(), id))
}
