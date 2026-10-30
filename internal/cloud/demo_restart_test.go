package cloud

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// A demo whose front door is restarted while it is starting carries on starting under the new one, and the visitor's wait
// page takes them in once it is ready (planning#516).
func TestADemoTheFrontDoorRestartedPartWayThroughStartsAndTheVisitorGoesIn(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := demoFront(t, "FAKE_DELAY=3s")
		visitor := startDemoAs(t, f, "Ada Lovelace", "ada@acme.com", "Acme Research")
		if _, waiting := visitor.get("/demo/wait"); !strings.Contains(waiting, "Getting your demo ready") {
			t.Fatalf("the demo is not starting:\n%s", waiting)
		}
		// The upgrade: the first front door's servers are stopped part-way through starting, and it is gone.
		f.server.Controller.stopAll()
		f.http.Close()
		f.server.Wait()

		sessions, err := f.store.OpenSessions(false)
		if err != nil {
			t.Fatal(err)
		}
		controller := controlled(t, f.store)
		base, _ := url.Parse("http://cloud.test")
		server, err := New(Config{Store: f.store, Mail: Outbox{Dir: f.outbox}, Sessions: sessions, URL: base, Domain: "cloud.test", Controller: controller})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		go controller.Run(ctx, time.Hour)
		after := &front{t: t, server: server, http: httptest.NewServer(server.Handler()), store: f.store, outbox: f.outbox}
		t.Cleanup(after.http.Close)
		t.Cleanup(server.Wait)

		again := after.browser()
		from, _ := url.Parse(f.http.URL)
		to, _ := url.Parse(after.http.URL)
		again.client.Jar.SetCookies(to, visitor.client.Jar.Cookies(from))
		waitFor(t, func() bool {
			status, _, header := again.do("GET", "/demo/wait", nil, nil)
			return status == http.StatusSeeOther && header.Get("Location") == "/open"
		})
		if demo, _, found := f.store.DemoLeft(sluggedDemo(t, f.store)); !found || demo.Ready.IsZero() {
			t.Error("the demo's time did not start once it was ready")
		}
	})
}

// sluggedDemo is the one demo organization there is.
func sluggedDemo(t *testing.T, s *Store) string {
	t.Helper()
	orgs, err := s.Orgs()
	if err != nil || len(orgs) != 1 {
		t.Fatalf("%d organizations: %v", len(orgs), err)
	}
	return orgs[0].Slug
}
