package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/apitoken"
	"github.com/getlatentic/gusto/internal/connector/connectortest"
	"github.com/getlatentic/gusto/internal/roster"
	"github.com/getlatentic/gusto/internal/setup"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/gateway"
	"github.com/getlatentic/gusto/pkg/task"
)

// cy is in Business only, bob in Engineering only, and alice, the admin, in both.
const cy = "cy@example.com"

// twoWorkspaces is an organization's server behind gusto cloud whose roster has two workspaces: Engineering, the first,
// with alice and bob, and Business with alice and cy. Its apps are offered and the one called tasks is on.
func twoWorkspaces(t *testing.T) (*site, *connectortest.App) {
	t.Helper()
	here, _, app := connected(t)
	members := filepath.Join(t.TempDir(), "members.json")
	list := roster.Roster{
		Members: []roster.Member{{Email: alice, Role: "admin"}, {Email: bob, Role: "runner"}, {Email: cy, Role: "runner"}},
		Workspaces: []roster.Workspace{
			{ID: "main", Name: "Engineering", Members: []string{alice, bob}},
			{ID: "business", Name: "Business", Members: []string{alice, cy}},
		},
	}
	if err := roster.Write(members, list); err != nil {
		t.Fatal(err)
	}
	here.server.UseRoster(members)
	here.server.UseCloud(Cloud{Key: entryKey, Org: "acme-com", URL: "https://cloud.test"})
	here.handler = here.server.Handler()
	here.ask(t, "POST", "/admin/connectors/tasks/on", nil)
	return here, app
}

// inWorkspace is somebody's site with a workspace of theirs chosen.
func (s *site) inWorkspace(t *testing.T, id string) *site {
	t.Helper()
	chosen := s.ask(t, "GET", "/workspace/"+id, nil)
	seen := *s
	for _, cookie := range chosen.Result().Cookies() {
		if cookie.Name == workspaceCookie {
			seen.extra = append(seen.extra, cookie)
		}
	}
	if len(seen.extra) == 0 {
		t.Fatalf("choosing %s kept no choice: %d", id, chosen.Code)
	}
	return &seen
}

// saidIn starts a conversation and says something in it, and is the conversation, its thread and the task it made.
func saidIn(t *testing.T, who *site) (where, thread, id string) {
	t.Helper()
	where, thread = started(t, who)
	answer := who.ask(t, "POST", thread+"/say", url.Values{"said": {"go"}}).Header().Get("Location")
	if !strings.Contains(answer, "#") || strings.Contains(answer, "refused=") {
		t.Fatalf("saying something made no task: %q", answer)
	}
	id = answer[strings.LastIndex(answer, "#")+1:]
	who.wait(t, id)
	return where, thread, id
}

func TestABusinessMemberIsRefusedEngineeringsTasksRecordsAndConversations(t *testing.T) {
	here, _ := twoWorkspaces(t)
	engineer, sales, admin := here.as(t, bob), here.as(t, cy), here.as(t, alice)
	where, thread, id := saidIn(t, engineer)
	if record, _ := here.index.Get(id); record.Workspace != "" {
		t.Fatalf("a task in the first workspace names it: %q", record.Workspace)
	}
	for _, path := range []string{where, thread, "/jobs/" + id} {
		if got := sales.ask(t, "GET", path, nil).Code; got != http.StatusNotFound {
			t.Errorf("a Business member reached Engineering's %s: %d", path, got)
		}
		if got := admin.ask(t, "GET", path, nil).Code; got == http.StatusNotFound {
			t.Errorf("an admin in Engineering could not reach its %s", path)
		}
	}
	for _, path := range []string{"/jobs", "/api/jobs"} {
		if page := sales.ask(t, "GET", path, nil).Body.String(); strings.Contains(page, id) {
			t.Errorf("Engineering's task is listed to a Business member at %s", path)
		}
	}
	if page := admin.inWorkspace(t, "business").ask(t, "GET", "/api/jobs", nil).Body.String(); strings.Contains(page, id) {
		t.Errorf("an admin in Business is listed Engineering's task:\n%s", page)
	}
	if page := admin.ask(t, "GET", "/api/jobs?workspace=business", nil).Body.String(); strings.Contains(page, id) {
		t.Errorf("a list that names Business holds Engineering's task:\n%s", page)
	}
	if page := sales.ask(t, "GET", "/api/jobs?workspace=main", nil).Body.String(); strings.Contains(page, id) {
		t.Errorf("a Business member who names Engineering is listed its task:\n%s", page)
	}

	_, _, theirs := saidIn(t, sales.inWorkspace(t, "business"))
	record, _ := here.index.Get(theirs)
	spec, err := task.Read(task.Dir(filepath.Join(here.index.Root(), theirs)))
	if err != nil || record.Workspace != "business" || spec.Workspace != "business" {
		t.Fatalf("a task in Business is kept in it: record %q, spec %q, %v", record.Workspace, spec.Workspace, err)
	}
	if got := engineer.ask(t, "GET", "/jobs/"+theirs, nil).Code; got != http.StatusNotFound {
		t.Errorf("an Engineering member reached Business's task: %d", got)
	}
}

