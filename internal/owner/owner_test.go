package owner

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/cell"
	"github.com/getlatentic/gusto/internal/cloud"
)

type hosts []cell.Instance

func (h hosts) Running() ([]cell.Instance, error) { return h, nil }

func app(t *testing.T) (*App, *cloud.Store) {
	t.Helper()
	store, err := cloud.Open(t.TempDir(), cloud.Settings{Demo: cloud.DemoSettings{Length: 30 * time.Minute}, PerDomain: 3})
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(store, hosts{{ID: "i-0123456789abcdef0", Launched: time.Now().Add(-5 * time.Minute)}})
	if err != nil {
		t.Fatal(err)
	}
	return a, store
}

func ask(a *App, method, path, host string, header map[string]string, form url.Values) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://"+host+path, strings.NewReader(form.Encode()))
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range header {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	a.Handler().ServeHTTP(w, r)
	return w
}

func TestTheOwnerPageIsServedOnlyToRequestsToTheLoopback(t *testing.T) {
	a, _ := app(t)
	if got := ask(a, "GET", "/", "127.0.0.1:8090", nil, nil); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "gusto operator") {
		t.Fatalf("%d", got.Code)
	}
	for name, c := range map[string]struct {
		host   string
		header map[string]string
	}{
		"another host name":  {"gusto.getlatentic.com", nil},
		"a rebinding name":   {"evil.example:8090", nil},
		"through a proxy":    {"127.0.0.1:8090", map[string]string{"X-Forwarded-For": "203.0.113.9"}},
		"a forwarded header": {"localhost:8090", map[string]string{"Forwarded": "for=203.0.113.9"}},
	} {
		if got := ask(a, "GET", "/", c.host, c.header, nil); got.Code != http.StatusForbidden || strings.Contains(got.Body.String(), "Leads") {
			t.Errorf("%s: %d", name, got.Code)
		}
	}
}

func TestTheOwnerSeesWhoAskedForADemoAndTheHostsThatAreUp(t *testing.T) {
	a, store := app(t)
	if _, _, err := store.StartDemo("Ada Lovelace", "ada@acme.com", "Acme Research"); err != nil {
		t.Fatal(err)
	}
	body := ask(a, "GET", "/", "localhost:8090", nil, nil).Body.String()
	for _, want := range []string{"Ada Lovelace", "ada@acme.com", "Acme Research", "i-0123456789abcdef0", "5 min ago", "1 so far"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not say %q:\n%s", want, body)
		}
	}
}

func TestDemosArePausedAndResumedFromThePageAndOnlyFromThisSite(t *testing.T) {
	a, store := app(t)
	if got := ask(a, "POST", "/demos/pause", "localhost:8090", map[string]string{"Sec-Fetch-Site": "cross-site"}, url.Values{"paused": {"on"}}); got.Code != http.StatusForbidden || store.DemosPaused() {
		t.Fatalf("another site paused demos: %d", got.Code)
	}
	if got := ask(a, "POST", "/demos/pause", "localhost:8090", map[string]string{"Sec-Fetch-Site": "same-origin"}, url.Values{"paused": {"on"}}); got.Code != http.StatusSeeOther || !store.DemosPaused() {
		t.Fatalf("demos were not paused: %d", got.Code)
	}
	if body := ask(a, "GET", "/", "localhost:8090", nil, nil).Body.String(); !strings.Contains(body, "New demos are paused") || !strings.Contains(body, "Accept new demos") {
		t.Errorf("the page does not say so:\n%s", body)
	}
	ask(a, "POST", "/demos/pause", "localhost:8090", nil, url.Values{"paused": {"off"}})
	if store.DemosPaused() {
		t.Error("demos were not resumed")
	}
}

func TestTheLeadsCsvWritesWhatAVisitorTypedAsTextAndNotAsAFormula(t *testing.T) {
	a, store := app(t)
	if _, _, err := store.StartDemo("=HYPERLINK(\"http://evil.example\")", "mallory@acme.com", "+cmd Co"); err != nil {
		t.Skip("the store refused the name: ", err)
	}
	got := ask(a, "GET", "/leads.csv", "localhost:8090", nil, nil)
	if !strings.HasPrefix(got.Header().Get("Content-Type"), "text/csv") || !strings.HasPrefix(got.Body.String(), "requested,name,email,company") {
		t.Fatalf("%v\n%s", got.Header(), got.Body.String())
	}
	if strings.Contains(got.Body.String(), ",=HYPERLINK") || strings.Contains(got.Body.String(), ",+cmd") {
		t.Errorf("a formula reaches the spreadsheet:\n%s", got.Body.String())
	}
}

func TestTheOwnerChangesTheDemoLimitsFromThePageAndTheyHoldAtOnce(t *testing.T) {
	a, store := app(t)
	if got := store.DemoLimits(); got.PerPerson != 0 || got.MaxOrgs != 1 {
		t.Fatalf("the starting limits: %+v", got)
	}
	form := url.Values{"perDay": {"25"}, "perPerson": {"1"}, "perCompany": {"4"}, "maxOrgs": {"3"}}
	if got := ask(a, "POST", "/demos/limits", "localhost:8090", map[string]string{"Sec-Fetch-Site": "cross-site"}, form); got.Code != http.StatusForbidden || store.DemoLimitsSet() {
		t.Fatalf("another site set the limits: %d", got.Code)
	}
	if got := ask(a, "POST", "/demos/limits", "localhost:8090", map[string]string{"Sec-Fetch-Site": "same-origin"}, form); got.Code != http.StatusSeeOther {
		t.Fatalf("%d", got.Code)
	}
	if got := store.DemoLimits(); got != (cloud.DemoLimits{PerDay: 25, PerPerson: 1, PerCompany: 4, MaxOrgs: 3}) {
		t.Errorf("%+v", got)
	}
	body := ask(a, "GET", "/", "localhost:8090", nil, nil).Body.String()
	for _, want := range []string{`name="perPerson" min="0" max="1000" value="1"`, `name="maxOrgs" min="1" max="50" value="3"`, "limits you set", "Go back to the starting limits"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page does not say %q", want)
		}
	}
	// It holds on the very next demo, with no restart.
	if _, _, err := store.StartDemo("Ada", "ada@acme.com", "Acme"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.StartDemo("Ada", "ada+2@acme.com", "Acme"); err == nil || !strings.Contains(err.Error(), "demos for today") {
		t.Errorf("a person limit of 1 did not hold: %v", err)
	}
	for name, bad := range map[string]url.Values{
		"a negative":       {"perDay": {"-1"}, "perPerson": {"1"}, "perCompany": {"4"}, "maxOrgs": {"3"}},
		"not a number":     {"perDay": {"x"}, "perPerson": {"1"}, "perCompany": {"4"}, "maxOrgs": {"3"}},
		"none at once":     {"perDay": {"1"}, "perPerson": {"1"}, "perCompany": {"4"}, "maxOrgs": {"0"}},
		"too many at once": {"perDay": {"1"}, "perPerson": {"1"}, "perCompany": {"4"}, "maxOrgs": {"51"}},
	} {
		ask(a, "POST", "/demos/limits", "localhost:8090", nil, bad)
		if got := store.DemoLimits(); got.PerDay != 25 {
			t.Errorf("%s was kept: %+v", name, got)
		}
	}
	ask(a, "POST", "/demos/limits", "localhost:8090", nil, url.Values{"reset": {"1"}})
	if store.DemoLimitsSet() || store.DemoLimits().PerPerson != 0 {
		t.Errorf("not back to the starting limits: %+v", store.DemoLimits())
	}
}
