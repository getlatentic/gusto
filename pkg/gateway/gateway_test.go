package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/getlatentic/gusto/pkg/redact"
)

func namesOf(t *testing.T, h *harness) []string {
	t.Helper()
	listing := h.send("tools/list", nil).Result.(map[string]any)["tools"].([]any)
	var names []string
	for _, one := range listing {
		names = append(names, one.(map[string]any)["name"].(string))
	}
	return names
}

func reports(t *testing.T) (*upstream, Server) {
	fake, address := newUpstream(t,
		tool("get_report", "Fetch a quarterly report by its id. Returns the text."),
		tool("list_reports", "List the reports there are."),
		tool("delete_report", "Delete a report for good."),
		tool("explode", "Always fails."),
		tool("send_email", "Send an email to a customer."),
	)
	return fake, Server{Name: "reports", URL: address, Headers: map[string]string{"Authorization": "Bearer s3cret"}}
}

func TestAJobIsShownThreeToolsAndFindsAndCallsTheOthersByName(t *testing.T) {
	fake, server := reports(t)
	h := start(t, Config{Servers: []Server{server}})
	if got := namesOf(t, h); strings.Join(got, ",") != "tool_search,tool_schema,tool_call" {
		t.Fatalf("the job is shown %v", got)
	}
	found, failed := h.call("tool_search", map[string]any{"query": "report text"})
	if failed || !strings.HasPrefix(found, "reports/get_report:") {
		t.Fatalf("search found %q", found)
	}
	schema, _ := h.call("tool_schema", map[string]any{"name": "reports/get_report"})
	if !strings.Contains(schema, `"inputSchema"`) || !strings.Contains(schema, "an identifier") {
		t.Errorf("schema = %s", schema)
	}
	said, failed := h.call("tool_call", map[string]any{"name": "reports/get_report", "arguments": map[string]any{"id": "q3"}})
	if failed || said != `did get_report with {"id":"q3"}` {
		t.Fatalf("the call answered %q (failed %v)", said, failed)
	}
	if len(fake.calls) != 1 || fake.calls[0] != `get_report {"id":"q3"}` {
		t.Errorf("the server was called %v", fake.calls)
	}
	if _, failed := h.call("tool_call", map[string]any{"name": "reports/explode"}); !failed {
		t.Error("a tool that says it failed is not passed on as failed")
	}
	if _, failed := h.call("get_report", nil); !failed {
		t.Error("a tool that is not one of the three was called by its bare name in discover mode")
	}
}

func TestEveryCallIsWrittenDownWithWhoMadeItAndWhatCameOfIt(t *testing.T) {
	_, server := reports(t)
	h := start(t, Config{Servers: []Server{server}})
	h.send("tools/list", nil)
	h.call("tool_call", map[string]any{"name": "reports/get_report", "arguments": map[string]any{"id": "q3"}})
	h.call("tool_call", map[string]any{"name": "reports/explode"})
	var calls []Entry
	for _, entry := range h.entries() {
		if entry.Kind == "call" {
			calls = append(calls, entry)
		}
	}
	if len(calls) != 2 || calls[0].Outcome != "ok" || calls[1].Outcome != "error" {
		t.Fatalf("calls = %+v", calls)
	}
	first := calls[0]
	if first.Person != "ada@acme.example" || first.Job != "20261008-1" || first.Server != "reports" || first.Tool != "get_report" || first.ArgBytes != len(`{"id":"q3"}`) || first.ResultBytes == 0 || first.At == 0 {
		t.Errorf("entry = %+v", first)
	}
	data, _ := os.ReadFile(h.record)
	if strings.Contains(string(data), "q3") || strings.Contains(string(data), "s3cret") {
		t.Errorf("the record holds what was sent or a credential: %s", data)
	}
}

