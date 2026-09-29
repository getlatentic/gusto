package openshell_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/openshell"
	"github.com/getlatentic/haas/internal/runner"
	"github.com/getlatentic/haas/internal/signing"
)

// A harness that makes the change, and says what it was started with.
const fixture = `{
  "name": "fixture",
  "program": "sh",
  "argv": ["-c", "printf yes > answer.txt; echo model=$HAAS_MODEL_URL token=$HAAS_MODEL_TOKEN; echo password=${HAAS_PASSWORD:-absent} signer=${HAAS_SIGNER:-absent}; echo wrote the answer"],
  "needs": "nothing",
  "stream": "plain"
}`

func init() {
	openshell.SetSettle(time.Millisecond, 3)
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// aJob is a job dir whose workspace already holds a repository whose gate fails
// until answer.txt says yes, as the server leaves it after the checkout.
func aJob(t *testing.T) (job.Dir, job.Spec) {
	t.Helper()
	harnesses := t.TempDir()
	write(t, filepath.Join(harnesses, "fixture.json"), fixture)
	t.Setenv("HAAS_HARNESSES", harnesses)
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir.Workspace(), "answer.txt"), "no")
	for _, args := range [][]string{{"init", "--quiet"}, {"add", "answer.txt"}, {"-c", "user.email=t@t", "-c", "user.name=t", "commit", "--quiet", "-m", "start"}} {
		command := exec.Command("git", append([]string{"-C", dir.Workspace()}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	spec := job.Spec{Brief: "make the answer yes", Repo: "unused", Gate: []string{"sh", "-c", `test "$(cat answer.txt)" = yes`}, Harness: "fixture"}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	return dir, spec
}

func sandboxFor(t *testing.T, dir job.Dir) *openshell.Sandbox {
	t.Helper()
	configured, err := harness.Open(dir, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	box, err := openshell.New(openshell.Config{Binary: "openshell", Image: "runner.tar", CPU: "1", Memory: "1Gi", Allowed: []string{"registry.npmjs.org"}},
		"job-one", configured.Adapter, "http://host.openshell.internal:8041/model/v1", "http://127.0.0.1:8041/model/v1", "a-job-token")
	if err != nil {
		t.Fatal(err)
	}
	return box
}

// The runner stays outside: it writes the record and signs it, the harness runs
// in the sandbox with an environment of its own, and what the sandbox's guard
// saw is in the record the signature covers.
func TestAJobRunInASandboxLeavesARecordTheHostWroteAndSigned(t *testing.T) {
	root := installFake(t)
	t.Setenv("HAAS_PASSWORD", "the-servers-own-secret")
	t.Setenv("HAAS_SIGNER", "the-servers-signer")
	keys := t.TempDir()
	key, err := signing.Generate(keys, "acme")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(signing.KeyEnv, filepath.Join(keys, "acme.key"))
	ring, err := signing.OpenKeyring(keys)
	if err != nil {
		t.Fatal(err)
	}
	dir, _ := aJob(t)

	exit, err := runner.Outside(dir, sandboxFor(t, dir))
	if err != nil || exit != 0 {
		t.Fatalf("exit %d, %v: %s", exit, err, runner.Tail(dir.OutFile("stderr.log"), 5))
	}

	// The harness saw the proxy and its token, and none of the server's own environment.
	said, _ := os.ReadFile(dir.OutFile("transcript.log"))
	for _, want := range []string{"model=http://host.openshell.internal:8041/model/v1", "token=a-job-token", "password=absent", "signer=absent"} {
		if !strings.Contains(string(said), want) {
			t.Errorf("the harness's output lacks %q:\n%s", want, said)
		}
	}
	if strings.Contains(string(said), "the-servers-own-secret") {
		t.Fatal("the server's secret reached the sandbox")
	}

	// The change came back, and the gate ran in the sandbox and passed.
	if patch, _ := os.ReadFile(dir.OutFile("diff.patch")); !strings.Contains(string(patch), "+yes") {
		t.Errorf("the patch does not say what changed:\n%s", patch)
	}
	if verdict, _ := os.ReadFile(dir.OutFile("gates.json")); !strings.Contains(string(verdict), `"passed": true`) {
		t.Errorf("the gate did not pass in the sandbox: %s", verdict)
	}

	// What the guard saw and enforced is in the record, and the record verifies and is signed.
	events, _ := os.ReadFile(dir.OutFile(runner.GuardEvents))
	if !strings.Contains(string(events), "net.deny") || !strings.Contains(string(events), "net.allow") {
		t.Errorf("the sandbox's events are not in the record: %s", events)
	}
	policy, _ := os.ReadFile(dir.OutFile(runner.GuardPolicy))
	if !strings.Contains(string(policy), "registry.npmjs.org") || !strings.Contains(string(policy), "host.openshell.internal") {
		t.Errorf("the policy is not in the record: %s", policy)
	}
	if wrong, err := evidence.Check(dir); err != nil || len(wrong) > 0 {
		t.Fatalf("the record does not verify: %v %v", wrong, err)
	}
	if signed, err := evidence.Attest(dir, ring); err != nil || signed.Key != key.ID() {
		t.Fatalf("the record is not signed by the host's key: %+v %v", signed, err)
	}
	manifest, _ := evidence.Read(dir)
	listed := map[string]bool{}
	for _, file := range manifest.Files {
		listed[file.Name] = true
	}
	if !listed[runner.GuardEvents] || !listed[runner.GuardPolicy] {
		t.Errorf("the guard's files are not among the ones the manifest hashes: %v", manifest.Files)
	}

	// The sandbox is gone when the run is.
	if deleted, _ := os.ReadFile(filepath.Join(root, "deleted.log")); !strings.Contains(string(deleted), openshell.Name("job-one")) {
		t.Errorf("the sandbox was left behind: %q", deleted)
	}
}

func TestATamperedGuardLogFailsVerification(t *testing.T) {
	installFake(t)
	dir, _ := aJob(t)
	if exit, err := runner.Outside(dir, sandboxFor(t, dir)); err != nil || exit != 0 {
		t.Fatalf("exit %d, %v", exit, err)
	}
	write(t, dir.OutFile(runner.GuardEvents), "nothing was denied\n")
	if wrong, _ := evidence.Check(dir); !strings.Contains(strings.Join(wrong, ";"), runner.GuardEvents+" changed") {
		t.Errorf("an edited guard log was not caught: %v", wrong)
	}
}

func TestWhatASandboxCannotYetCarryIsRefusedByNameBeforeAnythingStarts(t *testing.T) {
	root := installFake(t)
	dir, spec := aJob(t)
	spec.Setup = &job.Setup{Name: "engineering", Servers: []string{"memory"}}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := runner.Outside(dir, sandboxFor(t, dir)); err == nil || !strings.Contains(err.Error(), "does not yet carry") {
		t.Fatalf("a job with skills was not refused: %v", err)
	}
	if calls, _ := os.ReadFile(filepath.Join(root, "calls.log")); len(calls) != 0 {
		t.Errorf("a sandbox was made for a refused job: %s", calls)
	}
}

func TestADriverSubprocessCarriesNoneOfTheServersSecrets(t *testing.T) {
	installFake(t)
	t.Setenv("HAAS_PASSWORD", "the-servers-own-secret")
	t.Setenv("HAAS_MODEL_KEY", "a-real-key")
	t.Setenv("OPENSHELL_GATEWAY", "kept")
	dir, _ := aJob(t)
	if _, err := runner.Outside(dir, sandboxFor(t, dir)); err != nil {
		t.Fatal(err)
	}
	// The fake records its arguments, and the environment is what it was started with.
	calls, _ := os.ReadFile(filepath.Join(os.Getenv("OPENSHELL_FAKE_ROOT"), "calls.log"))
	if strings.Contains(string(calls), "the-servers-own-secret") || strings.Contains(string(calls), "a-real-key") {
		t.Fatalf("a secret was passed to openshell: %s", calls)
	}
}

func TestASandboxNameFitsWhatAGatewayTakesAndIsTheSameEachTime(t *testing.T) {
	long := "20260929-1245-make-the-answer-yes-and-then-some-more-words-in-the-brief"
	if got := openshell.Name(long); len(got) > 19 || got != openshell.Name(long) || got == openshell.Name(long+"x") {
		t.Errorf("%q", got)
	}
}

func TestTheCatalogAsksASandboxWhichProgramsTheImageCarriesAndRemovesIt(t *testing.T) {
	root := installFake(t)
	carried := openshell.Image(openshell.Config{Binary: "openshell", Image: "runner.tar"}).Carried([]string{"sh", "a-program-no-image-carries"})
	if carried.Unanswered != nil || !carried.OnPath["sh"] || carried.OnPath["a-program-no-image-carries"] {
		t.Fatalf("%+v", carried)
	}
	if deleted, _ := os.ReadFile(filepath.Join(root, "deleted.log")); len(strings.Fields(string(deleted))) != 1 {
		t.Errorf("the sandbox made to be asked was left behind: %q", deleted)
	}
	calls, _ := os.ReadFile(filepath.Join(root, "calls.log"))
	if !strings.Contains(string(calls), "--policy") {
		t.Errorf("the survey sandbox was made without a policy: %s", calls)
	}
}

// A job's skills are put where the harness finds them, under the sandbox's home
// and not under the runner's.
func TestASkillIsPlacedInTheSandboxsHomeNotTheRunners(t *testing.T) {
	root := installFake(t)
	dir, spec := aJob(t)
	harnesses := t.TempDir()
	write(t, filepath.Join(harnesses, "fixture.json"), `{
  "name": "fixture", "program": "sh", "skills": "/root/.config/fixture/skills",
  "argv": ["-c", "printf yes > answer.txt; cat .config/fixture/skills/review/SKILL.md 2>/dev/null | head -1 > /dev/null; echo skills: $(ls ../.config/fixture/skills 2>&1 | head -1)"],
  "needs": "nothing", "stream": "plain"}`)
	t.Setenv("HAAS_HARNESSES", harnesses)
	if err := os.MkdirAll(filepath.Join(dir.SetupDir(), "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir.SetupDir(), "skills", "review", "SKILL.md"), "review the change\n")
	spec.Setup = &job.Setup{Name: "engineering", Skills: []string{"review"}}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	if exit, err := runner.Outside(dir, sandboxFor(t, dir)); err != nil || exit != 0 {
		t.Fatalf("exit %d, %v", exit, err)
	}
	// The fake keeps a sandbox as a directory, and it is gone after the run, so
	// the run itself is what shows it: the harness listed the skills folder.
	said, _ := os.ReadFile(dir.OutFile("transcript.log"))
	if !strings.Contains(string(said), "skills: review") {
		t.Errorf("the skill was not where the harness looks:\n%s", said)
	}
	calls, _ := os.ReadFile(filepath.Join(root, "calls.log"))
	if !strings.Contains(string(calls), "/sandbox/.config/fixture/skills") || strings.Contains(string(calls), "/root/") {
		t.Errorf("the skill went to the runner's home, or nowhere: %s", calls)
	}
}

// An adapter is written for a container, where the harness's home is /root, so
// every mention of it in what the harness is told is rewritten for the sandbox.
func TestTheRunnersHomeIsRewrittenForTheSandboxInWhatTheHarnessIsTold(t *testing.T) {
	installFake(t)
	for from, want := range map[string]string{
		"/root": "/sandbox", "/root/.local/share/codex": "/sandbox/.local/share/codex",
		`mkdir -p /root/.a && cp /root/.config/x /root/.b`: `mkdir -p /sandbox/.a && cp /sandbox/.config/x /sandbox/.b`,
		"/rootless/thing": "/rootless/thing", "/usr/root/x": "/usr/root/x",
	} {
		if got := openshell.InSandbox(from); got != want {
			t.Errorf("%q became %q, want %q", from, got, want)
		}
	}
}

// A program the image does not carry is refused by name once the sandbox is up,
// and the sandbox is removed, rather than the harness failing inside it.
func TestAHarnessTheImageDoesNotCarryIsRefusedByNameAndTheSandboxRemoved(t *testing.T) {
	root := installFake(t)
	dir, _ := aJob(t)
	harnesses := t.TempDir()
	write(t, filepath.Join(harnesses, "fixture.json"), `{"name":"fixture","program":"a-program-no-image-carries","argv":[],"needs":"nothing","stream":"plain"}`)
	t.Setenv("HAAS_HARNESSES", harnesses)
	_, err := runner.Outside(dir, sandboxFor(t, dir))
	if err == nil || !strings.Contains(err.Error(), "does not carry a-program-no-image-carries") {
		t.Fatalf("%v", err)
	}
	if deleted, _ := os.ReadFile(filepath.Join(root, "deleted.log")); !strings.Contains(string(deleted), openshell.Name("job-one")) {
		t.Errorf("the sandbox was left behind: %q", deleted)
	}
	if said, _ := os.ReadFile(dir.OutFile("transcript.log")); !strings.Contains(string(said), "refused this job before the harness started") {
		t.Errorf("the refusal is not where a person looks: %s", said)
	}
}
