package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/apps"
)

func TestAFolderWithAPageInItCanBePreviewedAndNothingElseCan(t *testing.T) {
	got := previewsOf([]string{"2048-app/index.html", "2048-app/script.js", "index.html", "docs/guide/index.html", "a/b.html", "../x/index.html", "a b/index.html", "2048-app/index.html"})
	want := []string{"2048-app", ".", "docs/guide"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("%v, wanted %v", got, want)
	}
	for _, bad := range []string{"", "..", "../x", "/etc", "a/../b", "a b", "a;b", "$(x)", "-e", ".hidden/../x", "a'b", "a\nb"} {
		if previewCommand(bad) != "" {
			t.Errorf("%q can be previewed", bad)
		}
	}
	for _, good := range []string{".", "2048-app", "docs/guide", "a.b_c-d"} {
		command := previewCommand(good)
		if !strings.HasPrefix(command, "node -e '") || !strings.HasSuffix(command, "' "+good) || strings.Count(command, "'") != 2 {
			t.Errorf("%q: %s", good, command)
		}
	}
}

func TestThePreviewPortIsTheFirstOneTheAgentIsNotUsing(t *testing.T) {
	if got := previewPort(nil); got != 8080 {
		t.Errorf("%d", got)
	}
	taken := []apps.App{{Port: 8080, State: apps.Live}, {Port: 8081, State: apps.Starting}, {Port: 8082, State: apps.Failed}}
	if got := previewPort(taken); got != 8082 {
		t.Errorf("a port of an app that is gone is free again: %d", got)
	}
}

// sharingSite is a server whose agents can share apps, through a runner that only says it ran them.
type ranApps struct{ started []apps.App }

func (r *ranApps) Start(_ context.Context, app apps.App) (apps.App, error) {
	r.started = append(r.started, app)
	app.Host, app.Target = "a.example", "127.0.0.1:1"
	return app, nil
}
func (r *ranApps) Stop(context.Context, apps.App) error { return nil }

func TestAPageTheAgentBuiltIsPreviewedFromTheConversationItWasBuiltIn(t *testing.T) {
	site := open(t)
	store, err := apps.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner := &ranApps{}
	site.server.UseApps(store, runner)
	site.signIn(t)
	thread := startedThread(t, site)
	instance := thread[:strings.Index(thread, "/threads/")]

	submit := func(form url.Values) (int, string) {
		answer := site.ask(t, "POST", instance+"/apps", form)
		return answer.Code, answer.Header().Get("Location")
	}
	code, where := submit(url.Values{"preview": {"2048-app"}, "back": {thread}})
	site.server.publishing.Wait()
	if code != http.StatusSeeOther || where != thread+"#apps" {
		t.Fatalf("the person is sent back to the conversation: %d %q", code, where)
	}
	if len(runner.started) != 1 || runner.started[0].Port != 8080 || !strings.HasSuffix(runner.started[0].Command, "' 2048-app") {
		t.Fatalf("%+v", runner.started)
	}
	if page := site.ask(t, "GET", thread, nil).Body.String(); !strings.Contains(page, "Open the app") {
		t.Errorf("the conversation links to the app that is live:\n%s", page)
	}
	code, where = submit(url.Values{"preview": {"../secrets"}, "back": {thread}})
	if !strings.Contains(where, "refused=") || !strings.HasPrefix(where, thread) || len(runner.started) != 1 {
		t.Errorf("a folder outside the agent's own was started: %d %q", code, where)
	}
	for _, back := range []string{"//evil.example/x", "https://evil.example/", "/instances/other/threads/1", thread + "/../../x", thread + "?a=1"} {
		code, where := submit(url.Values{"preview": {"docs"}, "back": {back}})
		if code != http.StatusSeeOther || !strings.HasPrefix(where, instance) || strings.Contains(where, "/threads/") || strings.Contains(where, "evil") {
			t.Errorf("%q sent the person to %q", back, where)
		}
	}
	site.server.publishing.Wait()
	for i, app := range runner.started {
		if app.Port != 8080+i {
			t.Errorf("each preview has a port of its own: %+v", runner.started)
			break
		}
	}
}
