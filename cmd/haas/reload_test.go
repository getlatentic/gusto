package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/web"
)

type fakePages struct {
	mu       sync.Mutex
	models   func() web.Models
	notes    []string
	building bool
}

func (f *fakePages) UseLiveModels(models func() web.Models) {
	f.mu.Lock()
	f.models = models
	f.mu.Unlock()
}
func (f *fakePages) UseInBuilding(b bool) { f.mu.Lock(); f.building = b; f.mu.Unlock() }
func (f *fakePages) NoteSetting(setting, was, now, refused string) {
	f.mu.Lock()
	f.notes = append(f.notes, setting+" "+refused)
	f.mu.Unlock()
}

// reloading is a server's way out with one credential that reaches a provider which says which key it was called with, a live
// settings file, a spend policy, and the pieces that read them again.
type reloading struct {
	t         *testing.T
	way       *WayOut
	live      *liveSettings
	pages     *fakePages
	index     *jobs.Index
	creds     string
	liveFile  string
	spend     string
	front     *httptest.Server
	token     string
	keys      []string
	bodies    []string
	redaction string
	mu        sync.Mutex
}

func newReloading(t *testing.T) *reloading {
	t.Helper()
	r := &reloading{t: t, pages: &fakePages{}}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		sent, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.bodies = append(r.bodies, string(sent))
		r.keys = append(r.keys, req.Header.Get("Authorization"))
		r.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"x","choices":[{"message":{"role":"assistant","content":"hi"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	t.Cleanup(provider.Close)
	dir := t.TempDir()
	r.creds, r.liveFile, r.spend = filepath.Join(dir, "credentials"), filepath.Join(dir, "live.json"), filepath.Join(dir, "spend.json")
	os.MkdirAll(r.creds, 0o755)
	r.setKey(provider.URL, "key-one")
	t.Setenv("HAAS_CREDENTIALS", r.creds)
	t.Setenv("HAAS_LIVE", r.liveFile)
	t.Setenv("HAAS_SPEND_POLICY", r.spend)
	r.redaction = filepath.Join(dir, "redaction.json")
	t.Setenv("HAAS_REDACTION", r.redaction)
	t.Setenv("HAAS_MODELS", "platform/first")
	t.Setenv("HAAS_LIMIT_PERSON", "1")
	os.WriteFile(r.spend, []byte(`{"teams":{"payments":{"usdPerMonth":10}}}`), 0o644)
	jobsRoot := filepath.Join(dir, "jobs")
	var err error
	if r.index, err = jobs.Open(jobsRoot); err != nil {
		t.Fatal(err)
	}
	if r.way, err = openWayOut(jobsRoot); err != nil {
		t.Fatal(err)
	}
	if r.live, err = startLive(t.Context(), r.way, r.index, r.pages, false); err != nil {
		t.Fatal(err)
	}
	r.front = httptest.NewServer(r.way.proxy.Handler())
	t.Cleanup(r.front.Close)
	if r.token, err = r.way.tokens.Mint("20261008-1200-a-turn", "platform"); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *reloading) setKey(address, key string) {
	r.t.Helper()
	text := `{"address":"` + address + `","key":"` + key + `","auth":"bearer"}`
	if err := os.WriteFile(filepath.Join(r.creds, "platform.json"), []byte(text), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

// call is one model call by the running job, and the key the provider saw.
func (r *reloading) call() string {
	r.t.Helper()
	request, _ := http.NewRequest("POST", r.front.URL+"/model/v1/chat/completions", strings.NewReader(`{"model":"first","messages":[{"role":"user","content":"hi"}]}`))
	request.Header.Set("Authorization", "Bearer "+r.token)
	request.Header.Set("Content-Type", "application/json")
	answer, err := http.DefaultClient.Do(request)
	if err != nil {
		r.t.Fatal(err)
	}
	defer answer.Body.Close()
	io.Copy(io.Discard, answer.Body)
	if answer.StatusCode != http.StatusOK {
		r.t.Fatalf("a model call answered %d", answer.StatusCode)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.keys[len(r.keys)-1]
}

func TestAJobThatIsRunningKeepsCallingItsModelWhenTheCredentialAndTheLimitsChange(t *testing.T) {
	r := newReloading(t)
	if got := r.call(); got != "Bearer key-one" {
		t.Fatalf("the provider saw %q", got)
	}
	if got := r.index.Limits(); got.Person != 1 || got.Team != 0 {
		t.Fatalf("limits at the start: %+v", got)
	}

	os.WriteFile(r.liveFile, []byte(`{"limitPerson": 3, "limitTeam": 5, "models": ["platform/first", "platform/second"]}`), 0o644)
	if err := r.live.applyFile(); err != nil {
		t.Fatal(err)
	}
	if got := r.index.Limits(); got.Person != 3 || got.Team != 5 {
		t.Errorf("limits after the change: %+v", got)
	}
	if offered := r.pages.models(); strings.Join(offered, ",") != "platform/first,platform/second" {
		t.Errorf("the models offered after the change: %v", offered)
	}

	credentials, _ := os.ReadFile(filepath.Join(r.creds, "platform.json"))
	address := strings.Split(strings.Split(string(credentials), `"address":"`)[1], `"`)[0]
	r.setKey(address, "key-two")
	if err := r.live.applyCredentials(); err != nil {
		t.Fatal(err)
	}
	if got := r.call(); got != "Bearer key-two" {
		t.Errorf("the job's next call went with %q, and the credential changed under it", got)
	}
}

func TestASettingThatDoesNotReadLeavesWhatWasInForce(t *testing.T) {
	r := newReloading(t)
	os.WriteFile(r.liveFile, []byte(`{"limitPersn": 9}`), 0o644)
	if err := r.live.applyFile(); err == nil {
		t.Error("a misspelt setting was taken")
	}
	if got := r.index.Limits(); got.Person != 1 {
		t.Errorf("the limit after a refused file is %d, and it was 1", got.Person)
	}
	os.WriteFile(r.spend, []byte(`{"teams":{"payments":{"usdPerMonthh":1}}}`), 0o644)
	if err := r.live.applySpend(); err == nil {
		t.Error("a misspelt cap was taken")
	}
	if got := r.way.ledger.Policy().Teams["payments"].USDPerMonth; got != 10 {
		t.Errorf("the cap after a refused file is %v, and it was 10", got)
	}
	if err := os.RemoveAll(r.creds); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(r.creds, 0o755)
	if err := r.live.applyCredentials(); err == nil {
		t.Error("an empty credentials folder was taken, and the proxy would hold nothing")
	}
	if got := r.call(); got != "Bearer key-one" {
		t.Errorf("after a refused folder the provider saw %q", got)
	}
}

func TestACapChangeKeepsWhatWasSpentAndAppliesFromTheNextCall(t *testing.T) {
	r := newReloading(t)
	os.WriteFile(r.spend, []byte(`{"teams":{"payments":{"usdPerMonth":25}},"organisation":{"usdPerMonth":100}}`), 0o644)
	if err := r.live.applySpend(); err != nil {
		t.Fatal(err)
	}
	policy := r.way.ledger.Policy()
	if policy.Teams["payments"].USDPerMonth != 25 || policy.Organisation == nil || policy.Organisation.USDPerMonth != 100 {
		t.Errorf("policy = %+v", policy)
	}
}

func TestTheWatcherTakesAChangeWithoutAnyoneAskingForIt(t *testing.T) {
	t.Setenv("HAAS_RELOAD_SECONDS", "1")
	r := newReloading(t)
	time.Sleep(200 * time.Millisecond)
	os.WriteFile(r.liveFile, []byte(`{"limitPerson": 7}`), 0o644)
	deadline := time.Now().Add(8 * time.Second)
	for r.index.Limits().Person != 7 {
		if time.Now().After(deadline) {
			t.Fatal("a change to the settings file was not taken in eight seconds at a one-second cadence")
		}
		time.Sleep(50 * time.Millisecond)
	}
	r.pages.mu.Lock()
	defer r.pages.mu.Unlock()
	if len(r.pages.notes) != 1 || !strings.HasPrefix(r.pages.notes[0], "HAAS_LIVE") {
		t.Errorf("the change record was told %v", r.pages.notes)
	}
}

func TestRedactionTurnedOnInTheFileReachesTheNextCallOfARunningJob(t *testing.T) {
	r := newReloading(t)
	send := func() string {
		request, _ := http.NewRequest("POST", r.front.URL+"/model/v1/chat/completions", strings.NewReader(`{"model":"first","messages":[{"role":"user","content":"mail ada@example.com and AKIAIOSFODNN7EXAMPLE"}]}`))
		request.Header.Set("Authorization", "Bearer "+r.token)
		answer, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, answer.Body)
		answer.Body.Close()
		r.mu.Lock()
		defer r.mu.Unlock()
		return r.bodies[len(r.bodies)-1]
	}
	if got := send(); !strings.Contains(got, "ada@example.com") {
		t.Fatalf("redaction was on before it was asked for: %s", got)
	}
	os.WriteFile(r.redaction, []byte(`{"patterns":["email","key"]}`), 0o644)
	if err := r.live.applyRedaction(); err != nil {
		t.Fatal(err)
	}
	if got := send(); strings.Contains(got, "ada@example.com") || strings.Contains(got, "AKIAIOSFODNN7EXAMPLE") || !strings.Contains(got, "[redacted:email]") {
		t.Errorf("the provider was sent %s", got)
	}
	os.WriteFile(r.redaction, []byte(`{"patterns":["emial"]}`), 0o644)
	if err := r.live.applyRedaction(); err == nil {
		t.Error("a misspelt kind was taken")
	}
	if got := send(); strings.Contains(got, "ada@example.com") {
		t.Errorf("a refused file turned redaction off: %s", got)
	}
}

func TestAServerWithNoProxyCannotPromiseToMaskWhatItSends(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "redaction.json")
	os.WriteFile(file, []byte(`{"patterns":["email"]}`), 0o644)
	t.Setenv("HAAS_REDACTION", file)
	t.Setenv("HAAS_CREDENTIALS", "")
	t.Setenv("HAAS_MODEL_UPSTREAM", "")
	index, err := jobs.Open(filepath.Join(dir, "jobs"))
	if err != nil {
		t.Fatal(err)
	}
	way, err := openWayOut(filepath.Join(dir, "jobs"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := startLive(t.Context(), way, index, &fakePages{}, false); err == nil || !strings.Contains(err.Error(), "no proxy") {
		t.Errorf("err = %v", err)
	}
}