func TestAConnectionIsMadeInOneWorkspaceAndReachesNothingInAnother(t *testing.T) {
	here, app := twoWorkspaces(t)
	admin := here.as(t, alice)
	business := admin.inWorkspace(t, "business")
	if back := signInAt(t, business, app); back.StatusCode != http.StatusSeeOther {
		t.Fatalf("connecting in Business: %d", back.StatusCode)
	}
	if _, connected := here.server.connectors.Connected(alice, "business", "tasks"); !connected {
		t.Fatal("the sign-in made in Business is not kept there")
	}
	if _, connected := here.server.connectors.Connected(alice, "main", "tasks"); connected {
		t.Fatal("a sign-in made in Business is used in Engineering")
	}
	if page := admin.ask(t, "GET", "/connections", nil).Body.String(); !strings.Contains(page, `href="/connections/tasks/connect"`) {
		t.Error("in Engineering the app is not offered to connect, as if Business's sign-in were Engineering's")
	}
	spec, err := task.Read(placed(t, here, "engineering", tasks.Record{Asked: alice}, "opencode"))
	if err != nil || (spec.Setup != nil && len(spec.Setup.Connectors) > 0) {
		t.Fatalf("a task in Engineering was given Business's sign-ins: %+v, %v", spec.Setup, err)
	}
	dir := placed(t, here, "business", tasks.Record{Asked: alice, Workspace: "business"}, "opencode")
	spec, err = task.Read(dir)
	if err != nil || spec.Setup == nil || strings.Join(spec.Setup.Connectors, ",") != "tasks" {
		t.Fatalf("a task in Business was not given the sign-in made there: %+v, %v", spec.Setup, err)
	}
	var behind []gateway.Server
	text, _ := os.ReadFile(filepath.Join(dir.SetupDir(), setup.GatewayFile))
	if json.Unmarshal(text, &behind) != nil || len(behind) != 1 || behind[0].Person != alice || behind[0].Workspace != "business" {
		t.Errorf("the gateway is not told to call the app with the sign-in made in Business: %s", text)
	}
}

func TestAgentsAndRoomsStayInTheirWorkspace(t *testing.T) {
	here, _ := twoWorkspaces(t)
	business, engineer := here.as(t, alice).inWorkspace(t, "business"), here.as(t, bob)
	made := business.ask(t, "POST", "/members/agents", url.Values{"name": {"Closer"}, "harness": {"fixture"}})
	agent := made.Header().Get("Location")
	if !strings.HasPrefix(agent, "/members/agents/") {
		t.Fatalf("making an agent in Business: %d %q", made.Code, agent)
	}
	if got := engineer.ask(t, "GET", agent, nil).Code; got != http.StatusNotFound {
		t.Errorf("an Engineering member opened Business's agent: %d", got)
	}
	if page := engineer.ask(t, "GET", "/members", nil).Body.String(); strings.Contains(page, "Closer") {
		t.Error("Business's agent is listed in Engineering")
	}
	if again := here.as(t, bob).ask(t, "POST", "/members/agents", url.Values{"name": {"Closer"}, "harness": {"fixture"}}); !strings.HasPrefix(again.Header().Get("Location"), "/members/agents/") {
		t.Errorf("a name taken in Business could not be used in Engineering: %q", again.Header().Get("Location"))
	}
	room := business.ask(t, "POST", "/rooms", url.Values{"name": {"Deals"}}).Header().Get("Location")
	room = strings.TrimSuffix(room, "?add=1")
	if !strings.Contains(room, "/threads/") {
		t.Fatalf("making a room in Business went to %q", room)
	}
	id := strings.TrimPrefix(room[:strings.Index(room, "/threads/")], "/instances/")
	added := business.ask(t, "POST", "/rooms/"+id+"/members/add", url.Values{"who": {bob}}).Header().Get("Location")
	if !strings.Contains(added, "refused=") || !strings.Contains(added, url.QueryEscape("not in this room")[:10]) {
		t.Errorf("an Engineering member was put in a Business room: %q", added)
	}
	if got := engineer.ask(t, "GET", room, nil).Code; got != http.StatusNotFound {
		t.Errorf("an Engineering member opened a Business room: %d", got)
	}
}

