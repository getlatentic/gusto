package ship_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/getlatentic/gusto/internal/ship"
)

// These tests read a pull request the way a reviewer does: they take the
// command out of the body that reached the forge and run it as printed, with
// the gusto this repository builds, against the remote the job shipped to.

func TestThePrintedCommandVerifiesTheEvidence(t *testing.T) {
	dir, _ := finished(t, "yes")
	fake := &forge{}
	shipped(t, dir, fake)
	said, code := reviewed(t, printedCommand(t, fake))
	if code != 0 || !strings.Contains(said, "files match, and the check still exits 0") {
		t.Fatalf("the printed command should verify a good job, exit %d:\n%s", code, said)
	}
}

func TestThePrintedCommandNamesAFileChangedOnTheRemote(t *testing.T) {
	dir, bare := finished(t, "yes")
	fake := &forge{}
	plan, _ := shipped(t, dir, fake)
	forged(t, bare, plan, "", map[string]string{"out/transcript.log": "the harness said something else\n"})
	said, code := reviewed(t, printedCommand(t, fake))
	if code != 1 || !strings.Contains(said, "transcript.log changed") {
		t.Fatalf("want exit 1 naming transcript.log, got exit %d:\n%s", code, said)
	}
}

// Whoever can push can move the ref, manifest and all. The command carries the
// manifest's hash, so a manifest rewritten to match its files is still caught.
func TestThePrintedCommandHoldsTheManifestToTheHashInThePullRequest(t *testing.T) {
	dir, bare := finished(t, "yes")
	fake := &forge{}
	plan, _ := shipped(t, dir, fake)
	forged(t, bare, plan, "", map[string]string{"out/manifest.json": `{"version": 1, "files": []}` + "\n"})
	said, code := reviewed(t, printedCommand(t, fake))
	if code != 1 || !strings.Contains(said, "manifest.json hashes to") {
		t.Fatalf("want exit 1 naming manifest.json, got exit %d:\n%s", code, said)
	}
}

func TestThePrintedCommandRefusesAPullRequestCommitThatIsNotThePatch(t *testing.T) {
	dir, bare := finished(t, "yes")
	fake := &forge{}
	plan, _ := shipped(t, dir, fake)
	forged(t, bare, plan, plan.At, nil)
	said, code := reviewed(t, printedCommand(t, fake))
	if code != 1 || !strings.Contains(said, "is not diff.patch on") {
		t.Fatalf("want exit 1 naming the commit, got exit %d:\n%s", code, said)
	}
}

func TestThePullRequestLinksTheManifestAndTheTranscriptAtTheEvidenceCommit(t *testing.T) {
	dir, bare := finished(t, "yes")
	fake := &forge{}
	plan, _ := shipped(t, dir, fake)
	evidence := git(t, bare, "rev-parse", ship.EvidenceRef(plan.Manifest))
	if evidence != plan.Commits.Evidence {
		t.Fatalf("the remote's evidence ref is %s, and the plan named %s", evidence, plan.Commits.Evidence)
	}
	body := bodyOf(t, fake)
	for _, file := range []string{"out/manifest.json", "out/transcript.log"} {
		link := "https://github.com/o/r/blob/" + evidence + "/" + file
		if !strings.Contains(body, "("+link+")") {
			t.Fatalf("the body should link %s:\n%s", link, body)
		}
	}
	if manifest := git(t, bare, "show", evidence+":out/manifest.json"); !strings.Contains(manifest, `"signer": "a-test"`) {
		t.Fatalf("the evidence commit should hold the run's manifest:\n%s", manifest)
	}
}

// The evidence sits beside the change, never in it: the pull request's branch
// differs from its base by the patch alone.
func TestTheEvidenceIsNotPartOfTheChange(t *testing.T) {
	dir, bare := finished(t, "yes")
	plan, _ := shipped(t, dir, &forge{})
	if changed := git(t, bare, "diff", "--name-only", plan.At, plan.Branch); changed != "answer.txt" {
		t.Fatalf("the branch should change answer.txt and nothing else, changes:\n%s", changed)
	}
	if parent := git(t, bare, "rev-parse", ship.EvidenceRef(plan.Manifest)+"^"); parent != git(t, bare, "rev-parse", plan.Branch) {
		t.Fatal("the evidence commit's parent should be the pull request's commit")
	}
}

