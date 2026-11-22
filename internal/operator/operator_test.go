package operator

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/cloud"
)

func app(t *testing.T) (*App, *cloud.Store) {
	t.Helper()
	store, err := cloud.Open(t.TempDir(), cloud.Settings{Demo: cloud.DemoSettings{Length: 30 * time.Minute}, PerDomain: 3})
	if err != nil {
		t.Fatal(err)
	}
	return New(store), store
}

// shown is the operator's page as the server wrote it into the HTML for the web app to draw.
func shown(t *testing.T, a *App) page {
	t.Helper()
	body := ask(a, "GET", "/", "localhost:8090", nil, nil).Body.String()
	start := strings.Index(body, `<script id="gusto-data" type="application/json">`)
	if start < 0 {
		t.Fatalf("the page carries no data:\n%s", body)
	}
	data := body[start+len(`<script id="gusto-data" type="application/json">`):]
	var view page
	if err := json.Unmarshal([]byte(data[:strings.Index(data, "</script>")]), &view); err != nil {
		t.Fatal(err)
	}
	return view
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

func TestTheOperatorsPageIsServedOnlyToRequestsToTheLoopback(t *testing.T) {
	a, _ := app(t)
	if got := ask(a, "GET", "/", "127.0.0.1:8090", nil, nil); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "<title>gusto operator</title>") {
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

func TestTheOperatorSeesWhoAskedForADemo(t *testing.T) {
	a, store := app(t)
	if _, _, err := store.StartDemo(t.Context(), "Ada Lovelace", "ada@acme.com", "Acme Research"); err != nil {
		t.Fatal(err)
	}
	view := shown(t, a)
	if len(view.Leads) != 1 || view.Leads[0].Name != "Ada Lovelace" || view.Leads[0].Email != "ada@acme.com" || view.Leads[0].Company != "Acme Research" || view.Leads[0].Outcome != "starting" {
		t.Errorf("the leads: %+v", view.Leads)
	}
}

func TestDemosArePausedAndResumedFromThePageAndOnlyFromThisSite(t *testing.T) {
	a, store := app(t)
	if got := ask(a, "POST", "/demos/pause", "localhost:8090", map[string]string{"Sec-Fetch-Site": "cross-site"}, url.Values{"paused": {"on"}}); got.Code != http.StatusForbidden || store.DemosPaused(t.Context()) {
		t.Fatalf("another site paused demos: %d", got.Code)
	}
	if got := ask(a, "POST", "/demos/pause", "localhost:8090", map[string]string{"Sec-Fetch-Site": "same-origin"}, url.Values{"paused": {"on"}}); got.Code != http.StatusSeeOther || !store.DemosPaused(t.Context()) {
		t.Fatalf("demos were not paused: %d", got.Code)
	}
	if view := shown(t, a); !view.Paused {
		t.Errorf("the page does not say demos are paused: %+v", view)
	}
	ask(a, "POST", "/demos/pause", "localhost:8090", nil, url.Values{"paused": {"off"}})
	if store.DemosPaused(t.Context()) {
		t.Error("demos were not resumed")
	}
}

func TestTheLeadsCsvWritesWhatAVisitorTypedAsTextAndNotAsAFormula(t *testing.T) {
	a, store := app(t)
	if _, _, err := store.StartDemo(t.Context(), "=HYPERLINK(\"http://evil.example\")", "mallory@acme.com", "+cmd Co"); err != nil {
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

func TestTheOperatorChangesTheDemoLimitsFromThePageAndTheyHoldAtOnce(t *testing.T) {
	a, store := app(t)
	if got := store.DemoLimits(t.Context()); got.PerPerson != 0 || got.MaxOrgs != 1 {
		t.Fatalf("the starting limits: %+v", got)
	}
	form := url.Values{"perDay": {"25"}, "perPerson": {"1"}, "perCompany": {"4"}, "maxOrgs": {"3"}}
	if got := ask(a, "POST", "/demos/limits", "localhost:8090", map[string]string{"Sec-Fetch-Site": "cross-site"}, form); got.Code != http.StatusForbidden || store.DemoLimitsSet(t.Context()) {
		t.Fatalf("another site set the limits: %d", got.Code)
	}
	if got := ask(a, "POST", "/demos/limits", "localhost:8090", map[string]string{"Sec-Fetch-Site": "same-origin"}, form); got.Code != http.StatusSeeOther {
		t.Fatalf("%d", got.Code)
	}
	if got := store.DemoLimits(t.Context()); got != (cloud.DemoLimits{PerDay: 25, PerPerson: 1, PerCompany: 4, MaxOrgs: 3}) {
		t.Errorf("%+v", got)
	}
	if view := shown(t, a); !view.LimitsSet || view.Limits != (cloud.DemoLimits{PerDay: 25, PerPerson: 1, PerCompany: 4, MaxOrgs: 3}) {
		t.Errorf("the page does not show the limits set: %+v", view)
	}
	// It holds on the very next demo, with no restart.
	if _, _, err := store.StartDemo(t.Context(), "Ada", "ada@acme.com", "Acme"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.StartDemo(t.Context(), "Ada", "ada+2@acme.com", "Acme"); err == nil || !strings.Contains(err.Error(), "demos for today") {
		t.Errorf("a person limit of 1 did not hold: %v", err)
	}
	for name, bad := range map[string]url.Values{
		"a negative":       {"perDay": {"-1"}, "perPerson": {"1"}, "perCompany": {"4"}, "maxOrgs": {"3"}},
		"not a number":     {"perDay": {"x"}, "perPerson": {"1"}, "perCompany": {"4"}, "maxOrgs": {"3"}},
		"none at once":     {"perDay": {"1"}, "perPerson": {"1"}, "perCompany": {"4"}, "maxOrgs": {"0"}},
		"too many at once": {"perDay": {"1"}, "perPerson": {"1"}, "perCompany": {"4"}, "maxOrgs": {"51"}},
	} {
		ask(a, "POST", "/demos/limits", "localhost:8090", nil, bad)
		if got := store.DemoLimits(t.Context()); got.PerDay != 25 {
			t.Errorf("%s was kept: %+v", name, got)
		}
	}
	ask(a, "POST", "/demos/limits", "localhost:8090", nil, url.Values{"reset": {"1"}})
	if store.DemoLimitsSet(t.Context()) || store.DemoLimits(t.Context()).PerPerson != 0 {
		t.Errorf("not back to the starting limits: %+v", store.DemoLimits(t.Context()))
	}
}
