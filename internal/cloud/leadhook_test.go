package cloud

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// sheet is what Apps Script does: a request to its address is answered with a redirect, and the redirect's address
// answers with what the script said, so a client that follows the redirect with a GET reads the answer.
type sheet struct {
	mu     sync.Mutex
	got    []map[string]any
	secret string
	down   bool
	server *httptest.Server
}

func newSheet(t *testing.T, secret string) *sheet {
	t.Helper()
	s := &sheet{secret: secret}
	var answer string
	mux := http.NewServeMux()
	mux.HandleFunc("POST /exec", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.down {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		text, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(text, &body)
		answer = `{"ok":false}`
		if body["secret"] == s.secret {
			s.got = append(s.got, body)
			answer = `{"ok":true}`
		}
		http.Redirect(w, r, "/echo", http.StatusFound)
	})
	mux.HandleFunc("GET /echo", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		io.WriteString(w, answer)
	})
	s.server = httptest.NewTLSServer(mux)
	t.Cleanup(s.server.Close)
	return s
}

func (s *sheet) hook(t *testing.T, secret string) *LeadHook {
	return &LeadHook{URL: s.server.URL + "/exec", Secret: secret, Dir: filepath.Join(t.TempDir(), "leadhook"), Client: s.server.Client()}
}

func (s *sheet) events() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, body := range s.got {
		out = append(out, body["event"].(string)+" "+body["lead"].(map[string]any)["slug"].(string))
	}
	return out
}

func TestALeadIsSentWithTheSecretInItsBodyAndFollowsTheRedirectAScriptAnswersWith(t *testing.T) {
	s := newSheet(t, "s3cret")
	hook := s.hook(t, "s3cret")
	if err := hook.Queue("started", Lead{Slug: "demo-acme-1", Name: "Ada", Email: "ada@acme.com", Company: "Acme"}); err != nil {
		t.Fatal(err)
	}
	if err := hook.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := s.events(); len(got) != 1 || got[0] != "started demo-acme-1" {
		t.Fatalf("%v", got)
	}
	if left, _ := os.ReadDir(hook.Dir); len(left) != 0 {
		t.Errorf("a lead that was taken is still waiting: %d", len(left))
	}
}

func TestALeadTheAddressRefusesStaysWaitingAndAnOutageDoesNotReorderThem(t *testing.T) {
	s := newSheet(t, "s3cret")
	wrong := s.hook(t, "not-the-secret")
	wrong.Queue("started", Lead{Slug: "demo-a"})
	if err := wrong.Flush(context.Background()); err == nil {
		t.Error("a refused secret was taken as sent")
	}
	if left, _ := os.ReadDir(wrong.Dir); len(left) != 1 {
		t.Errorf("a refused lead was dropped: %d", len(left))
	}

	hook := s.hook(t, "s3cret")
	s.mu.Lock()
	s.down = true
	s.mu.Unlock()
	hook.Queue("started", Lead{Slug: "demo-a"})
	hook.Queue("started", Lead{Slug: "demo-b"})
	hook.Queue("ended", Lead{Slug: "demo-a"})
	if err := hook.Flush(context.Background()); err == nil {
		t.Fatal("an outage was not an error")
	}
	if len(s.events()) != 0 {
		t.Fatalf("something was sent while it was down: %v", s.events())
	}
	s.mu.Lock()
	s.down = false
	s.mu.Unlock()
	if err := hook.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"started demo-a", "started demo-b", "ended demo-a"}
	if got := s.events(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("%v, want %v", got, want)
	}
}

func TestTheLeadHookNeedsBothItsAddressAndItsSecretAndAnHttpsAddress(t *testing.T) {
	for name, hook := range map[string]*LeadHook{
		"no secret":  {URL: "https://script.google.com/macros/s/x/exec"},
		"no address": {Secret: "s"},
		"plain http": {URL: "http://script.example/exec", Secret: "s"},
	} {
		if hook.Complete() == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if (&LeadHook{}).Complete() != nil || (*LeadHook)(nil).Complete() != nil || (&LeadHook{}).Enabled() {
		t.Error("a front door with no lead hook is not an error")
	}
	if (&LeadHook{URL: "https://script.google.com/macros/s/x/exec", Secret: "s"}).Complete() != nil {
		t.Error("a whole hook was refused")
	}
}

func TestADemoStartedAndEndedIsToldToTheSheetInThatOrder(t *testing.T) {
	s := newSheet(t, "s3cret")
	f := newFront(t, demoSet, func(c *Config) {
		c.Controller = controlled(t, c.Store)
		c.ComputersWait = 50 * time.Millisecond
		c.Leads = s.hook(t, "s3cret")
	})
	b := startDemoAs(t, f, "Ada", "ada@acme.com", "Acme")
	f.server.Wait()
	b.get("/demo/wait")
	slug := demoSlugOf(t, f)
	f.store.now = func() time.Time { return time.Now().Add(time.Hour) }
	f.server.EndDueDemos()
	f.server.ender.running.Wait()
	waitFor(t, func() bool { return len(s.events()) == 2 })
	if got := s.events(); got[0] != "started "+slug || got[1] != "ended "+slug {
		t.Errorf("%v", got)
	}
	last := s.got[1]["lead"].(map[string]any)
	if last["reason"] != "its time was up" || last["email"] != "ada@acme.com" {
		t.Errorf("%v", last)
	}
}
