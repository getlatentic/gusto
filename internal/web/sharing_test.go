package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/apps"
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
		// A gateway answers a request that names an origin other than its own with a refusal.
		if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
			http.Error(w, "Cross-origin service request rejected", http.StatusForbidden)
			return
		}
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

func TestAnAppAnAgentBuiltIsSharedOnceItAnswersAndOnlyTheOrganizationsMembersOpenIt(t *testing.T) {
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

func TestAnotherOrganizationsServerHasNoSuchApp(t *testing.T) {
	first, _, _ := sharingSite(t)
	id := sharedApp(t, first, startedInstance(t, first))
	second, _, _ := sharingSite(t)
	if got := second.ask(t, "GET", "/apps/"+id+"/", nil); got.Code != http.StatusNotFound {
		t.Errorf("another organization opened it: %d", got.Code)
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

// behindAFrontDoorSharing is a server that is acme-com's in a hosted gusto, and shares apps.
func behindAFrontDoorSharing(t *testing.T) (*site, *runnerFake, *gatewayFake) {
	t.Helper()
	site, runner, gateway := sharingSite(t)
	site.server.UseFrontDoor(FrontDoor{Key: entryKey, Org: "acme-com", URL: "https://cloud.test"})
	site.handler = site.server.Handler()
	return site, runner, gateway
}

// appRequest is a request as the front door passes one for an app's own host name.
func (s *site) appRequest(t *testing.T, id, method, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	request.Header.Set(AppHeader, SignApp(entryKey, id))
	request.Header.Set("X-Forwarded-Proto", "https")
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	recorder := httptest.NewRecorder()
	s.handler.ServeHTTP(recorder, request)
	return recorder
}

// signedInTo is a person the front door signed in on the app's host name, with the cookie they were given.
func (s *site) signedInTo(t *testing.T, id string) *http.Cookie {
	t.Helper()
	token := tokenFor(t, "acme-com", "ada@acme.com", "runner", time.Now())
	answer := s.appRequest(t, id, "GET", "/auth/entry?t="+url.QueryEscape(token))
	if answer.Code != http.StatusSeeOther || answer.Header().Get("Location") != "/" {
		t.Fatalf("entering: %d %v", answer.Code, answer.Header())
	}
	for _, cookie := range answer.Result().Cookies() {
		if strings.HasSuffix(cookie.Name, "gusto") {
			return cookie
		}
	}
	t.Fatal("entering gave no cookie")
	return nil
}

// An app the agent wrote with absolute paths, which is what an agent writes, works: it has its own
// host name, so /click is the app's and not the organization's.
func TestAnAppHasAHostNameOfItsOwnSoWhatItWroteWorksAsItWroteIt(t *testing.T) {
	site, _, gateway := behindAFrontDoorSharing(t)
	where := startedInstance(t, site)
	id := sharedApp(t, site, where)
	app, _ := site.server.apps.Get(id)

	if page := site.ask(t, "GET", where, nil).Body.String(); !strings.Contains(page, `href="https://acme-com--`+id+`.cloud.test/"`) {
		t.Errorf("the agent's page links the app at its own host name:\n%s", page)
	}
	if got := site.ask(t, "GET", "/apps/"+id+"/x", nil); got.Header().Get("Location") != "https://acme-com--"+id+".cloud.test/" {
		t.Errorf("a path link somebody kept goes to the app's host: %d %q", got.Code, got.Header().Get("Location"))
	}

	cookie := site.signedInTo(t, id)
	got := site.appRequest(t, id, "POST", "/click", cookie)
	if got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "/click") {
		t.Fatalf("the app's own /click: %d %s", got.Code, got.Body)
	}
	if asked := gateway.last(); asked.URL.Path != "/click" || asked.Host != app.Host {
		t.Errorf("the app was asked for %q as %q", asked.URL.Path, asked.Host)
	}
	if policy := got.Header().Values("Content-Security-Policy"); len(policy) != 0 {
		t.Errorf("an app with an origin of its own is not fenced by this server's policy: %v", policy)
	}
}

func TestAnAppsHostNameSignsInWithTheFrontDoorsTokenAndSendsAStrangerThere(t *testing.T) {
	site, _, _ := behindAFrontDoorSharing(t)
	id := sharedApp(t, site, startedInstance(t, site))

	stranger := site.appRequest(t, id, "GET", "/")
	if stranger.Code != http.StatusSeeOther || stranger.Header().Get("Location") != "https://cloud.test/open?app="+id {
		t.Errorf("a stranger is sent to be signed in where people are: %d %q", stranger.Code, stranger.Header().Get("Location"))
	}
	cookie := site.signedInTo(t, id)
	if cookie.Name != "__Host-gusto" || !cookie.Secure || cookie.Path != "/" || cookie.Domain != "" {
		t.Errorf("the session over TLS is one a sibling host name cannot set: %+v", cookie)
	}
	if got := site.appRequest(t, id, "GET", "/", cookie); got.Code != http.StatusOK {
		t.Errorf("a person signed in opens the app: %d", got.Code)
	}
	// A token is used once, and one for another organization is not one.
	token := tokenFor(t, "acme-com", "ada@acme.com", "runner", time.Now())
	site.appRequest(t, id, "GET", "/auth/entry?t="+url.QueryEscape(token))
	if again := site.appRequest(t, id, "GET", "/auth/entry?t="+url.QueryEscape(token)); again.Code != http.StatusForbidden {
		t.Errorf("a token was used twice: %d", again.Code)
	}
	other := tokenFor(t, "globex-com", "eve@globex.com", "runner", time.Now())
	if got := site.appRequest(t, id, "GET", "/auth/entry?t="+url.QueryEscape(other)); got.Code != http.StatusForbidden {
		t.Errorf("another organization's token signed somebody in: %d", got.Code)
	}
}

func TestOnlyTheFrontDoorCanNameAnApp(t *testing.T) {
	site, _, _ := behindAFrontDoorSharing(t)
	id := sharedApp(t, site, startedInstance(t, site))
	for name, header := range map[string]string{
		"an unsigned id":         id,
		"another id's signature": id + "." + strings.Split(SignApp(entryKey, "0000000000"), ".")[1],
		"a signature of no key":  id + ".00",
		"a wrong key":            SignApp([]byte("fedcba9876543210fedcba9876543210"), id),
	} {
		request := httptest.NewRequest("GET", "/", nil)
		request.Header.Set(AppHeader, header)
		recorder := httptest.NewRecorder()
		site.handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s: %d", name, recorder.Code)
		}
	}
	// A server that is not behind a front door has no key to check it by, and believes nobody.
	alone, _, _ := sharingSite(t)
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set(AppHeader, SignApp(entryKey, id))
	recorder := httptest.NewRecorder()
	alone.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("a server with no front door took an app's header: %d", recorder.Code)
	}
}