func TestAServersCredentialsStayInTheGatewayAndAreSentToTheServer(t *testing.T) {
	fake, server := reports(t)
	h := start(t, Config{Servers: []Server{server}})
	var seen []string
	for _, reply := range []response{h.send("initialize", map[string]any{"protocolVersion": "2025-06-18"}), h.send("tools/list", nil)} {
		text, _ := json.Marshal(reply)
		seen = append(seen, string(text))
	}
	said, _ := h.call("tool_search", map[string]any{})
	seen = append(seen, said)
	h.call("tool_call", map[string]any{"name": "reports/get_report"})
	for _, text := range seen {
		if strings.Contains(text, "s3cret") || strings.Contains(text, server.URL) {
			t.Errorf("a credential or an address reached the job: %s", text)
		}
	}
	if len(fake.headers) == 0 || fake.headers[len(fake.headers)-1].Get("Authorization") != "Bearer s3cret" {
		t.Error("the server was not called with its credential")
	}
}

func TestAToolThePolicyTakesFromACallerIsNotFoundAndIsRefusedWithWhyAndRecorded(t *testing.T) {
	fake, server := reports(t)
	policy := Policy{Rules: []Rule{
		{Server: "reports", Tool: "delete_*", Effect: "deny", Why: "deleting reports is for the finance team"},
		{Server: "reports", Tool: "send_email", Team: "support", Effect: "allow"},
		{Server: "reports", Tool: "send_email", Effect: "deny"},
	}}
	h := start(t, Config{Servers: []Server{server}, Policy: policy})
	found, _ := h.call("tool_search", map[string]any{"query": "delete"})
	if strings.Contains(found, "delete_report") || strings.Contains(found, "send_email") {
		t.Errorf("a tool the caller may not use was offered: %q", found)
	}
	said, failed := h.call("tool_call", map[string]any{"name": "reports/delete_report", "arguments": map[string]any{"id": "q3"}})
	if !failed || said != "deleting reports is for the finance team" {
		t.Errorf("the refusal said %q (failed %v)", said, failed)
	}
	if _, failed := h.call("tool_call", map[string]any{"name": "reports/send_email"}); !failed {
		t.Error("a tool another team may use was called")
	}
	if len(fake.calls) != 0 {
		t.Errorf("a refused tool reached the server: %v", fake.calls)
	}
	var refused []Entry
	for _, entry := range h.entries() {
		if entry.Outcome == "refused" {
			refused = append(refused, entry)
		}
	}
	if len(refused) != 2 || refused[0].Tool != "delete_report" || refused[0].Why != "deleting reports is for the finance team" || refused[0].Person != "ada@acme.example" {
		t.Errorf("refusals recorded: %+v", refused)
	}
	support := start(t, Config{Servers: []Server{server}, Policy: policy, Caller: Caller{Job: "j2", Person: "bo@acme.example", Team: "support"}})
	if _, failed := support.call("tool_call", map[string]any{"name": "reports/send_email"}); failed {
		t.Error("the team a rule names was refused")
	}
}

func TestAHarnessThatListsEveryToolIsGivenTheOnesItMayUseUnderNamesItTakes(t *testing.T) {
	fake, server := reports(t)
	policy := Policy{Rules: []Rule{{Tool: "delete_*", Effect: "deny"}}}
	h := start(t, Config{Servers: []Server{server}, Policy: policy, Mode: All})
	names := namesOf(t, h)
	if strings.Join(names, ",") != "reports__explode,reports__get_report,reports__list_reports,reports__send_email" {
		t.Fatalf("names = %v", names)
	}
	if said, failed := h.call("reports__get_report", map[string]any{"id": "x"}); failed || !strings.Contains(said, "get_report") {
		t.Errorf("%q %v", said, failed)
	}
	if _, failed := h.call("reports__delete_report", nil); !failed {
		t.Error("a tool the policy takes was called")
	}
	if len(fake.calls) != 1 {
		t.Errorf("calls = %v", fake.calls)
	}
}

