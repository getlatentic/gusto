package cloud

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// cloudflare stands in for Cloudflare's check: it accepts one answer and says what it was asked.
type cloudflare struct {
	*httptest.Server
	mu    sync.Mutex
	asked []url.Values
	down  bool
}

func newCloudflare(t *testing.T) *cloudflare {
	t.Helper()
	c := &cloudflare{}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		c.mu.Lock()
		c.asked = append(c.asked, r.PostForm)
		down := c.down
		c.mu.Unlock()
		if down {
			http.Error(w, "unavailable", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.PostForm.Get("secret") == "the-secret" && r.PostForm.Get("response") == "a-human" {
			w.Write([]byte(`{"success":true}`))
			return
		}
		w.Write([]byte(`{"success":false,"error-codes":["invalid-input-response"]}`))
	}))
	t.Cleanup(c.Close)
	return c
}

func (c *cloudflare) questions() int { c.mu.Lock(); defer c.mu.Unlock(); return len(c.asked) }

func checkedFront(t *testing.T, c *cloudflare) *front {
	t.Helper()
	return newFront(t, demoSet, func(cfg *Config) {
		cfg.Controller = controlled(t, cfg.Store)
		cfg.Turnstile = Turnstile{Site: "the-site-key", Secret: "the-secret", Verify: c.URL}
	})
}

func (f *front) tryDemo(answer string) (int, string) {
	form := url.Values{"name": {"Maria Okafor"}, "email": {"maria@futureacmebank.com"}, "company": {"Future Acme Bank"}}
	if answer != "" {
		form.Set("cf-turnstile-response", answer)
	}
	status, page, _ := f.browser().post("/demo", form)
	return status, page
}

func TestTheDemoFormCarriesTheBotCheckAndMayLoadItsScriptFromCloudflareAndNothingElse(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f := checkedFront(t, newCloudflare(t))
		status, page, header := f.browser().do("GET", "/demo", nil, nil)
		if status != http.StatusOK || !strings.Contains(page, `class="cf-turnstile" data-size="flexible" data-sitekey="the-site-key"`) || !strings.Contains(page, "https://challenges.cloudflare.com/turnstile/v0/api.js") {
			t.Errorf("the form has no widget:\n%s", page)
		}
		csp := header.Get("Content-Security-Policy")
		for _, want := range []string{"script-src 'self' https://challenges.cloudflare.com", "frame-src https://challenges.cloudflare.com", "frame-ancestors 'none'"} {
			if !strings.Contains(csp, want) {
				t.Errorf("%q is not in %s", want, csp)
			}
		}
		if strings.Contains(page, "the-secret") {
			t.Error("the secret is on the form")
		}
		_, plain, plainHeader := newFront(t, demoSet, nil).browser().do("GET", "/demo", nil, nil)
		if strings.Contains(plain, "turnstile") || strings.Contains(plainHeader.Get("Content-Security-Policy"), "cloudflare") {
			t.Error("a host with no bot check loads a script from Cloudflare")
		}
		f.server.Wait()
	})
}

func TestADemoIsStartedOnlyForAVisitorCloudflareAcceptsAndTheSecretAndAddressAreWhatItIsAsked(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		c := newCloudflare(t)
		f := checkedFront(t, c)
		if status, page := f.tryDemo("a-robot"); status != http.StatusForbidden || !strings.Contains(page, "could not tell that you are not a robot") || !strings.Contains(page, `value="maria@futureacmebank.com"`) {
			t.Errorf("%d\n%s", status, page)
		}
		if orgs, _ := f.store.Orgs(); len(orgs) != 0 {
			t.Error("a robot made an organization")
		}
		asked := c.questions()
		if status, _ := f.tryDemo(""); status != http.StatusForbidden || c.questions() != asked {
			t.Errorf("a form with no answer is turned away without asking Cloudflare: %d, %d questions", status, c.questions()-asked)
		}
		if status, _ := f.tryDemo(strings.Repeat("x", 3000)); status != http.StatusForbidden || c.questions() != asked {
			t.Errorf("an answer that cannot be one is not sent on: %d", status)
		}
		if status, _ := f.tryDemo("a-human"); status != http.StatusSeeOther {
			t.Fatalf("a visitor Cloudflare accepts was refused: %d", status)
		}
		f.server.Wait()
		last := c.asked[len(c.asked)-1]
		if last.Get("secret") != "the-secret" || last.Get("response") != "a-human" || last.Get("remoteip") == "" {
			t.Errorf("%v", last)
		}
		if orgs, _ := f.store.Orgs(); len(orgs) != 1 {
			t.Errorf("%d organizations", len(orgs))
		}
	})
}

func TestWhenTheCheckCannotBeMadeNobodyIsLetThroughAndTheyAreToldToTryAgain(t *testing.T) {
	c := newCloudflare(t)
	c.down = true
	f := checkedFront(t, c)
	status, page := f.tryDemo("a-human")
	if status != http.StatusServiceUnavailable || !strings.Contains(page, "could not be made") {
		t.Errorf("%d\n%s", status, page)
	}
	if orgs, _ := f.store.Orgs(); len(orgs) != 0 {
		t.Error("a visitor who could not be checked was let in")
	}
	dead := checkedFront(t, newCloudflare(t))
	dead.server.Turnstile.Verify = "http://127.0.0.1:1/"
	if status, _ := dead.tryDemo("a-human"); status != http.StatusServiceUnavailable {
		t.Errorf("an unreachable check let somebody through: %d", status)
	}
}

func TestTheLimitsAreCheckedBeforeCloudflareIsAskedAnything(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		c := newCloudflare(t)
		f := checkedFront(t, c)
		f.server.signups = NewLimiter(1, 3600e9)
		f.tryDemo("a-robot")
		asked := c.questions()
		if status, _ := f.tryDemo("a-robot"); status != http.StatusTooManyRequests || c.questions() != asked {
			t.Errorf("a visitor over the limit costs a question to Cloudflare: %d", status)
		}
	})
}

func TestABotCheckThatIsHalfSetUpStopsTheFrontDoorStartingAndNoneIsFine(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		for name, tc := range map[string]struct {
			check Turnstile
			ok    bool
		}{
			"none":          {Turnstile{}, true},
			"both":          {Turnstile{Site: "s", Secret: "x"}, true},
			"a site only":   {Turnstile{Site: "s"}, false},
			"a secret only": {Turnstile{Secret: "x"}, false},
		} {
			store := open(t, Settings{})
			sessions, _ := store.OpenSessions(false)
			_, err := New(Config{Store: store, Sessions: sessions, Turnstile: tc.check})
			if (err == nil) != tc.ok {
				t.Errorf("%s: %v", name, err)
			}
		}
	})
}
