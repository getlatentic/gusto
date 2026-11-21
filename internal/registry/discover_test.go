package registry

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/getlatentic/gusto/pkg/proxy"
	"github.com/getlatentic/gusto/pkg/spend"
	"github.com/getlatentic/gusto/pkg/wire"
)

const gatewayKey = "otari-key-that-must-stay-on-the-server"

// gateway is a stand-in for an Otari-shaped gateway: one model list with prices, and the three shapes of call, each answered
// with usage. It remembers what it was sent.
type gateway struct {
	*httptest.Server
	mu   sync.Mutex
	seen []sent
}

type sent struct{ method, path, authorization, apiKey, body string }

func newGateway(t *testing.T, models string) *gateway {
	t.Helper()
	g := &gateway{}
	g.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		g.mu.Lock()
		g.seen = append(g.seen, sent{r.Method, r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("x-api-key"), string(body)})
		g.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+gatewayKey {
			http.Error(w, "no key", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/models":
			io.WriteString(w, models)
		case "/api/v1/chat/completions":
			io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"hi"}}],"usage":{"prompt_tokens":1000000,"completion_tokens":500000}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(g.Close)
	return g
}

const otariListing = `{"object":"list","data":[
 {"id":"deepseek/deepseek-v4.1-flash","object":"model","created":1,"owned_by":"deepseek","type":"chat",
  "pricing":{"input_price_per_million":0.14,"output_price_per_million":0.28,"cache_read_price_per_million":0.014,"unit":"token"}},
 {"id":"openai:gpt-4o-mini","object":"model","created":1,"owned_by":"openai","pricing":{"input_price_per_million":0.15,"output_price_per_million":0.6}},
 {"id":"openai/text-embedding-3-small","object":"model","created":1,"owned_by":"openai","type":"embedding"},
 {"id":"mystery/unpriced","object":"model","created":1,"owned_by":"mystery"},
 {"id":"image/per-image","object":"model","created":1,"owned_by":"x","type":"chat","pricing":{"input_price_per_million":1,"output_price_per_million":1,"unit":"image"}}
]}`

func connected(t *testing.T, g *gateway, wires ...string) *Registry {
	t.Helper()
	r, _ := opened(t, Rules{PrivateAddresses: true})
	err := r.AddProvider(Give{Name: "otari", Address: g.URL + "/api", Reference: "stored", Key: gatewayKey, Wires: wires})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAGatewaysModelsAreFoundOnceWithTheirPricesAndOnlyChatModelsAreKept(t *testing.T) {
	g := newGateway(t, otariListing)
	r := connected(t, g, "chat", "messages", "responses")
	listed, added, err := r.Import(context.Background(), "otari")
	if err != nil {
		t.Fatal(err)
	}
	if listed != 4 || added != 4 {
		t.Fatalf("the embedding model is left out and the other four are added: listed %d, added %d", listed, added)
	}
	byName := map[string]Model{}
	for _, m := range r.Document().Models {
		byName[m.Name] = m
		if !m.Pending {
			t.Errorf("%s is on before an administrator turned it on", m.Name)
		}
	}
	if p := byName["deepseek/deepseek-v4.1-flash"].Price; p == nil || p.InputPerMillion != 0.14 || p.CachedInputPerMillion == nil || *p.CachedInputPerMillion != 0.014 {
		t.Errorf("the gateway's price is kept: %+v", p)
	}
	if byName["mystery/unpriced"].Price != nil {
		t.Error("a model with no price is not priced at nothing")
	}
	if byName["image/per-image"].Price != nil {
		t.Error("a price per image is not a price per million tokens")
	}
	if got := g.seen[0]; got.method != "GET" || got.path != "/api/v1/models" || got.authorization != "Bearer "+gatewayKey {
		t.Errorf("the list was asked for as %+v", got)
	}
	if _, _, err := r.Import(context.Background(), "otari"); err == nil || !strings.Contains(err.Error(), "already") {
		t.Errorf("finding them again adds nothing and says so: %v", err)
	}
}

func TestAGatewayThatRefusesTheKeyIsSaidSoWithoutTheKeyInTheWords(t *testing.T) {
	g := newGateway(t, otariListing)
	r, _ := opened(t, Rules{PrivateAddresses: true})
	if err := r.AddProvider(Give{Name: "otari", Address: g.URL + "/api", Reference: "stored", Key: "a-wrong-key-1234"}); err != nil {
		t.Fatal(err)
	}
	_, _, err := r.Import(context.Background(), "otari")
	if err == nil || !strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "a-wrong-key-1234") {
		t.Errorf("%v", err)
	}
	if len(r.Document().Models) != 0 {
		t.Error("a refused list added models")
	}
}