func TestShowingThreeToolsCostsFarFewerTokensThanShowingAllOfThem(t *testing.T) {
	var many []Tool
	for i := range 60 {
		many = append(many, tool(fmt.Sprintf("operation_%02d", i), "Does one of the things this server does, and says what it did."))
	}
	_, address := newUpstream(t, many...)
	h := start(t, Config{Servers: []Server{{Name: "big", URL: address, Headers: map[string]string{"Authorization": "Bearer s3cret"}}}})
	h.send("tools/list", nil)
	var listed Entry
	for _, entry := range h.entries() {
		if entry.Kind == "list" {
			listed = entry
		}
	}
	if listed.Tools != 60 || listed.TokensShown == 0 || listed.TokensAll < 10*listed.TokensShown {
		t.Errorf("list = %+v: showing the three tools should cost a tenth of showing all sixty", listed)
	}
}

func TestAServerThatAnswersWithAStreamAndLosesItsSessionIsStillCalled(t *testing.T) {
	fake, server := reports(t)
	fake.stream = true
	h := start(t, Config{Servers: []Server{server}})
	if said, failed := h.call("tool_call", map[string]any{"name": "reports/get_report"}); failed || !strings.Contains(said, "get_report") {
		t.Fatalf("%q %v", said, failed)
	}
	fake.mu.Lock()
	fake.gone = true
	fake.mu.Unlock()
	if said, failed := h.call("tool_call", map[string]any{"name": "reports/list_reports"}); failed || !strings.Contains(said, "list_reports") {
		t.Errorf("after the session was lost: %q %v", said, failed)
	}
	if fake.inits != 2 {
		t.Errorf("the session was started %d times, want 2", fake.inits)
	}
}

func TestAServerThatCannotBeReachedIsSaidAndItsCallsAreRecordedAsFailed(t *testing.T) {
	_, up := reports(t)
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close()
	h := start(t, Config{Servers: []Server{{Name: "dead", URL: deadURL}, up}})
	found, _ := h.call("tool_search", map[string]any{"query": "report"})
	if !strings.Contains(found, "reports/get_report") || !strings.Contains(found, "the server dead could not be reached") {
		t.Errorf("search = %q", found)
	}
	if said, failed := h.call("tool_call", map[string]any{"name": "dead/anything"}); !failed || !strings.Contains(said, "no tool") {
		t.Errorf("%q %v", said, failed)
	}
}