// printedCommand is the one line in the fenced block after "To verify it".
func printedCommand(t *testing.T, fake *forge) string {
	t.Helper()
	body := bodyOf(t, fake)
	found := regexp.MustCompile("To verify it[^\n]*\n\n~~~\n([^\n]+)\n~~~").FindStringSubmatch(body)
	if found == nil || !strings.HasPrefix(found[1], "gusto pr verify ") {
		t.Fatalf("the body should print one gusto pr verify command:\n%s", body)
	}
	return found[1]
}

func bodyOf(t *testing.T, fake *forge) string {
	t.Helper()
	requests := fake.requests()
	if len(requests) != 1 {
		t.Fatalf("want one request, got %d", len(requests))
	}
	body, _ := requests[0].Body["body"].(string)
	return body
}

// reviewed runs the command through a shell, as typed, with this repository's
// gusto first on the path, and returns everything it printed and its exit code.
func reviewed(t *testing.T, command string) (string, int) {
	t.Helper()
	shell := exec.Command("sh", "-c", command)
	shell.Env = append(os.Environ(), "PATH="+binaries(t)+string(os.PathListSeparator)+os.Getenv("PATH"))
	shell.Dir = t.TempDir()
	output, err := shell.CombinedOutput()
	if _, exited := err.(*exec.ExitError); err != nil && !exited {
		t.Fatal(err)
	}
	return string(output), shell.ProcessState.ExitCode()
}

// forged replaces the evidence on the remote, the way anybody who can push
// could: with files rewritten, and, when change names a commit, on top of that
// commit left as it is instead of on top of the patch.
func forged(t *testing.T, bare string, plan ship.Plan, change string, files map[string]string) {
	t.Helper()
	ref := ship.EvidenceRef(plan.Manifest)
	work := t.TempDir()
	git(t, work, "init", "--quiet")
	git(t, work, "fetch", "--quiet", bare, ref+":"+ref)
	if change == "" {
		change = git(t, work, "rev-parse", ref+"^")
	} else {
		git(t, work, "checkout", "--quiet", "--detach", change)
		change = commit(t, work, "--allow-empty", "-m", "a pull request commit without the patch")
	}
	git(t, work, "checkout", "--quiet", "--detach", change)
	git(t, work, "rm", "-r", "--quiet", "--cached", ".")
	git(t, work, "checkout", ref, "--", "job.json", "out")
	for name, text := range files {
		write(t, filepath.Join(work, name), text)
	}
	git(t, work, "add", "job.json", "out")
	commit(t, work, "-m", "forged evidence")
	git(t, work, "push", "--quiet", "--force", bare, "HEAD:"+ref)
}

func commit(t *testing.T, work string, args ...string) string {
	t.Helper()
	git(t, work, append([]string{"-c", "user.email=f@example.com", "-c", "user.name=F", "commit", "--quiet", "--no-verify"}, args...)...)
	return git(t, work, "rev-parse", "HEAD")
}

var (
	built     sync.Once
	binDir    string
	buildFail error
)

// binaries is a directory holding gusto, built once from this repository for
// every test that runs the printed command.
func binaries(t *testing.T) string {
	t.Helper()
	built.Do(func() {
		binDir, buildFail = os.MkdirTemp("", "gusto-bin-")
		if buildFail != nil {
			return
		}
		output, err := exec.Command("go", "build", "-o", filepath.Join(binDir, "gusto"), "github.com/getlatentic/gusto/cmd/gusto").CombinedOutput()
		if err != nil {
			buildFail = err
			t.Logf("%s", output)
		}
	})
	if buildFail != nil {
		t.Fatalf("building gusto: %v", buildFail)
	}
	return binDir
}

func TestMain(m *testing.M) {
	code := m.Run()
	if binDir != "" {
		os.RemoveAll(binDir)
	}
	os.Exit(code)
}