// On an app's host name nothing of the organization's own pages can be reached, whatever is asked for.
func TestOnAnAppsHostNameEveryPathIsTheAppsAndNoneIsAPageOfTheOrganization(t *testing.T) {
	site, _, gateway := behindAFrontDoorSharing(t)
	id := sharedApp(t, site, startedInstance(t, site))
	cookie := site.signedInTo(t, id)
	for _, path := range []string{"/spend", "/instances/anything", "/jobs", "/tokens", "/logout", "/apps/" + id + "/", "/static/app.css"} {
		got := site.appRequest(t, id, "GET", path, cookie)
		if !strings.Contains(got.Body.String(), "<h1>the app</h1> "+path) {
			t.Errorf("%s was not the app's: %d %.100s", path, got.Code, got.Body)
		}
		if gateway.last().URL.Path != path {
			t.Errorf("%s: the app was asked for %q", path, gateway.last().URL.Path)
		}
	}
	if got := site.appRequest(t, id, "POST", "/logout", cookie); strings.Contains(strings.Join(got.Header().Values("Set-Cookie"), ";"), "gusto=;") {
		t.Errorf("the app's /logout signed the person out of their session: %v", got.Header())
	}
}

func TestAnAppSetsItsOwnCookiesOnItsHostOnlyAndNeverOneOfTheOrganizationsOrForTheWholeDomain(t *testing.T) {
	site, _, gateway := behindAFrontDoorSharing(t)
	id := sharedApp(t, site, startedInstance(t, site))
	cookie := site.signedInTo(t, id)
	gateway.Server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gateway.mu.Lock()
		gateway.seen = append(gateway.seen, r.Clone(context.Background()))
		gateway.mu.Unlock()
		w.Header().Add("Set-Cookie", "theme=dark; Path=/; Domain=.cloud.test")
		w.Header().Add("Set-Cookie", "__Host-gusto=evil; Path=/; Secure")
		w.Header().Add("Set-Cookie", "gusto=evil; Path=/")
		w.Header().Add("Set-Cookie", "__Secure-x=1; Path=/; Secure")
		w.Write([]byte("ok"))
	})
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set(AppHeader, SignApp(entryKey, id))
	request.Header.Set("X-Forwarded-Proto", "https")
	request.AddCookie(cookie)
	request.AddCookie(&http.Cookie{Name: "theme", Value: "light"})
	recorder := httptest.NewRecorder()
	site.handler.ServeHTTP(recorder, request)

	set := recorder.Header().Values("Set-Cookie")
	if len(set) != 1 || !strings.HasPrefix(set[0], "theme=dark") || strings.Contains(strings.ToLower(set[0]), "domain") {
		t.Errorf("only the app's own cookie, and for its host alone: %v", set)
	}
	asked := gateway.last()
	if _, err := asked.Cookie("theme"); err != nil {
		t.Error("the app's own cookie was not sent back to it")
	}
	if asked.Header.Get("Cookie") == "" || strings.Contains(asked.Header.Get("Cookie"), "gusto") {
		t.Errorf("the app was sent the person's session: %q", asked.Header.Get("Cookie"))
	}
}

