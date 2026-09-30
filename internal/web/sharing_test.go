package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/getlatentic/haas/internal/apps"
)

// gatewayFake stands in for the gateway: it answers for an app, and remembers what it was sent.
type gatewayFake struct {
	*httptest.Server
	mu   sync.Mutex
	seen []*http.Request
}

func newGateway(t *testing.T) *gatewayFake {
	t.Helper()
	g := &gatewayFake{}
	g.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		g.seen = append(g.seen, r.Clone(context.Background()))
		g.mu.Unlock()
		switch r.URL.Path {
		case "/go":
			http.Redirect(w, r, "/somewhere?x=1", http.StatusFound)
		default:
			w.Header().Set("Set-Cookie", "session=stolen; Path=/")
			w.Header().Set("Strict-Transport-Security", "max-age=999999")
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<h1>the app</h1> " + r.URL.RequestURI()))
		}
	}))
	t.Cleanup(g.Close)
	return g
}

func (g *gatewayFake) last() *http.Request {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.seen) == 0 {
		return nil
	}
	return g.seen[len(g.seen)-1]
}

// runnerFake starts an app by saying it answers at the gateway, or refuses.
type runnerFake struct {
	gateway *gatewayFake
	mu      sync.Mutex
	started []apps.App
	stopped []apps.App
	fail    error
}

func (r *runnerFake) Start(_ context.Context, app apps.App) (apps.App, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.started = append(r.started, app)
	if r.fail != nil {
		return apps.App{}, r.fail
	}
	app.Sandbox, app.Host, app.Target = "c-1111-2222", "default--c-1111-2222--a"+app.ID+".openshell.localhost:18080", strings.TrimPrefix(r.gateway.URL, "http://")
	return app, nil
}

func (r *runnerFake) Stop(_ context.Context, app apps.App) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = append(r.stopped, app)
	return nil
}

func sharingSite(t *testing.T) (*site, *runnerFake, *gatewayFake) {
	t.Helper()
	site := open(t)
	site.signIn(t)
	gateway := newGateway(t)
	runner := &runnerFake{gateway: gateway}
	store, err := apps.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	site.server.UseApps(store, runner)
	return site, runner, gateway
}

func sharedApp(t *testing.T, site *site, where string) (id string) {
	t.Helper()
	answer := site.ask(t, "POST", where+"/apps", url.Values{"command": {"node server.js"}, "port": {"8080"}})
	if answer.Code != http.StatusSeeOther || strings.Contains(answer.Header().Get("Location"), "refused") {
		t.Fatalf("sharing an app: %d %q", answer.Code, answer.Header().Get("Location"))
	}
	site.server.Wait()
	all, _ := site.server.apps.Of(strings.TrimPrefix(where, "/instances/"))
	if len(all) == 0 {
		t.Fatal("no app was made")
	}
	return all[0].ID
}

func TestAnAppAnAgentBuiltIsSharedOnceItAnswersAndOnlyTheOrganisationsMembersOpenIt(t *testing.T) {
	site, runner, gateway := sharingSite(t)
	where := startedInstance(t, site)
	id := sharedApp(t, site, where)

	app, _ := site.server.apps.Get(id)
	if app.State != apps.Live || app.Command != "node server.js" || app.Port != 8080 || len(runner.started) != 1 {
		t.Fatalf("%+v", app)
	}
	if page := site.ask(t, "GET", where, nil).Body.String(); !strings.Contains(page, `href="/apps/`+id+`/"`) || !strings.Contains(page, "node server.js") {
		t.Errorf("the agent's page links the app:\n%s", page)
	}

	answer := site.ask(t, "GET", "/apps/"+id+"/some/page?x=1&y=2", nil)
	if answer.Code != http.StatusOK || !strings.Contains(answer.Body.String(), "<h1>the app</h1> /some/page?x=1&y=2") {
		t.Fatalf("the app answered %d: %s", answer.Code, answer.Body)
	}
	asked := gateway.last()
	if asked.Host != app.Host {
		t.Errorf("the gateway is asked by the name it knows the app by: %q", asked.Host)
	}
	if asked.Header.Get("Cookie") != "" || asked.Header.Get("Authorization") != "" {
		t.Errorf("the app was sent the person's credentials: %v", asked.Header)
	}
	header := answer.Header()
	if header.Get("Set-Cookie") != "" || header.Get("Strict-Transport-Security") != "" {
		t.Errorf("the app set something on the person's origin: %v", header)
	}
	policy := header.Values("Content-Security-Policy")
	if len(policy) != 1 || !strings.HasPrefix(policy[0], "sandbox allow-scripts") || strings.Contains(policy[0], "allow-same-origin") || strings.Contains(policy[0], "default-src") {
		t.Errorf("the page runs sandboxed, with no origin of its own and with none of this server's policy: %q", policy)
	}
	if redirect := site.ask(t, "GET", "/apps/"+id+"/go", nil); redirect.Header().Get("Location") != "/apps/"+id+"/somewhere?x=1" {
		t.Errorf("a redirect to the app's own path stays in the app: %q", redirect.Header().Get("Location"))
	}
	if bare := site.ask(t, "GET", "/apps/"+id, nil); bare.Header().Get("Location") != "/apps/"+id+"/" {
		t.Errorf("%d %q", bare.Code, bare.Header().Get("Location"))
	}

	stranger := *site
	stranger.cookie = nil
	if got := stranger.ask(t, "GET", "/apps/"+id+"/", nil); got.Code != http.StatusSeeOther || got.Header().Get("Location") != "/login" {
		t.Errorf("a stranger opened the app: %d %q", got.Code, got.Header().Get("Location"))
	}
}