func TestAnAddressOnAPrivateNetworkIsNotAskedForItsModelsUnlessTheOperatorAllowedIt(t *testing.T) {
	g := newGateway(t, otariListing)
	r, _ := opened(t, Rules{PrivateAddresses: true})
	if err := r.AddProvider(Give{Name: "otari", Address: g.URL + "/api", Reference: "stored", Key: gatewayKey}); err != nil {
		t.Fatal(err)
	}
	r.rules.PrivateAddresses = false
	_, _, err := r.Import(context.Background(), "otari")
	if err == nil {
		t.Fatal("the registry reached this machine for an organization")
	}
	if len(g.seen) != 0 {
		t.Errorf("the gateway was reached: %+v", g.seen)
	}
}

func TestAModelThroughAGatewayIsPricedByTheLedgerAndTheKeyNeverLeavesTheServer(t *testing.T) {
	g := newGateway(t, otariListing)
	r := connected(t, g, "chat", "messages", "responses")
	if _, _, err := r.Import(context.Background(), "otari"); err != nil {
		t.Fatal(err)
	}
	for _, step := range [][2]string{{"otari", ""}, {"otari", "deepseek/deepseek-v4.1-flash"}} {
		if err := r.Enable(step[0], step[1]); err != nil {
			t.Fatal(err)
		}
	}
	held := r.Effective()
	if len(held.Models) != 2 || held.Models[1] != "otari/deepseek/deepseek-v4.1-flash" {
		t.Fatalf("only what was turned on is offered: %v", held.Models)
	}
	ledger := spend.NewLedger(spend.Policy{Prices: held.Prices})
	ledger.Begin("a-job", spend.Terms{})
	tokens := proxy.NewTokens()
	kept := t.TempDir()
	p := proxy.New(tokens, held.Credentials, func(job string) (string, bool) { return filepath.Join(kept, job+".jsonl"), true }, ledger, proxy.Allowlist{})
	p.UseClient(&http.Client{})
	token, err := tokens.Mint("a-job", "otari")
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/model/"+token+"/v1/chat/completions", strings.NewReader(`{"model":"deepseek/deepseek-v4.1-flash","messages":[]}`))
	request.Header.Set("Authorization", "Bearer what-the-container-holds")
	answer := httptest.NewRecorder()
	p.Handler().ServeHTTP(answer, request)
	if answer.Code != http.StatusOK {
		t.Fatalf("%d %s", answer.Code, answer.Body)
	}
	call := g.seen[len(g.seen)-1]
	if call.path != "/api/v1/chat/completions" || call.authorization != "Bearer "+gatewayKey {
		t.Errorf("the gateway was sent %+v", call)
	}
	if strings.Contains(answer.Body.String(), gatewayKey) {
		t.Error("the key is in the answer")
	}
	tally, _ := ledger.Task("a-job")
	want := 1*0.14 + 0.5*0.28
	if tally.Calls != 1 || tally.InputTokens != 1000000 || tally.OutputTokens != 500000 || tally.USD < want-1e-9 || tally.USD > want+1e-9 {
		t.Errorf("the call is counted and priced with the gateway's own rate: %+v, wanted $%v", tally, want)
	}
}

func TestAProviderThatAnswersAllThreeShapesIsNotTranslatedForAny(t *testing.T) {
	g := newGateway(t, otariListing)
	r := connected(t, g, "chat", "messages", "responses")
	credential := r.Document().Providers[0]
	held := proxy.Credential{Wires: credential.Wires}
	for _, shape := range []string{"chat", "messages", "responses"} {
		if !held.Answers(wire.Shape(shape)) {
			t.Errorf("a gateway that names %s is sent %s calls as they are", shape, shape)
		}
	}
}