func TestAServerOnAPrivateNetworkIsNotReachedUnlessAnAdministratorAllowsIt(t *testing.T) {
	_, server := reports(t)
	config := Config{Servers: []Server{server}, Record: OpenRecord(filepath.Join(t.TempDir(), RecordFile))}
	session := New(config)
	defer session.Close()
	web := httptest.NewServer(session)
	defer web.Close()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"tool_search","arguments":{}}}`
	answer, err := http.Post(web.URL, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer answer.Body.Close()
	var reply struct {
		Result struct {
			Content []struct{ Text string } `json:"content"`
		} `json:"result"`
	}
	json.NewDecoder(answer.Body).Decode(&reply)
	if len(reply.Result.Content) == 0 || !strings.Contains(reply.Result.Content[0].Text, "private network") {
		t.Errorf("the server on loopback was reached: %+v", reply.Result)
	}
}

func TestAServerThatRedirectsIsNotFollowed(t *testing.T) {
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("the redirect was followed") }))
	defer elsewhere.Close()
	redirecting := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL, http.StatusTemporaryRedirect)
	}))
	defer redirecting.Close()
	h := start(t, Config{Servers: []Server{{Name: "r", URL: redirecting.URL}}})
	found, _ := h.call("tool_search", map[string]any{})
	if !strings.Contains(found, "could not be reached") || !strings.Contains(found, "redirect") {
		t.Errorf("search = %q", found)
	}
}

func TestACallIsNotMadeWhenItCannotBeWrittenDown(t *testing.T) {
	fake, server := reports(t)
	h := start(t, Config{Servers: []Server{server}, Record: OpenRecord(filepath.Join(t.TempDir(), "no-such-folder", RecordFile))})
	h.send("tools/list", nil)
	if said, failed := h.call("tool_call", map[string]any{"name": "reports/get_report"}); !failed || !strings.Contains(said, "write down") {
		t.Errorf("%q %v", said, failed)
	}
	if len(fake.calls) != 0 {
		t.Errorf("a call was made that could not be recorded: %v", fake.calls)
	}
}

func TestTheProtocolAtTheDoorIsKept(t *testing.T) {
	_, server := reports(t)
	h := start(t, Config{Servers: []Server{server}})
	if reply := h.send("initialize", map[string]any{"protocolVersion": "2024-11-05"}); reply.Result.(map[string]any)["protocolVersion"] != "2024-11-05" {
		t.Errorf("initialize = %+v", reply)
	}
	if reply := h.send("initialize", map[string]any{"protocolVersion": "1999-01-01"}); reply.Result.(map[string]any)["protocolVersion"] != protocols[0] {
		t.Errorf("an unknown version was echoed: %+v", reply)
	}
	if reply := h.send("no/such", nil); reply.Error == nil || reply.Error.Code != codeMethod {
		t.Errorf("an unknown method: %+v", reply)
	}
	if reply := h.send("tools/call", map[string]any{}); reply.Error == nil || reply.Error.Code != codeParams {
		t.Errorf("a call with no name: %+v", reply)
	}
	post := func(body string, headers map[string]string) *http.Response {
		request, _ := http.NewRequest("POST", h.server.URL, strings.NewReader(body))
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		answer, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		answer.Body.Close()
		return answer
	}
	if got := post(`{"jsonrpc":"2.0","method":"notifications/initialized"}`, nil).StatusCode; got != http.StatusAccepted {
		t.Errorf("a notification answered %d", got)
	}
	if got := post(`{"jsonrpc":"2.0","id":1,"method":"ping"}`, map[string]string{"Origin": "https://evil.example"}).StatusCode; got != http.StatusForbidden {
		t.Errorf("a browser's request answered %d", got)
	}
	if answer, _ := http.Get(h.server.URL); answer.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET answered %d", answer.StatusCode)
	}
	batch, _ := http.Post(h.server.URL, "application/json", strings.NewReader(`[{"jsonrpc":"2.0","id":1,"method":"ping"},{"jsonrpc":"2.0","method":"notifications/initialized"},{"jsonrpc":"2.0","id":2,"method":"ping"}]`))
	var replies []map[string]any
	json.NewDecoder(batch.Body).Decode(&replies)
	if len(replies) != 2 {
		t.Errorf("a batch of two requests and a notification was answered with %d", len(replies))
	}
	if got := post("not json", nil).StatusCode; got != http.StatusOK {
		t.Errorf("a message that is not JSON answered %d, and the error is in the body", got)
	}
	huge := post(strings.Repeat("x", mostRequest+10), nil)
	_ = huge
}

func TestManyCallsAtOnceAreAllAnsweredAndAllRecorded(t *testing.T) {
	_, server := reports(t)
	h := start(t, Config{Servers: []Server{server}})
	var wait sync.WaitGroup
	for i := range 20 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			said, failed := h.call("tool_call", map[string]any{"name": "reports/get_report", "arguments": map[string]any{"id": fmt.Sprint(i)}})
			if failed || !strings.Contains(said, fmt.Sprintf(`"id":"%d"`, i)) {
				t.Errorf("call %d answered %q", i, said)
			}
		}()
	}
	wait.Wait()
	calls := 0
	for _, entry := range h.entries() {
		if entry.Kind == "call" && entry.Outcome == "ok" {
			calls++
		}
	}
	if calls != 20 {
		t.Errorf("%d calls were recorded, want 20", calls)
	}
}

func TestTheToolsTheJobIsShownAreSchemasAClientTakes(t *testing.T) {
	_, server := reports(t)
	h := start(t, Config{Servers: []Server{server}})
	for _, mode := range []Mode{Discover, All} {
		h.session.config.Mode = mode
		listing := h.send("tools/list", nil).Result.(map[string]any)["tools"].([]any)
		for _, one := range listing {
			tool := one.(map[string]any)
			schema, ok := tool["inputSchema"].(map[string]any)
			if !ok || schema["type"] != "object" {
				t.Errorf("%s: inputSchema = %v", tool["name"], tool["inputSchema"])
				continue
			}
			if required, there := schema["required"]; there {
				if _, isList := required.([]any); !isList {
					t.Errorf("%s: required is %v, which a client refuses unless it is a list", tool["name"], required)
				}
			}
		}
	}
}

func TestAChangeToThePolicyReachesAJobThatIsAlreadyRunning(t *testing.T) {
	fake, server := reports(t)
	var mu sync.Mutex
	current := Policy{}
	h := start(t, Config{Servers: []Server{server}, Live: func() Policy { mu.Lock(); defer mu.Unlock(); return current }})
	if _, failed := h.call("tool_call", map[string]any{"name": "reports/send_email"}); failed {
		t.Fatal("the tool was refused before the policy changed")
	}
	mu.Lock()
	current = Policy{Rules: []Rule{{Tool: "send_*", Effect: "deny", Why: "email is off since the incident"}}}
	mu.Unlock()
	said, failed := h.call("tool_call", map[string]any{"name": "reports/send_email"})
	if !failed || said != "email is off since the incident" {
		t.Errorf("after the change: %q %v", said, failed)
	}
	if found, _ := h.call("tool_search", map[string]any{"query": "email"}); strings.Contains(found, "send_email") {
		t.Errorf("a tool the policy now takes is still found: %q", found)
	}
	if len(fake.calls) != 1 {
		t.Errorf("calls = %v", fake.calls)
	}
	mu.Lock()
	current = Policy{}
	mu.Unlock()
	if _, failed := h.call("tool_call", map[string]any{"name": "reports/send_email"}); failed {
		t.Error("the tool stayed refused after the policy was loosened")
	}
}

func TestWhatAToolAnswersIsMaskedBeforeTheJobSeesItAndCountedInTheRecord(t *testing.T) {
	_, server := reports(t)
	redactor, err := redact.New("email", "card")
	if err != nil {
		t.Fatal(err)
	}
	h := start(t, Config{Servers: []Server{server}, Redactor: func() *redact.Redactor { return redactor }})
	said, failed := h.call("tool_call", map[string]any{"name": "reports/get_report", "arguments": map[string]any{"owner": "ada@example.com", "card": "4111 1111 1111 1111"}})
	if failed || strings.Contains(said, "ada@example.com") || strings.Contains(said, "4111") || !strings.Contains(said, "[redacted:email]") || !strings.Contains(said, "[redacted:card]") {
		t.Errorf("the job was given %q", said)
	}
	var call Entry
	for _, entry := range h.entries() {
		if entry.Kind == "call" {
			call = entry
		}
	}
	if call.Redacted["email"] != 1 || call.Redacted["card"] != 1 {
		t.Errorf("the record says %+v", call)
	}
	data, _ := os.ReadFile(h.record)
	if strings.Contains(string(data), "ada@example.com") {
		t.Error("the record keeps what was masked")
	}
}

func TestWithNoRedactorWhatAToolAnswersIsGivenAsItIs(t *testing.T) {
	_, server := reports(t)
	h := start(t, Config{Servers: []Server{server}})
	said, _ := h.call("tool_call", map[string]any{"name": "reports/get_report", "arguments": map[string]any{"owner": "ada@example.com"}})
	if !strings.Contains(said, "ada@example.com") {
		t.Errorf("the job was given %q", said)
	}
}
