package proxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/pkg/spend"
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
	proxy := New(tokens, Credentials{"": {Address: model.URL, Key: key, Name: "the-platform-key"}},
		func(job string) (string, bool) { return filepath.Join(kept, job+".jsonl"), true },
		spend.NewLedger(spend.Policy{}), Allowlist{})
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
	token, err := tokens.Mint("a-job", "")
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
	token, err := tokens.Mint("a-job", "")
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
	first, _ := tokens.Mint("job-one", "")
	second, _ := tokens.Mint("job-two", "")
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
	token, _ := tokens.Mint("a-job", "")
	ask(t, proxy, "/model/"+token+"/v1/chat/completions", `{"model":"x"}`)

	text, err := os.ReadFile(filepath.Join(kept, "a-job.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var seen spend.Call
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(text))), &seen); err != nil {
		t.Fatal(err)
	}
	if seen.Path != "/v1/chat/completions" || seen.Status != 200 || seen.Bytes == 0 {
		t.Fatalf("the record is %+v", seen)
	}
	if seen.Credential != "the-platform-key" {
		t.Fatalf("the record names the credential as %q", seen.Credential)
	}
	if strings.Contains(string(text), "the-real-key") {
		t.Fatal("the credential itself is in the record")
	}
}

func TestAModelThatNeedsNoCredentialGetsNone(t *testing.T) {
	proxy, tokens, _, seen := site(t, "")
	token, _ := tokens.Mint("a-job", "")
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
	token, err := tokens.Mint("a-job", "")
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

// A host that signs its tokens and keeps its keys elsewhere plugs both into the proxy.
type signedTokens map[string]Ticket

func (s signedTokens) Job(token string) (Ticket, bool) { ticket, ok := s[token]; return ticket, ok }

type keysByJob struct {
	address string
	keys    map[string]string
	asked   []string
}

func (k *keysByJob) Credential(_ context.Context, ticket Ticket) (Credential, bool) {
	k.asked = append(k.asked, ticket.Job)
	key, ok := k.keys[ticket.Job]
	if !ok {
		return Credential{}, false
	}
	return Credential{Name: "asker-" + ticket.Job, Address: k.address, Key: key, Auth: Anthropic}, true
}

func TestATokenAndAKeyFromElsewhereAreUsedInPlaceOfTheHeldOnes(t *testing.T) {
	proxy, _, _, seen := site(t, "the-held-key")
	keys := &keysByJob{keys: map[string]string{"lease-9": "the-askers-key"}}
	var model *httptest.Server
	model = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*seen = r.Header.Clone()
		w.Write([]byte(`{}`))
	}))
	defer model.Close()
	keys.address = model.URL
	proxy.UseTokens(signedTokens{"v1.signed": {Job: "lease-9"}, "v1.keyless": {Job: "lease-10"}})
	proxy.UseCredentials(keys)

	if answer := ask(t, proxy, "/model/v1.signed/v1/messages", `{"model":"x"}`); answer.Code != http.StatusOK {
		t.Fatalf("code %d", answer.Code)
	}
	if got := seen.Get("x-api-key"); got != "the-askers-key" {
		t.Fatalf("the model was sent %q, not the key the source gave", got)
	}
	if answer := ask(t, proxy, "/model/v1.keyless/v1/messages", `{"model":"x"}`); answer.Code != http.StatusBadGateway {
		t.Fatalf("a ticket with no key answered %d", answer.Code)
	}
	if answer := ask(t, proxy, "/model/the-held-minted-token/v1/messages", `{}`); answer.Code != http.StatusForbidden {
		t.Fatalf("a token the source does not know answered %d", answer.Code)
	}
	if len(keys.asked) != 2 {
		t.Fatalf("the source was asked %v: once per call that had a ticket", keys.asked)
	}
}

func TestTheSignedTokenAlsoOpensTheForwardProxy(t *testing.T) {
	proxy, _, _, _ := site(t, "k")
	proxy.UseTokens(signedTokens{"v1.signed": {Job: "lease-9"}})
	request := httptest.NewRequest("CONNECT", "http://example.com:443", nil)
	request.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("vuxto:v1.signed")))
	recorder := httptest.NewRecorder()
	proxy.Handler().ServeHTTP(recorder, request)
	if recorder.Code == http.StatusProxyAuthRequired {
		t.Fatal("the proxy did not take the token the source knows")
	}
	bad := httptest.NewRequest("CONNECT", "http://example.com:443", nil)
	bad.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("vuxto:unknown")))
	refused := httptest.NewRecorder()
	proxy.Handler().ServeHTTP(refused, bad)
	if refused.Code != http.StatusProxyAuthRequired {
		t.Fatalf("an unknown token answered %d", refused.Code)
	}
}

func TestUseCredentialsWithNilGoesBackToTheHeldOnes(t *testing.T) {
	proxy, tokens, _, seen := site(t, "the-held-key")
	proxy.UseCredentials(&keysByJob{})
	proxy.UseCredentials(nil)
	token, _ := tokens.Mint("a-job", "")
	if answer := ask(t, proxy, "/model/"+token+"/v1/models", `{}`); answer.Code != http.StatusOK || seen.Get("Authorization") != "Bearer the-held-key" {
		t.Fatalf("code %d, sent %q", answer.Code, seen.Get("Authorization"))
	}
}

func TestTheCallARequestMadeIsToldToTheResultItsContextCarries(t *testing.T) {
	proxy, tokens, _, _ := site(t, "k")
	token, _ := tokens.Mint("a-job", "")
	ctx, result := WithResult(context.Background())
	request := httptest.NewRequest("POST", "/model/"+token+"/v1/chat/completions", strings.NewReader(`{"model":"some-model"}`)).WithContext(ctx)
	recorder := httptest.NewRecorder()
	proxy.Handler().ServeHTTP(recorder, request)
	call, made := result.Call()
	if !made || call.Model != "some-model" || call.Status != http.StatusOK {
		t.Fatalf("made %v, %+v", made, call)
	}
	plain := httptest.NewRecorder()
	proxy.Handler().ServeHTTP(plain, httptest.NewRequest("POST", "/model/"+token+"/v1/chat/completions", strings.NewReader(`{"model":"x"}`)))
	if _, made := (&Result{}).Call(); made {
		t.Fatal("an empty result says a call was made")
	}
}

type countingTransport struct{ calls int }

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.calls++
	return http.DefaultTransport.RoundTrip(r)
}

func TestAHostsOwnClientCarriesTheModelCall(t *testing.T) {
	proxy, tokens, _, _ := site(t, "k")
	carrier := &countingTransport{}
	proxy.UseClient(&http.Client{Transport: carrier, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }})
	token, _ := tokens.Mint("a-job", "")
	if answer := ask(t, proxy, "/model/"+token+"/v1/models", `{}`); answer.Code != http.StatusOK || carrier.calls != 1 {
		t.Fatalf("code %d, the client carried %d calls", answer.Code, carrier.calls)
	}
}
