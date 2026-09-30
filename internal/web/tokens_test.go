package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/apitoken"
	"github.com/getlatentic/haas/internal/changelog"
	"github.com/getlatentic/haas/internal/jobs"
)

// tokened is a site that takes API tokens, with the change log on, and the
// tokens it made: one for each thing a token may do, and one that may do all.
type tokened struct {
	*site
	log     *changelog.Log
	file    string
	secrets map[string]string
}

func withTokens(t *testing.T) *tokened {
	t.Helper()
	here, log := recording(t)
	file := filepath.Join(t.TempDir(), "tokens.json")
	store, err := apitoken.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	here.server.UseTokens(store)
	made := &tokened{site: here, log: log, file: file, secrets: map[string]string{}}
	for name, scopes := range map[string][]string{
		"ci":       {apitoken.Submit},
		"finance":  {apitoken.Spend},
		"auditor":  {apitoken.Audit},
		"everyone": apitoken.Scopes,
	} {
		_, secret, err := store.Create(name, scopes, "the test")
		if err != nil {
			t.Fatal(err)
		}
		made.secrets[name] = secret
	}
	return made
}

func (s *site) call(method, path, bearer string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, nil)
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	recorder := httptest.NewRecorder()
	s.handler.ServeHTTP(recorder, request)
	return recorder
}

func TestOnceTokensAreOnTheSharedPasswordOpensNoEndpoint(t *testing.T) {
	here := withTokens(t)
	for _, target := range [][2]string{{"GET", "/api/spend"}, {"GET", "/api/changes"}} {
		if got := here.call(target[0], target[1], password); got.Code != http.StatusUnauthorized {
			t.Errorf("the shared password gave %d at %s", got.Code, target[1])
		}
	}
	code, answer := here.send(t, password, Submission{Spec: aJob()})
	if code != http.StatusUnauthorized || answer.ID != "" {
		t.Fatalf("the shared password sent a job: %d %+v", code, answer)
	}
	here.nothing(t)
}

func TestATokenDoesWhatItWasGivenAndNothingElse(t *testing.T) {
	here := withTokens(t)
	allowed := map[string]string{"ci": "submit", "finance": "spend", "auditor": "audit"}
	endpoint := map[string][2]string{
		"submit": {"POST", "/api/jobs"}, "spend": {"GET", "/api/spend"}, "audit": {"GET", "/api/changes"},
	}
	for name, may := range allowed {
		for scope, target := range endpoint {
			var code int
			if scope == "submit" {
				code, _ = here.send(t, here.secrets[name], Submission{Spec: aJob()})
			} else {
				code = here.call(target[0], target[1], here.secrets[name]).Code
			}
			if scope == may && code >= 300 {
				t.Errorf("%s may %s and got %d", name, scope, code)
			}
			if scope != may && code != http.StatusForbidden {
				t.Errorf("%s may not %s and got %d", name, scope, code)
			}
		}
	}
}

func TestAJobSentWithATokenIsAskedForByThatToken(t *testing.T) {
	here := withTokens(t)
	_, answer := here.send(t, here.secrets["ci"], Submission{Spec: aJob()})
	record, found := here.index.Get(answer.ID)
	if !found || record.Asked != "token:ci" {
		t.Fatalf("the job was asked for by %q", record.Asked)
	}
	here.signIn(t)
	page := here.ask(t, "GET", "/jobs/"+answer.ID, nil).Body.String()
	if !strings.Contains(page, "sent to POST /api/jobs") {
		t.Fatalf("the record does not say the job came over the API:\n%s", page)
	}
	stranger := here.as(t, "amy@acme.test")
	if got := stranger.ask(t, "GET", "/jobs/"+answer.ID, nil); got.Code != http.StatusNotFound {
		t.Fatalf("somebody else read a token's job: %d", got.Code)
	}
}

func TestAWrongTokenAndARevokedOneGetTheSameAnswer(t *testing.T) {
	here := withTokens(t)
	here.signIn(t)
	var id string
	for _, held := range here.server.tokens.List() {
		if held.Name == "ci" {
			id = held.ID
		}
	}
	if got := here.call("GET", "/api/spend", here.secrets["everyone"]); got.Code != http.StatusOK {
		t.Fatalf("a live token gave %d", got.Code)
	}
	before := here.call("GET", "/api/spend", "haas_00000000_nothing")
	if revoked := here.ask(t, "POST", "/admin/tokens/"+id+"/revoke", url.Values{}); revoked.Code != http.StatusSeeOther {
		t.Fatalf("revoking gave %d", revoked.Code)
	}
	after := here.call("GET", "/api/spend", here.secrets["ci"])
	if after.Code != http.StatusUnauthorized || before.Code != http.StatusUnauthorized || after.Body.String() != before.Body.String() {
		t.Fatalf("a revoked token gave %d %q and an unknown one %d %q", after.Code, after.Body.String(), before.Code, before.Body.String())
	}
	if code, _ := here.send(t, here.secrets["ci"], Submission{Spec: aJob()}); code != http.StatusUnauthorized {
		t.Fatalf("a revoked token sent a job: %d", code)
	}
	here.nothing(t)
}

