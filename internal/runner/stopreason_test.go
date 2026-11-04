package runner

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/job"
)

// agentThatStops is an ACP agent, as a shell script, that answers its prompt with the stop reason given.
func agentThatStops(reason string) *exec.Cmd {
	script := `while IFS= read -r line; do
  id=$(printf '%s' "$line" | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
  case "$line" in
    *'"initialize"'*) echo '{"jsonrpc":"2.0","id":'"$id"',"result":{"protocolVersion":1,"agentCapabilities":{},"authMethods":[]}}' ;;
    *'"session/new"'*) echo '{"jsonrpc":"2.0","id":'"$id"',"result":{"sessionId":"s1"}}' ;;
    *'"session/prompt"'*) echo '{"jsonrpc":"2.0","id":'"$id"',"result":{"stopReason":"` + reason + `"}}' ;;
  esac
done`
	return exec.Command("sh", "-c", script)
}

func TestATurnTheAgentEndedWithoutAnsweringIsNotAnAnswer(t *testing.T) {
	for reason, want := range map[string]int{"end_turn": 0, "refusal": 1, "cancelled": 1, "max_tokens": 1} {
		dir := job.Dir(t.TempDir())
		if err := dir.Make(); err != nil {
			t.Fatal(err)
		}
		adapter, err := harness.Load("", "dsh")
		if err != nil {
			t.Fatal(err)
		}
		record, err := Open(dir, adapter)
		if err != nil {
			t.Fatal(err)
		}
		exit, err := converse(context.Background(), agentThatStops(reason), t.TempDir(), job.Spec{Brief: "go"}, nil, "", record)
		record.Close()
		if err != nil || exit != want {
			t.Errorf("a turn the agent ended with %s exited %d (%v), want %d", reason, exit, err, want)
		}
		said, _ := os.ReadFile(dir.OutFile("stderr.log"))
		if want == 1 && !strings.Contains(string(said), reason) {
			t.Errorf("the record does not say the agent ended the turn with %s: %q", reason, said)
		}
	}
}
