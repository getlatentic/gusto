//go:build realharnesses

// The test here runs the real Claude Code and Codex programs through the proxy to a model that answers Chat
// Completions alone, which is the only proof a translation is right: a harness's own parser is the judge. It runs where
// both are installed:
//
//	go test -tags realharnesses -run RealHarnesses ./internal/proxy
package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/spend"
)

// toolModel answers Chat Completions the way a model that uses tools does: asked with a shell tool, it calls it to run
// `echo hi`, then `echo bye`, and given both results it says it is done. It numbers the calls of every answer from
// call_0, as Bedrock does, which a harness must not be given twice.
type toolModel struct {
	mu      sync.Mutex
	results []string
}

func (m *toolModel) serve(w http.ResponseWriter, r *http.Request) {
	var asked struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
		Tools []struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		} `json:"tools"`
		Stream bool `json:"stream"`
	}
	if !strings.HasSuffix(r.URL.Path, "/chat/completions") || json.NewDecoder(r.Body).Decode(&asked) != nil {
		http.Error(w, "this model answers Chat Completions only", http.StatusNotFound)
		return
	}
	answered := 0
	for _, message := range asked.Messages {
		if message.Role == "tool" {
			answered++
			m.mu.Lock()
			m.results = append(m.results, string(message.Content))
			m.mu.Unlock()
		}
	}
	shell := ""
	for _, tool := range asked.Tools {
		if tool.Function.Name == "Bash" || tool.Function.Name == "exec_command" {
			shell = tool.Function.Name
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	chunk := func(data string) { fmt.Fprintf(w, "data: %s\n\n", data) }
	switch {
	case answered < 2 && shell != "":
		word := []string{"hi", "bye"}[answered]
		arguments := fmt.Sprintf(`{\"command\":\"echo %s\",\"description\":\"Say %s\"}`, word, word)
		if shell == "exec_command" {
			arguments = fmt.Sprintf(`{\"cmd\":\"echo %s\"}`, word)
		}
		chunk(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_0","type":"function","function":{"name":"` + shell + `","arguments":"` + arguments + `"}}]}}]}`)
		chunk(`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`)
	default:
		chunk(`{"choices":[{"delta":{"content":"done"}}]}`)
		chunk(`{"choices":[{"delta":{},"finish_reason":"stop"}]}`)
	}
	chunk(`{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":2}}`)
	chunk(`[DONE]`)
}

func (m *toolModel) sawResult(text string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, result := range m.results {
		if strings.Contains(result, text) {
			return true
		}
	}
	return false
}

func TestRealHarnessesFinishATurnWithATool(t *testing.T) {
	for _, program := range []string{"claude", "codex"} {
		if _, err := exec.LookPath(program); err != nil {
			t.Skipf("%s is not installed here", program)
		}
	}
	for _, harness := range []string{"claude", "codex"} {
		t.Run(harness, func(t *testing.T) {
			model := &toolModel{}
			provider := httptest.NewServer(http.HandlerFunc(model.serve))
			t.Cleanup(provider.Close)
			tokens := NewTokens()
			kept := t.TempDir()
			ledger := spend.NewLedger(spend.Policy{Prices: map[string]spend.Price{"a-model": {InputPerMillion: 1, OutputPerMillion: 1}}})
			ledger.Begin("a-job", spend.Terms{})
			proxy := New(tokens, Credentials{"gemma": {Name: "gemma", Address: provider.URL, Key: "the-real-key", Wires: []string{"chat"}}},
				func(job string) (string, bool) { return filepath.Join(kept, job+".jsonl"), true }, ledger, Allowlist{})
			front := httptest.NewServer(proxy.Handler())
			t.Cleanup(front.Close)
			token, err := tokens.Mint("a-job", "gemma")
			if err != nil {
				t.Fatal(err)
			}
			said := run(t, harness, front.URL, token)
			for _, word := range []string{"hi", "bye"} {
				if !model.sawResult(word) {
					t.Errorf("%s never sent back what `echo %s` printed:\n%s", harness, word, said)
				}
			}
			if !strings.Contains(said, "done") {
				t.Errorf("%s did not finish its turn:\n%s", harness, said)
			}
			recorded, err := spend.ReadCalls(filepath.Join(kept, "a-job.jsonl"))
			if err != nil || len(recorded) < 2 {
				t.Fatalf("%v %d", err, len(recorded))
			}
			for _, call := range recorded {
				if call.Translated == "" || call.InputTokens == nil || call.Status != 200 {
					t.Errorf("a call was recorded as %+v", call)
				}
			}
		})
	}
}

// run starts a harness the way the runner does, pointed at the proxy, in a folder and a home of its own, and gives
// what it printed.
func run(t *testing.T, harness, proxy, token string) string {
	t.Helper()
	work, home := t.TempDir(), t.TempDir()
	var command *exec.Cmd
	switch harness {
	case "claude":
		command = exec.Command("claude", "-p", "Run echo hi, then echo bye, with Bash", "--output-format", "stream-json", "--verbose",
			"--permission-mode", "acceptEdits", "--allowedTools", "Bash", "--model=a-model")
		command.Env = append(os.Environ(), "ANTHROPIC_BASE_URL="+proxy+"/model", "ANTHROPIC_API_KEY="+token, "CLAUDE_CONFIG_DIR="+home,
			"DISABLE_TELEMETRY=1", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1")
	case "codex":
		command = exec.Command("codex", "exec", "--json", "--skip-git-repo-check", "-s", "read-only",
			"-c", "model_providers.gusto.name=gusto", "-c", "model_providers.gusto.base_url="+proxy+"/model/v1",
			"-c", "model_providers.gusto.env_key=GUSTO_MODEL_TOKEN", "-c", "model_providers.gusto.wire_api=responses",
			"-c", "model_provider=gusto", "--model=a-model", "Run echo hi, then echo bye, in the shell")
		command.Env = append(os.Environ(), "GUSTO_MODEL_TOKEN="+token, "CODEX_HOME="+home)
	}
	command.Dir = work
	command.Stdin = strings.NewReader("")
	output := &lockedBuffer{}
	command.Stdout, command.Stderr = output, output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	go func() { finished <- command.Wait() }()
	select {
	case err := <-finished:
		if err != nil {
			t.Errorf("%s: %v", harness, err)
		}
	case <-time.After(3 * time.Minute):
		command.Process.Kill()
		t.Errorf("%s did not finish in three minutes", harness)
	}
	return output.String()
}

type lockedBuffer struct {
	mu   sync.Mutex
	text strings.Builder
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.text.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.text.String()
}

var _ io.Writer = (*lockedBuffer)(nil)