func TestAnotherOrganisationsServerHasNoSuchApp(t *testing.T) {
	first, _, _ := sharingSite(t)
	id := sharedApp(t, first, startedInstance(t, first))
	second, _, _ := sharingSite(t)
	if got := second.ask(t, "GET", "/apps/"+id+"/", nil); got.Code != http.StatusNotFound {
		t.Errorf("another organisation opened it: %d", got.Code)
	}
}

func TestAnAppThatDoesNotAnswerIsFailedWithWhatItSaidAndIsNotShared(t *testing.T) {
	site, runner, _ := sharingSite(t)
	runner.fail = errors.New("the app did not answer in 90 s: Error: Cannot find module 'express'")
	where := startedInstance(t, site)
	id := sharedApp(t, site, where)
	app, _ := site.server.apps.Get(id)
	if app.State != apps.Failed || !strings.Contains(app.Note, "Cannot find module 'express'") {
		t.Fatalf("%+v", app)
	}
	if page := site.ask(t, "GET", where, nil).Body.String(); !strings.Contains(page, "It did not start") || !strings.Contains(page, "Cannot find module") || strings.Contains(page, `href="/apps/`+id) {
		t.Errorf("the page says what the app wrote and offers no link:\n%s", page)
	}
	if got := site.ask(t, "GET", "/apps/"+id+"/", nil); got.Code != http.StatusNotFound {
		t.Errorf("an app that never answered was opened: %d", got.Code)
	}
}

func TestAnAppThatStopsAnsweringIsStoppedAndNoLongerOpens(t *testing.T) {
	site, _, gateway := sharingSite(t)
	id := sharedApp(t, site, startedInstance(t, site))
	gateway.Close()
	if got := site.ask(t, "GET", "/apps/"+id+"/", nil); got.Code != http.StatusBadGateway {
		t.Fatalf("%d", got.Code)
	}
	if app, _ := site.server.apps.Get(id); app.State != apps.Stopped {
		t.Errorf("%+v", app)
	}
	if got := site.ask(t, "GET", "/apps/"+id+"/", nil); got.Code != http.StatusNotFound {
		t.Errorf("a stopped app opened: %d", got.Code)
	}
}

