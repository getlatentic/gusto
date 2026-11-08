package spend

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func record(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "proxy.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A run whose every call to a model failed was never answered, and the reason is the proxy's own for the last call.
func TestARunWhoseModelCallsAllFailedWasNeverAnswered(t *testing.T) {
	path := record(t,
		`{"path":"/v1/models","error":"dial tcp: connect: connection refused","atUnix":100,"inputTokens":null,"outputTokens":null,"usd":null}`,
		`{"path":"/v1/chat/completions","status":503,"atUnix":101,"inputTokens":null,"outputTokens":null,"usd":null}`,
		`{"path":"","host":"example.com:443","denied":"not allowed","atUnix":102,"inputTokens":null,"outputTokens":null,"usd":null}`)
	silence, err := Unanswered(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	if silence == nil || silence.Calls != 2 || !strings.Contains(silence.Reason, "503") {
		t.Errorf("the run's silence is %+v", silence)
	}
}

// One answered call is an answer; a run that asked nothing, an earlier run's calls and a cap's refusals are not silence.
func TestARunWithAnAnswerOrNoCallsWasNotUnanswered(t *testing.T) {
	for name, lines := range map[string][]string{
		"answered once": {
			`{"path":"/v1/chat/completions","status":500,"atUnix":100,"inputTokens":null,"outputTokens":null,"usd":null}`,
			`{"path":"/v1/chat/completions","status":200,"atUnix":101,"inputTokens":1,"outputTokens":1,"usd":0}`},
		"asked nothing":  {`{"path":"","host":"example.com:443","atUnix":100,"inputTokens":null,"outputTokens":null,"usd":null}`},
		"an earlier run": {`{"path":"/v1/chat/completions","status":500,"atUnix":50,"inputTokens":null,"outputTokens":null,"usd":null}`},
		"a cap refused it": {`{"path":"/v1/chat/completions","status":402,"atUnix":100,"inputTokens":0,"outputTokens":0,"usd":0,` +
			`"refused":{"cap":"job","limitUsd":1,"spentUsd":1,"reason":"spent"}}`},
	} {
		silence, err := Unanswered(record(t, lines...), 100)
		if err != nil || silence != nil {
			t.Errorf("%s: the run's silence is %+v, %v", name, silence, err)
		}
	}
}
