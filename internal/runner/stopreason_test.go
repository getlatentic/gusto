package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/task"
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
		dir := task.Dir(t.TempDir())
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
		exit, err := converse(context.Background(), agentThatStops(reason), t.TempDir(), task.Spec{Brief: "go"}, nil, "", record)
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

// agentThatWorksUntilCancelled is an ACP agent, as a shell script, that never answers its prompt on its own: told to cancel,
// it writes down that it was, and ends the prompt as cancelled.
func agentThatWorksUntilCancelled(ctx context.Context, heard string) *exec.Cmd {
	script := `while IFS= read -r line; do
  id=$(printf '%s' "$line" | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
  case "$line" in
    *'"initialize"'*) echo '{"jsonrpc":"2.0","id":'"$id"',"result":{"protocolVersion":1,"agentCapabilities":{},"authMethods":[]}}' ;;
    *'"session/new"'*) echo '{"jsonrpc":"2.0","id":'"$id"',"result":{"sessionId":"s1"}}' ;;
    *'"session/prompt"'*) prompt=$id ;;
    *'"session/cancel"'*) echo cancelled > "$HEARD"; echo '{"jsonrpc":"2.0","id":'"$prompt"',"result":{"stopReason":"cancelled"}}' ;;
  esac
done`
	command := exec.CommandContext(ctx, "sh", "-c", script)
	command.Env = append(os.Environ(), "HEARD="+heard)
	return command
}

func TestAStoppedTurnIsCancelledByTheAgentAndKeepsItsSession(t *testing.T) {
	dir := task.Dir(t.TempDir())
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
	defer record.Close()
	ctx, stop := context.WithCancel(context.Background())
	life, end := outliving(ctx, time.Minute)
	defer end()
	heard := filepath.Join(t.TempDir(), "heard")
	go func() {
		time.Sleep(300 * time.Millisecond)
		stop()
	}()
	began := time.Now()
	exit, err := converse(ctx, agentThatWorksUntilCancelled(life, heard), t.TempDir(), task.Spec{Brief: "go"}, nil, "", record)
	if err != nil || exit != 1 {
		t.Errorf("a stopped turn exited %d (%v), want 1", exit, err)
	}
	if said, _ := os.ReadFile(heard); strings.TrimSpace(string(said)) != "cancelled" {
		t.Error("the agent was not asked to cancel the turn")
	}
	if took := time.Since(began); took > 20*time.Second {
		t.Errorf("the turn took %s to end: the agent was waited out, not cancelled", took)
	}
	if record.Remembered != "s1" {
		t.Errorf("the session after a cancelled turn is %q, want s1, so the next turn carries on from it", record.Remembered)
	}
	if life.Err() != nil {
		t.Error("the agent's process was killed when it was only asked to cancel")
	}
}

func TestAnAgentThatOutlivesItsGraceIsKilled(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	life, end := outliving(ctx, 100*time.Millisecond)
	defer end()
	stop()
	select {
	case <-life.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the agent's context did not end after its grace")
	}
}
