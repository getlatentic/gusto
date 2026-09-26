package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// upstream stands in for the model, and records what it was actually sent.
func upstream(t *testing.T, seen *http.Header) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = r.Header.Clone()
		seen.Set("X-Seen-Path", r.URL.Path)
		body, _ := io.ReadAll(r.Body)
		seen.Set("X-Seen-Body", string(body))
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)
	return server
}

func site(t *testing.T, key string) (*Proxy, *Tokens, string, *http.Header) {
	t.Helper()
	var seen http.Header
	model := upstream(t, &seen)
	tokens := NewTokens()
	kept := t.TempDir()
	proxy := New(tokens, Upstream{Address: model.URL, Key: key, Name: "the-platform-key"},
		func(job string) (string, bool) { return filepath.Join(kept, job+".jsonl"), true })
	return proxy, tokens, kept, &seen
}

func ask(t *testing.T, proxy *Proxy, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest("POST", path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer a-container-should-not-be-trusted")
	recorder := httptest.NewRecorder()
	proxy.Handler().ServeHTTP(recorder, request)
	return recorder
}

func TestTheCredentialIsAddedHereAndTheContainersIsDiscarded(t *testing.T) {
	proxy, tokens, _, seen := site(t, "the-real-key")
	token, err := tokens.Mint("a-job")
	if err != nil {
		t.Fatal(err)
	}
	answer := ask(t, proxy, "/model/"+token+"/v1/chat/completions", `{"model":"x"}`)
	if answer.Code != http.StatusOK {
		t.Fatalf("code %d", answer.Code)
	}
	if got := seen.Get("Authorization"); got != "Bearer the-real-key" {
		t.Fatalf("the model was sent %q", got)
	}
	if got := seen.Get("X-Seen-Path"); got != "/v1/chat/completions" {
		t.Fatalf("the model was asked for %q", got)
	}
}

func TestATokenNobodyMintedGetsNothing(t *testing.T) {
	proxy, _, _, seen := site(t, "the-real-key")
	answer := ask(t, proxy, "/model/not-a-token/v1/chat/completions", `{}`)
	if answer.Code != http.StatusForbidden {
		t.Fatalf("code %d", answer.Code)
	}
	if seen.Get("X-Seen-Path") != "" {
		t.Fatal("a request with no token reached the model")
	}
}

func TestAJobsWayOutEndsWithTheJob(t *testing.T) {
	proxy, tokens, _, _ := site(t, "the-real-key")
	token, err := tokens.Mint("a-job")
	if err != nil {
		t.Fatal(err)
	}
	if answer := ask(t, proxy, "/model/"+token+"/v1/models", `{}`); answer.Code != http.StatusOK {
		t.Fatalf("code %d while the job runs", answer.Code)
	}
	tokens.Drop("a-job")
	if answer := ask(t, proxy, "/model/"+token+"/v1/models", `{}`); answer.Code != http.StatusForbidden {
		t.Fatalf("code %d after the job ended", answer.Code)
	}
}

func TestOneJobsTokenIsNotAnothers(t *testing.T) {
	_, tokens, _, _ := site(t, "k")
	first, _ := tokens.Mint("job-one")
	second, _ := tokens.Mint("job-two")
	if first == second {
		t.Fatal("two jobs were given the same way out")
	}
	tokens.Drop("job-one")
	if _, still := tokens.Job(second); !still {
		t.Fatal("ending one job took another's way out with it")
	}
}

// What the proxy writes down is independent of what the harness claims, which
// is how a harness that under-reports is caught.
func TestWhatActuallyWentIsWrittenDown(t *testing.T) {
	proxy, tokens, kept, _ := site(t, "the-real-key")
	token, _ := tokens.Mint("a-job")
	ask(t, proxy, "/model/"+token+"/v1/chat/completions", `{"model":"x"}`)

	text, err := os.ReadFile(filepath.Join(kept, "a-job.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var seen record
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(text))), &seen); err != nil {
		t.Fatal(err)
	}
	if seen.Path != "/v1/chat/completions" || seen.Status != 200 || seen.Bytes == 0 {
		t.Fatalf("the record is %+v", seen)
	}
	if seen.Model != "the-platform-key" {
		t.Fatalf("the record names the credential as %q", seen.Model)
	}
	if strings.Contains(string(text), "the-real-key") {
		t.Fatal("the credential itself is in the record")
	}
}

func TestAModelThatNeedsNoCredentialGetsNone(t *testing.T) {
	proxy, tokens, _, seen := site(t, "")
	token, _ := tokens.Mint("a-job")
	ask(t, proxy, "/model/"+token+"/v1/models", `{}`)
	if got := seen.Get("Authorization"); got != "" {
		t.Fatalf("an unauthenticated model was sent %q", got)
	}
}

// A harness keeps its base URL in static configuration and its key in the
// Authorization header. Requiring the token in the path would mean rewriting
// that configuration for every job.
func TestATokenPresentedWhereAKeyWouldGoIsAccepted(t *testing.T) {
	proxy, tokens, _, seen := site(t, "the-real-key")
	token, err := tokens.Mint("a-job")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/model/v1/chat/completions", strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	proxy.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("code %d", recorder.Code)
	}
	if got := seen.Get("X-Seen-Path"); got != "/v1/chat/completions" {
		t.Fatalf("the model was asked for %q", got)
	}
	if got := seen.Get("Authorization"); got != "Bearer the-real-key" {
		t.Fatalf("the model was sent %q", got)
	}
}

func TestAKeyThatIsNotATokenStillGetsNothing(t *testing.T) {
	proxy, _, _, seen := site(t, "the-real-key")
	request := httptest.NewRequest("POST", "/model/v1/chat/completions", strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer sk-somebody-elses-key")
	recorder := httptest.NewRecorder()
	proxy.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("code %d", recorder.Code)
	}
	if seen.Get("X-Seen-Path") != "" {
		t.Fatal("a request with somebody else's key reached the model")
	}
}