func TestMakingAndRevokingATokenAreRecordedAndNoSecretIs(t *testing.T) {
	here := withTokens(t)
	here.signIn(t)
	made := here.ask(t, "POST", "/admin/tokens", url.Values{"name": {"nightly"}, "scope": {"submit", "spend"}})
	secret := regexp.MustCompile(`haas_[0-9a-f]{8}_[A-Za-z0-9_-]{43}`).FindString(made.Body.String())
	if secret == "" {
		t.Fatalf("the page did not show the secret:\n%s", made.Body.String())
	}
	if made.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("the secret's page may be cached: %q", made.Header().Get("Cache-Control"))
	}
	if again := here.ask(t, "GET", "/admin/tokens", nil).Body.String(); strings.Contains(again, secret) || !strings.Contains(again, "nightly") {
		t.Fatalf("the secret is shown again, or the token is not listed")
	}
	if got := here.call("GET", "/api/spend", secret); got.Code != http.StatusOK {
		t.Fatalf("the token that was made gave %d", got.Code)
	}
	here.call("GET", "/api/changes", secret)

	events := written(t, here.log)
	if len(ofKind(events, changelog.TokenCreated)) != 1 {
		t.Fatalf("making a token was not recorded once: %+v", events)
	}
	if refused := ofKind(events, changelog.Refused); len(refused) != 1 || refused[0].Who != "token:nightly" ||
		!strings.Contains(refused[0].What["why"], "may not audit") {
		t.Fatalf("a token asking for what it may not do was not recorded: %+v", refused)
	}
	raw, err := os.ReadFile(here.log.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) || strings.Contains(string(raw), strings.TrimPrefix(secret, "haas_")[9:]) {
		t.Fatal("the change log holds a token's secret")
	}
	for _, held := range here.server.tokens.List() {
		if held.Name == "nightly" {
			here.ask(t, "POST", "/admin/tokens/"+held.ID+"/revoke", url.Values{})
		}
	}
	if len(ofKind(written(t, here.log), changelog.TokenRevoked)) != 1 {
		t.Fatal("revoking a token was not recorded")
	}
}

func TestTheTokensPageIsAnAdminsAndOnlyOnAServerThatTakesTokens(t *testing.T) {
	here := withTokens(t)
	runner := here.as(t, "amy@acme.test")
	for _, target := range [][2]string{{"GET", "/admin/tokens"}, {"POST", "/admin/tokens"}, {"POST", "/admin/tokens/abcdef12/revoke"}} {
		if got := runner.ask(t, target[0], target[1], url.Values{"name": {"x"}, "scope": {"submit"}}); got.Code != http.StatusForbidden {
			t.Errorf("a person who runs work got %d from %s %s", got.Code, target[0], target[1])
		}
	}
	if n := len(here.server.tokens.List()); n != 4 {
		t.Fatalf("a person who runs work changed the tokens: %d", n)
	}

	plain := open(t)
	plain.signIn(t)
	if got := plain.ask(t, "GET", "/admin/tokens", nil); got.Code != http.StatusNotFound {
		t.Fatalf("a server with no tokens gave %d at /tokens", got.Code)
	}
	if page := plain.ask(t, "GET", "/admin", nil).Body.String(); strings.Contains(page, `href="/admin/tokens"`) {
		t.Fatal("a server with no tokens links to its tokens page")
	}
	here.signIn(t)
	if page := here.ask(t, "GET", "/admin", nil).Body.String(); !strings.Contains(page, `href="/admin/tokens"`) {
		t.Fatal("an admin is not offered the tokens page")
	}
}

func TestATokenNeedsAValidNameAndAScopeAndKeepsNoSecret(t *testing.T) {
	here := withTokens(t)
	here.signIn(t)
	for name, form := range map[string]url.Values{
		"a name in capitals": {"name": {"Nightly Build"}, "scope": {"submit"}},
		"no scope":           {"name": {"nightly"}},
		"a made-up scope":    {"name": {"nightly"}, "scope": {"root"}},
		"a name in use":      {"name": {"ci"}, "scope": {"submit"}},
	} {
		page := here.ask(t, "POST", "/admin/tokens", form).Body.String()
		if !strings.Contains(page, `class="refused"`) || strings.Contains(page, "haas_") {
			t.Errorf("%s was not refused, or showed a secret:\n%s", name, page)
		}
	}
	saved, err := os.ReadFile(here.file)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range here.secrets {
		if strings.Contains(string(saved), strings.SplitN(secret, "_", 3)[2]) {
			t.Fatal("the token file holds a secret")
		}
	}
}

func TestSpendSaysWhoAskedAndTheScriptGetsTheSame(t *testing.T) {
	here := withTokens(t)
	root := here.index.Root()
	for id, asked := range map[string]string{"by-token": "token:ci", "by-amy": "amy@acme.test", "by-api": sentOverHTTP} {
		spentJob(t, root, id, "", priced)
		if err := here.index.Add(jobs.Record{ID: id, State: jobs.Passed, Brief: "b", Asked: asked}); err != nil {
			t.Fatal(err)
		}
	}
	here.signIn(t)
	page := here.ask(t, "GET", "/spend?month=2026-09", nil).Body.String()
	for _, want := range []string{"By who asked", "Token ci", "amy@acme.test", "Sent over the API"} {
		if !strings.Contains(page, want) {
			t.Errorf("the spend page lacks %q", want)
		}
	}
	var report struct {
		Askers []struct {
			Name string  `json:"name"`
			USD  float64 `json:"usd"`
		} `json:"askers"`
	}
	got := here.call("GET", "/api/spend?month=2026-09", here.secrets["finance"])
	if err := json.Unmarshal(got.Body.Bytes(), &report); err != nil {
		t.Fatalf("%v: %s", err, got.Body.String())
	}
	if len(report.Askers) != 3 || report.Askers[0].USD != 0.25 {
		t.Fatalf("askers %+v", report.Askers)
	}
}