// A browser's POST names the origin it drew the page at, and the gateway refuses one that is not its own.
// That is what stopped the first app an agent built from counting a click.
func TestABrowsersPostIsPassedOnAsFromTheGatewaysOwnOriginInBothWaysAnAppIsServed(t *testing.T) {
	site, _, gateway := behindAFrontDoorSharing(t)
	id := sharedApp(t, site, startedInstance(t, site))
	app, _ := site.server.apps.Get(id)
	cookie := site.signedInTo(t, id)

	post := httptest.NewRequest("POST", "/click", nil)
	post.Header.Set(AppHeader, SignApp(entryKey, id))
	post.Header.Set("X-Forwarded-Proto", "https")
	post.Header.Set("Origin", "https://acme-com--"+id+".cloud.test")
	post.Header.Set("Referer", "https://acme-com--"+id+".cloud.test/")
	post.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	site.handler.ServeHTTP(recorder, post)
	if recorder.Code != http.StatusOK {
		t.Fatalf("the app's own page could not post to it: %d %s", recorder.Code, recorder.Body)
	}
	if asked := gateway.last(); asked.Header.Get("Origin") != "http://"+app.Host || asked.Header.Get("Referer") != "" {
		t.Errorf("the gateway was asked as %q, referred by %q", asked.Header.Get("Origin"), asked.Header.Get("Referer"))
	}

	// The same, on a server with no front door, where the app is a path of the organization's own origin.
	alone, _, aloneGateway := sharingSite(t)
	aloneID := sharedApp(t, alone, startedInstance(t, alone))
	request := httptest.NewRequest("POST", "/apps/"+aloneID+"/click", nil)
	request.Header.Set("Origin", "https://acme.test")
	request.AddCookie(alone.cookie)
	recorder = httptest.NewRecorder()
	alone.handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || aloneGateway.last().Header.Get("Origin") == "https://acme.test" {
		t.Errorf("a path-served app's post: %d, asked as %q", recorder.Code, aloneGateway.last().Header.Get("Origin"))
	}
}

// quietGateway answers every request except the first for a path, which it holds without a word until the test is over,
// as a gateway was seen to do for the first request to the first app on a new cell.
func quietGateway(t *testing.T) (*gatewayFake, *int32) {
	t.Helper()
	var tried sync.Map
	var held int32
	release := make(chan struct{})
	g := &gatewayFake{}
	g.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, again := tried.LoadOrStore(r.Method+" "+r.URL.Path, true); !again || r.URL.Path == "/never" {
			atomic.AddInt32(&held, 1)
			<-release
			return
		}
		w.Write([]byte("<h1>the app</h1>"))
	}))
	// Cleanups run last in first out: the held requests are let go before the server waits for them.
	t.Cleanup(g.Close)
	t.Cleanup(func() { close(release) })
	return g, &held
}

func TestARequestToAYoungAppThatIsNotAnsweredIsAskedAgainOnANewConnectionAndALaterOneIsNot(t *testing.T) {
	site, runner, _ := behindAFrontDoorSharing(t)
	gateway, held := quietGateway(t)
	runner.gateway = gateway
	site.server.appFirstByte = 200 * time.Millisecond
	id := sharedApp(t, site, startedInstance(t, site))
	cookie := site.signedInTo(t, id)

	began := time.Now()
	answer := site.appRequest(t, id, "GET", "/", cookie)
	if answer.Code != http.StatusOK || !strings.Contains(answer.Body.String(), "the app") {
		t.Fatalf("the first request was not answered: %d %q", answer.Code, answer.Body)
	}
	if took := time.Since(began); took > 3*time.Second {
		t.Errorf("a request that was held took %s to be answered", took)
	}
	if atomic.LoadInt32(held) != 1 {
		t.Errorf("one request was held and asked again: %d", atomic.LoadInt32(held))
	}

	// A request that changes something is not asked again: it may have been done.
	before := atomic.LoadInt32(held)
	posted := httptest.NewRequest("POST", "/save", strings.NewReader("x=1"))
	posted.Header.Set(AppHeader, SignApp(entryKey, id))
	posted.Header.Set("X-Forwarded-Proto", "https")
	posted.AddCookie(cookie)
	recorder := httptest.NewRecorder()
	site.handler.ServeHTTP(recorder, posted)
	if recorder.Code != http.StatusBadGateway || atomic.LoadInt32(held)-before != 1 {
		t.Errorf("a post was %d and sent %d times", recorder.Code, atomic.LoadInt32(held)-before)
	}

	// A page that is never answered is given up on, not waited for for ever.
	began = time.Now()
	gave := site.appRequest(t, id, "GET", "/never", cookie)
	if gave.Code != http.StatusBadGateway || time.Since(began) > 5*time.Second {
		t.Errorf("%d after %s", gave.Code, time.Since(began))
	}
}