func TestAPersonInOneWorkspaceIsShownNoWorkspacesAndCannotChooseAnother(t *testing.T) {
	here, _ := twoWorkspaces(t)
	engineer, admin := here.as(t, bob), here.as(t, alice)
	if page := engineer.ask(t, "GET", "/chat", nil).Body.String(); strings.Contains(page, "· Engineering") || strings.Contains(page, `data-workspace="business"`) {
		t.Error("a person in one workspace is shown its name")
	}
	if page := admin.ask(t, "GET", "/chat", nil).Body.String(); !strings.Contains(page, "· Engineering") || !strings.Contains(page, `data-workspace="main"`) {
		t.Error("a person in two workspaces is not shown which one they are in")
	}
	if page := admin.inWorkspace(t, "business").ask(t, "GET", "/chat", nil).Body.String(); !strings.Contains(page, "· Business") {
		t.Error("choosing Business did not put the admin in it")
	}
	tried := engineer.ask(t, "GET", "/workspace/business", nil)
	for _, cookie := range tried.Result().Cookies() {
		if cookie.Name == workspaceCookie {
			t.Fatal("a person chose a workspace they are not in")
		}
	}
}

func TestGustoCloudsSignInLinkOpensTheWorkspaceItNames(t *testing.T) {
	here, _ := twoWorkspaces(t)
	entered := here.ask(t, "GET", "/auth/entry?"+url.Values{"t": {tokenFor(t, "acme-com", alice, "admin", time.Now())}, "workspace": {"business"}}.Encode(), nil)
	chose := ""
	for _, cookie := range entered.Result().Cookies() {
		if cookie.Name == workspaceCookie {
			chose = cookie.Value
		}
	}
	if entered.Code != http.StatusSeeOther || chose != "business" {
		t.Fatalf("signing in to Business: %d, chose %q", entered.Code, chose)
	}
	entered = here.ask(t, "GET", "/auth/entry?"+url.Values{"t": {tokenFor(t, "acme-com", bob, "runner", time.Now())}, "workspace": {"business"}}.Encode(), nil)
	for _, cookie := range entered.Result().Cookies() {
		if cookie.Name == workspaceCookie {
			t.Fatal("a sign-in link chose a workspace its person is not in")
		}
	}
}

func TestAnOrganizationsTokenSendsOnlyIntoItsOwnWorkspace(t *testing.T) {
	here, _ := twoWorkspaces(t)
	file := filepath.Join(t.TempDir(), "tokens.json")
	store, err := apitoken.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	here.server.UseTokens(store)
	_, secret, err := store.CreateIn("sales-bot", []string{apitoken.Submit}, alice, "business")
	if err != nil {
		t.Fatal(err)
	}
	code, answer := here.send(t, secret, Submission{Spec: aSpec()})
	if code != http.StatusCreated {
		t.Fatalf("the token sent nothing: %d %+v", code, answer)
	}
	if record, _ := here.index.Get(answer.ID); record.Workspace != "business" {
		t.Fatalf("a task the token sent is kept in %q, not its workspace", record.Workspace)
	}
	elsewhere := aSpec()
	elsewhere.Workspace = "main"
	if code, answer := here.send(t, secret, Submission{Spec: elsewhere}); code != http.StatusForbidden || answer.ID != "" {
		t.Fatalf("a token sent into a workspace that is not its own: %d %+v", code, answer)
	}
}
