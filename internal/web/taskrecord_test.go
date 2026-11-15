package web

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/record"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/task"
)

// leftBehind is a task directory as a runner leaves it, written without running
// anything, so a page can show a record of any shape a harness might write.
type leftBehind struct {
	state tasks.State
	out   map[string]string
	check *record.Verdict
}

func (s *site) leave(t *testing.T, id string, run leftBehind) {
	t.Helper()
	dir := task.Dir(filepath.Join(s.index.Root(), id))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := task.Spec{Brief: "read the record", Repo: "/repo", Check: []string{"true"}, Harness: "fixture"}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir.Out(), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range run.out {
		put(t, dir.OutFile(name), text, 0o644)
	}
	if run.check != nil {
		sign(t, dir, *run.check)
	}
	if err := s.index.Add(tasks.Record{
		ID: id, State: run.state, Brief: spec.Brief, Repo: spec.Repo, Harness: spec.Harness,
		Queued: tasks.Now(), Ended: tasks.Now(),
	}); err != nil {
		t.Fatal(err)
	}
}

func sign(t *testing.T, dir task.Dir, check record.Verdict) {
	t.Helper()
	spec, files, err := record.Of(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := record.Write(dir, record.Manifest{Task: spec, Files: files, Check: &check, Signer: "a-test"}); err != nil {
		t.Fatal(err)
	}
}

var passed = &record.Verdict{Command: []string{"true"}, Code: 0, Passed: true}

func TestTheDiffIsDrawnFileByFileWithItsHunks(t *testing.T) {
	site := open(t)
	site.signIn(t)
	site.leave(t, "diffed", leftBehind{state: tasks.Passed, out: map[string]string{"diff.patch": twoFiles}, check: passed})

	page := site.task(t, "diffed")
	answer := page.patchFile(t, "answer.txt")
	page.patchFile(t, "notes.md")
	if len(answer.Hunks) != 1 || answer.Hunks[0].Header != "@@ -1,2 +1,2 @@ the answer" {
		t.Fatalf("answer.txt's hunks are %+v", answer.Hunks)
	}
	want := []diffLine{{Kind: "removed", Old: 1, Text: "no"}, {Kind: "added", New: 1, Text: "yes"}, {Kind: "context", Old: 2, New: 2, Text: "unchanged"}}
	for i, line := range want {
		if got := answer.Hunks[0].Lines[i]; got != line {
			t.Errorf("line %d is %+v, want %+v", i, got, line)
		}
	}
	for _, file := range page.Patch.Files {
		if strings.Contains(file.Name, "diff --git") {
			t.Error("the file headers are given as names, not dumped")
		}
	}
}

// The transcript, the patch and the check's output are written by a harness nobody trusts, and a page that ran a script from
// one would hand the harness the reviewer's session. The transcript is the one drawn as HTML here, so it is escaped here; the
// rest is text the page draws as text.
func TestMarkupFromTheHarnessIsDrawnAsText(t *testing.T) {
	site := open(t)
	site.signIn(t)
	site.leave(t, "hostile", leftBehind{state: tasks.Failed, out: map[string]string{
		"transcript.log": "done\n<script>alert('transcript')</script>\n",
		"diff.patch": "diff --git a/<b>x</b>.html b/<b>x</b>.html\n--- a/<b>x</b>.html\n+++ b/<b>x</b>.html\n" +
			"@@ -1 +1 @@\n-<p>old</p>\n+<script>alert('diff')</script>\n",
		"gate.log": "<img src=x onerror=alert('gate')>\n",
	}, check: &record.Verdict{Command: []string{"sh", "-c", "<script>alert('gate')</script>"}, Code: 1}})

	page := site.task(t, "hostile")
	if page.Transcript == nil || strings.Contains(page.Transcript.HTML, "<script>") {
		t.Fatalf("the transcript reached the page as markup: %+v", page.Transcript)
	}
	if !strings.Contains(page.Transcript.HTML, "&lt;script&gt;alert(&#39;transcript&#39;)&lt;/script&gt;") {
		t.Errorf("the transcript should show its script as text: %s", page.Transcript.HTML)
	}
	if page.Check == nil || page.Check.Log == nil || page.Check.Log.Text != "<img src=x onerror=alert('gate')>\n" {
		t.Errorf("the check's output should be given as the text it is: %+v", page.Check)
	}
}

// A run that changed nothing has no section about changes, so a page of ordinary replies is not full of empty answers.
func TestAnEmptyPatchHasNoSection(t *testing.T) {
	site := open(t)
	site.signIn(t)
	site.leave(t, "idle", leftBehind{state: tasks.Failed, out: map[string]string{"diff.patch": ""}, check: passed})
	if page := site.task(t, "idle"); page.Patch == nil || !page.Patch.Empty || len(page.Patch.Files) > 0 {
		t.Errorf("an empty patch should be said to be empty: %+v", page.Patch)
	}
}

func TestABinaryFileIsNamedAndNotDrawn(t *testing.T) {
	site := open(t)
	site.signIn(t)
	site.leave(t, "binary", leftBehind{state: tasks.Passed, out: map[string]string{
		"diff.patch": "diff --git a/logo.png b/logo.png\nnew file mode 100644\nindex 0000000..8c3f1a2\n" +
			"Binary files /dev/null and b/logo.png differ\n",
	}, check: passed})
	if logo := site.task(t, "binary").patchFile(t, "logo.png"); !logo.Binary || len(logo.Hunks) > 0 {
		t.Errorf("a binary file should be named and said to be binary: %+v", logo)
	}
}

// A harness can write any amount, and the page draws the start of it and links the rest rather than inlining megabytes.
func TestAHugeTranscriptAndPatchAreCutAndLinked(t *testing.T) {
	site := open(t)
	site.signIn(t)
	line := strings.Repeat("the harness goes on ", 4) + "\n"
	long := strings.Repeat(line, 2*drawn/len(line)) + "THE LAST LINE\n"
	patch := "diff --git a/big.txt b/big.txt\n--- a/big.txt\n+++ b/big.txt\n@@ -0,0 +1 @@\n" +
		strings.ReplaceAll(long, "the harness", "+the patch")
	site.leave(t, "huge", leftBehind{state: tasks.Passed, out: map[string]string{
		"transcript.log": long, "diff.patch": patch,
	}, check: passed})

	text := site.dataText(t, "/jobs/huge")
	if strings.Contains(text, "THE LAST LINE") {
		t.Error("the page drew past its limit")
	}
	if len(text) > 8*drawn {
		t.Errorf("the page's data is %d bytes", len(text))
	}
	page := site.task(t, "huge")
	for name, cut := range map[string]cutView{"transcript.log": page.Transcript.cutView, "diff.patch": page.Patch.cutView} {
		if !cut.Cut || cut.Raw != "/jobs/huge/files/"+name || cut.Name != name || cut.Shown == "" || cut.Whole == "" {
			t.Errorf("a cut %s should say so and link the whole file: %+v", name, cut)
		}
	}
}

func TestAShortTranscriptIsDrawnWholeWithItsLineBreaks(t *testing.T) {
	site := open(t)
	site.signIn(t)
	site.leave(t, "short", leftBehind{state: tasks.Passed, out: map[string]string{
		"transcript.log": "I read the tests.\nI changed answer.txt.\n",
	}, check: passed})
	transcript := site.task(t, "short").Transcript
	if transcript == nil || !strings.Contains(transcript.HTML, "I read the tests.<br>\nI changed answer.txt.") {
		t.Errorf("the transcript should be drawn with its line breaks: %+v", transcript)
	}
	if transcript != nil && transcript.Cut {
		t.Error("a file drawn whole has nothing more to link")
	}
}

func TestTheCheckShowsItsCommandItsExitAndWhatItPrinted(t *testing.T) {
	site := open(t)
	site.signIn(t)
	site.leave(t, "refused", leftBehind{state: tasks.Failed, out: map[string]string{"gate.log": "3 tests failed\n"},
		check: &record.Verdict{Command: []string{"sh", "-c", "go test ./... && exit 3"}, Code: 3}})

	check := site.task(t, "refused").Check
	if check == nil || check.Command != "sh -c 'go test ./... && exit 3'" || check.Verdict == nil || check.Verdict.Code != 3 ||
		check.Verdict.Passed || !check.Explains || check.Log == nil || check.Log.Text != "3 tests failed\n" {
		t.Errorf("the check should give its command, its exit, its verdict and what it said: %+v", check)
	}
}

// A running task's transcript is the live stream, and its check has not answered.
func TestARunningTaskIsLeftToTheStream(t *testing.T) {
	site := open(t)
	site.signIn(t)
	site.leave(t, "busy", leftBehind{state: tasks.Running, out: map[string]string{"transcript.log": "working\n"}})
	page := site.task(t, "busy")
	if !page.Live || page.Transcript != nil {
		t.Error("a running job should be left to the stream, not given the transcript so far")
	}
	if page.Check != nil || page.Proof != nil {
		t.Error("a job with no answer yet has no check and nothing to verify")
	}
}

func TestACheckIsWrittenSoAShellRunsTheSameArgv(t *testing.T) {
	for words, want := range map[string]string{
		"go|test|./...":            "go test ./...",
		"sh|-c|test 2 = 2":         "sh -c 'test 2 = 2'",
		"sh|-c|echo it's":          `sh -c 'echo it'\''s'`,
		"make|check|$(rm -rf ~)|;": `make check '$(rm -rf ~)' ';'`,
	} {
		if got := shellLine(strings.Split(words, "|")); got != want {
			t.Errorf("%q came out as %s, want %s", words, got, want)
		}
	}
}

// The command on the page is only worth showing if it works on what the page
// hands over, so this downloads the bundle and runs the command as written,
// with a gusto built from this tree.
func TestTheVerifyCommandOnThePageVerifiesTheBundle(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id, _ := site.ranTask(t, wroteTheAnswer)

	proof := site.task(t, id).Proof
	if proof == nil || proof.Command == "" {
		t.Fatal("the page gives no command to verify with")
	}
	command := proof.Command

	downloads := t.TempDir()
	bundle := site.ask(t, "GET", "/jobs/"+id+"/bundle", nil)
	put(t, filepath.Join(downloads, id+".tar.gz"), bundle.Body.String(), 0o644)

	bin := t.TempDir()
	build := exec.Command("go", "build", "-o", filepath.Join(bin, "gusto"), "github.com/getlatentic/gusto/cmd/gusto")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building gusto: %v: %s", err, output)
	}
	run := exec.Command("sh", "-c", command)
	run.Dir = downloads
	run.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v: %s", command, err, output)
	}
	if !strings.Contains(string(output), "files match, and the check still exits 0") {
		t.Errorf("%s said %s", command, output)
	}
}