func TestSharingRefusesWhatCannotWorkAndWhatIsSomebodyElses(t *testing.T) {
	site, runner, _ := sharingSite(t)
	mine, theirs := site.as(t, alice), site.as(t, bob)
	where, _ := started(t, mine)
	for name, form := range map[string]url.Values{
		"no command":             {"command": {""}, "port": {"8080"}},
		"two lines":              {"command": {"node a.js\nrm -rf /"}, "port": {"8080"}},
		"a port that is not one": {"command": {"node a.js"}, "port": {"http"}},
		"a privileged port":      {"command": {"node a.js"}, "port": {"80"}},
		"a port past the end":    {"command": {"node a.js"}, "port": {"70000"}},
	} {
		if got := mine.ask(t, "POST", where+"/apps", form); !strings.Contains(got.Header().Get("Location"), "refused=") {
			t.Errorf("%s: %q", name, got.Header().Get("Location"))
		}
	}
	if len(runner.started) != 0 {
		t.Errorf("something was started: %v", runner.started)
	}
	if got := theirs.ask(t, "POST", where+"/apps", url.Values{"command": {"node a.js"}, "port": {"8080"}}); got.Code != http.StatusNotFound || len(runner.started) != 0 {
		t.Errorf("somebody else shared an app from my agent: %d", got.Code)
	}
	for i := 0; i < appsPerAgent; i++ {
		mine.ask(t, "POST", where+"/apps", url.Values{"command": {"node a.js"}, "port": {"8080"}})
	}
	site.server.Wait()
	if got := mine.ask(t, "POST", where+"/apps", url.Values{"command": {"node a.js"}, "port": {"8080"}}); !strings.Contains(got.Header().Get("Location"), "already+shares") {
		t.Errorf("an agent shares as many apps as it may: %q", got.Header().Get("Location"))
	}
}

func TestStoppingAnAppStopsItsProcessesAndForgetsItAndOnlyItsAgentsOwnerMay(t *testing.T) {
	site, runner, _ := sharingSite(t)
	mine, theirs := site.as(t, alice), site.as(t, bob)
	where, _ := started(t, mine)
	id := sharedApp(t, mine, where)

	if got := theirs.ask(t, "POST", where+"/apps/"+id+"/remove", nil); got.Code != http.StatusNotFound || len(runner.stopped) != 0 {
		t.Errorf("somebody else stopped my app: %d", got.Code)
	}
	if got := mine.ask(t, "POST", where+"/apps/"+id+"/remove", nil); got.Code != http.StatusSeeOther || len(runner.stopped) != 1 {
		t.Fatalf("%d, stopped %d", got.Code, len(runner.stopped))
	}
	if _, err := site.server.apps.Get(id); err == nil {
		t.Error("the app is still there")
	}
	if got := mine.ask(t, "GET", "/apps/"+id+"/", nil); got.Code != http.StatusNotFound {
		t.Errorf("%d", got.Code)
	}
}

func TestRemovingAnAgentStopsTheAppsItShared(t *testing.T) {
	site, runner, _ := sharingSite(t)
	where := startedInstance(t, site)
	sharedApp(t, site, where)
	if got := site.ask(t, "POST", where+"/remove", nil); got.Code != http.StatusSeeOther {
		t.Fatal(got.Code)
	}
	if len(runner.stopped) != 1 {
		t.Errorf("its app kept running: %d stopped", len(runner.stopped))
	}
}

func TestAServerWhoseAgentsAreNotInComputersOffersNoSharing(t *testing.T) {
	site := open(t)
	site.signIn(t)
	where := startedInstance(t, site)
	if page := site.ask(t, "GET", where, nil).Body.String(); strings.Contains(page, "Share an app") {
		t.Error("it offers what it cannot do")
	}
	if got := site.ask(t, "POST", where+"/apps", url.Values{"command": {"node a.js"}, "port": {"8080"}}); got.Code != http.StatusNotFound {
		t.Errorf("%d", got.Code)
	}
	if got := site.ask(t, "GET", "/apps/0123456789/", nil); got.Code != http.StatusNotFound {
		t.Errorf("%d", got.Code)
	}
}

// A person's other agent is not the app's agent, so it cannot be used to stop the app, and it is
// not a way to find out whether an app is there.
func TestAnAppIsRemovedOnlyThroughTheAgentItBelongsTo(t *testing.T) {
	site, runner, _ := sharingSite(t)
	mine := site.as(t, alice)
	first, _ := started(t, mine)
	second, _ := started(t, mine)
	if first == second {
		t.Fatal("two agents were expected")
	}
	id := sharedApp(t, mine, first)
	if got := mine.ask(t, "POST", second+"/apps/"+id+"/remove", nil); got.Code != http.StatusNotFound || len(runner.stopped) != 0 {
		t.Errorf("an app was removed through another agent: %d, stopped %d", got.Code, len(runner.stopped))
	}
	if _, err := site.server.apps.Get(id); err != nil {
		t.Error("the app is gone")
	}
}
