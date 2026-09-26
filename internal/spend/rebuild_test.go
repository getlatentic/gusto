package spend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/getlatentic/haas/internal/job"
)

func jobWithCalls(t *testing.T, root, id string, spec job.Spec, calls ...Call) job.Dir {
	t.Helper()
	dir := job.Dir(filepath.Join(root, id))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	var text []byte
	for _, call := range calls {
		line, err := json.Marshal(call)
		if err != nil {
			t.Fatal(err)
		}
		text = append(append(text, line...), '\n')
	}
	if err := os.WriteFile(dir.OutFile("proxy.jsonl"), text, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// A server that restarts in the middle of a month still refuses a team that
// spent its cap before the restart.
func TestARestartedServerRemembersWhatATeamSpent(t *testing.T) {
	root := t.TempDir()
	spec := job.Spec{Brief: "b", Repo: "r", Gate: []string{"true"}, Harness: "h", Team: "platform"}
	jobWithCalls(t, root, "earlier", spec, answered(0.7, september))
	jobWithCalls(t, root, "later", spec, answered(0.4, september))

	ledger, err := Rebuild(root, fixturePolicy())
	if err != nil {
		t.Fatal(err)
	}
	ledger.now = func() time.Time { return september }
	if refusal := ledger.Allow("later", "fixture-model"); refusal == nil || refusal.Cap != "team" {
		t.Fatalf("refusal %+v", refusal)
	}
}

func TestAHalfWrittenLineIsPassedOver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "proxy.jsonl")
	os.WriteFile(path, []byte(`{"path":"/v1/models","status":200,"atUnix":1}`+"\n"+`{"path":"/v1/chat`), 0o644)
	calls, err := ReadCalls(path)
	if err != nil || len(calls) != 1 {
		t.Fatalf("calls %+v, %v", calls, err)
	}
}

// The record outlives a run, so a refusal from an earlier run of the same
// directory must not be read as this run's.
func TestTheCapThatStoppedARunIsTheFirstRefusalSinceItStarted(t *testing.T) {
	root := t.TempDir()
	spec := job.Spec{Brief: "b", Repo: "r", Gate: []string{"true"}, Harness: "h"}
	old := RefusedCall("/v1/chat/completions", "m", september.Unix()-60, &Refusal{Cap: "team", Reason: "then"})
	first := RefusedCall("/v1/chat/completions", "m", september.Unix()+5, &Refusal{Cap: "job", Reason: "now"})
	again := RefusedCall("/v1/chat/completions", "m", september.Unix()+6, &Refusal{Cap: "team", Reason: "later"})
	dir := jobWithCalls(t, root, "a-job", spec, old, answered(1, september), first, again)

	stop, err := Stopped(dir.OutFile("proxy.jsonl"), september.Unix())
	if err != nil || stop == nil || stop.Reason != "now" {
		t.Fatalf("stop %+v, %v", stop, err)
	}
	if none, err := Stopped(dir.OutFile("proxy.jsonl"), september.Unix()+60); err != nil || none != nil {
		t.Fatalf("a run after every refusal was stopped by %+v, %v", none, err)
	}
}
