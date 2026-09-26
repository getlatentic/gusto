package runner_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/runner"
	"github.com/getlatentic/haas/schema"
)

// cappedAdapter stands in for a harness whose model calls a cap refused. The
// proxy runs outside the box and writes into the job directory the box is
// given, so the harness appending the proxy's refusal to out/proxy.jsonl is the
// same file arriving by the same route.
const cappedAdapter = `{
  "name": "capped",
  "program": "sh",
  "argv": ["-c", "printf '{\"path\":\"/v1/chat/completions\",\"status\":402,\"atUnix\":%s,\"inputTokens\":0,\"outputTokens\":0,\"usd\":0,\"refused\":{\"cap\":\"team\",\"team\":\"platform\",\"limitUsd\":1,\"spentUsd\":1.25,\"reason\":\"team platform has spent $1.2500 of its $1.0000 cap for this month\"}}\\n' $(date +%s) >> ../out/proxy.jsonl"],
  "needs": "nothing",
  "stream": "plain"
}`

func cappedRun(t *testing.T, earlier string) job.Dir {
	t.Helper()
	harnesses := t.TempDir()
	write(t, filepath.Join(harnesses, "capped.json"), cappedAdapter)
	t.Setenv("HAAS_HARNESSES", harnesses)
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	write(t, dir.OutFile("proxy.jsonl"), earlier)
	spec := job.Spec{Brief: "b", Repo: repo(t), Gate: []string{"true"}, Harness: "capped", Team: "platform"}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Inside(dir); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestTheManifestSaysWhichCapStoppedTheRun(t *testing.T) {
	dir := cappedRun(t, "")
	manifest, err := evidence.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	stop := manifest.StoppedByCap
	if stop == nil || stop.Cap != "team" || stop.Team != "platform" || stop.SpentUSD != 1.25 {
		t.Fatalf("stoppedByCap is %+v", stop)
	}
	written, err := os.ReadFile(dir.OutFile("manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(schema.Manifest(), written); err != nil {
		t.Fatalf("the manifest does not match its schema: %v\n%s", err, written)
	}
	if wrong, err := evidence.Check(dir); err != nil || len(wrong) != 0 {
		t.Fatalf("the proxy's record should be hashed with the rest: %v %v", wrong, err)
	}
}

// A directory run again keeps its proxy record, and a cap that stopped the
// first run did not stop this one.
func TestARefusalFromAnEarlierRunIsNotThisRunsStop(t *testing.T) {
	earlier := `{"path":"/v1/chat/completions","status":402,"atUnix":1,"refused":{"cap":"job","limitUsd":1,"spentUsd":1,"reason":"then"}}` + "\n"
	manifest, err := evidence.Read(cappedRun(t, earlier))
	if err != nil {
		t.Fatal(err)
	}
	if stop := manifest.StoppedByCap; stop == nil || stop.Cap != "team" {
		t.Fatalf("stoppedByCap is %+v", stop)
	}
}

func TestARunNoCapRefusedSaysNothingAboutCaps(t *testing.T) {
	dir, _ := ran(t)
	manifest, err := evidence.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.StoppedByCap != nil {
		t.Fatalf("stoppedByCap is %+v", manifest.StoppedByCap)
	}
}
