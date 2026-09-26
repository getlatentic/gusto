package web

import (
	"html"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/jobs"
)

// leftBehind is a job directory as a runner leaves it, written without running
// anything, so a page can be shown evidence of any shape a harness might write.
type leftBehind struct {
	state jobs.State
	out   map[string]string
	gate  *evidence.Verdict
}

func (s *site) leave(t *testing.T, id string, run leftBehind) {
	t.Helper()
	dir := job.Dir(filepath.Join(s.index.Root(), id))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{Brief: "read the evidence", Repo: "/repo", Gate: []string{"true"}, Harness: "fixture"}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir.Out(), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range run.out {
		put(t, dir.OutFile(name), text, 0o644)
	}
	if run.gate != nil {
		sign(t, dir, *run.gate)
	}
	if err := s.index.Add(jobs.Record{
		ID: id, State: run.state, Brief: spec.Brief, Repo: spec.Repo, Harness: spec.Harness,
		Queued: jobs.Now(), Ended: jobs.Now(),
	}); err != nil {
		t.Fatal(err)
	}
}

func sign(t *testing.T, dir job.Dir, gate evidence.Verdict) {
	t.Helper()
	spec, files, err := evidence.Of(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := evidence.Write(dir, evidence.Manifest{Job: spec, Files: files, Gate: &gate, Signer: "a-test"}); err != nil {
		t.Fatal(err)
	}
}

func (s *site) page(t *testing.T, id string) string {
	t.Helper()
	got := s.ask(t, "GET", "/jobs/"+id, nil)
	if got.Code != 200 {
		t.Fatalf("the page gave %d", got.Code)
	}
	return got.Body.String()
}

var passed = &evidence.Verdict{Command: []string{"true"}, Code: 0, Passed: true}

func TestTheDiffIsDrawnFileByFileWithItsHunks(t *testing.T) {
	site := open(t)
	site.signIn(t)
	site.leave(t, "diffed", leftBehind{state: jobs.Passed, out: map[string]string{"diff.patch": twoFiles}, gate: passed})

	page := site.page(t, "diffed")
	for _, want := range []string{
		"<code>answer.txt</code>", "<code>notes.md</code>",
		`<td class="code">@@ -1,2 &#43;1,2 @@ the answer</td>`,
		`<tr class="removed"><td class="n">1</td><td class="n"></td><td class="code">no</td></tr>`,
		`<tr class="added"><td class="n"></td><td class="n">1</td><td class="code">yes</td></tr>`,
		`<tr class="context"><td class="n">2</td><td class="n">2</td><td class="code">unchanged</td></tr>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the page is missing %s", want)
		}
	}
	if strings.Contains(page, "diff --git") {
		t.Error("the file headers are drawn as names, not dumped")
	}
}

// The transcript, the patch and the gate's output are written by a harness
// nobody trusts, and a page that ran a script from one would hand the harness
// the reviewer's session.
func TestMarkupFromTheHarnessIsDrawnAsText(t *testing.T) {
	site := open(t)
	site.signIn(t)
	site.leave(t, "hostile", leftBehind{state: jobs.Failed, out: map[string]string{
		"transcript.log": "done\n<script>alert('transcript')</script>\n",
		"diff.patch": "diff --git a/<b>x</b>.html b/<b>x</b>.html\n--- a/<b>x</b>.html\n+++ b/<b>x</b>.html\n" +
			"@@ -1 +1 @@\n-<p>old</p>\n+<script>alert('diff')</script>\n",
		"gate.log": "<img src=x onerror=alert('gate')>\n",
	}, gate: &evidence.Verdict{Command: []string{"sh", "-c", "<script>alert('gate')</script>"}, Code: 1}})

	page := site.page(t, "hostile")
	for _, live := range []string{"<script>alert(", "<img src=x", "<b>x</b>", "<p>old</p>"} {
		if strings.Contains(page, live) {
			t.Errorf("%s reached the page as markup", live)
		}
	}
	for _, inert := range []string{
		"&lt;script&gt;alert(&#39;transcript&#39;)&lt;/script&gt;",
		"&lt;script&gt;alert(&#39;diff&#39;)&lt;/script&gt;",
		"&lt;img src=x onerror=alert(&#39;gate&#39;)&gt;",
	} {
		if !strings.Contains(page, inert) {
			t.Errorf("the page should show %s as text", inert)
		}
	}
}

func TestAnEmptyPatchSaysNothingChanged(t *testing.T) {
	site := open(t)
	site.signIn(t)
	site.leave(t, "idle", leftBehind{state: jobs.Failed, out: map[string]string{"diff.patch": ""}, gate: passed})
	if page := site.page(t, "idle"); !strings.Contains(page, "The harness changed nothing") {
		t.Error("an empty patch is an answer and should be given as one")
	}
}

func TestABinaryFileIsNamedAndNotDrawn(t *testing.T) {
	site := open(t)
	site.signIn(t)
	site.leave(t, "binary", leftBehind{state: jobs.Passed, out: map[string]string{
		"diff.patch": "diff --git a/logo.png b/logo.png\nnew file mode 100644\nindex 0000000..8c3f1a2\n" +
			"Binary files /dev/null and b/logo.png differ\n",
	}, gate: passed})
	page := site.page(t, "binary")
	if !strings.Contains(page, "<code>logo.png</code>") || !strings.Contains(page, "A binary file") {
		t.Error("a binary file should be named and said to be binary")
	}
}

// A harness can write any amount, and the page draws the start of it and
// links the rest rather than inlining megabytes.
func TestAHugeTranscriptAndPatchAreCutAndLinked(t *testing.T) {
	site := open(t)
	site.signIn(t)
	line := strings.Repeat("the harness goes on ", 4) + "\n"
	long := strings.Repeat(line, 2*drawn/len(line)) + "THE LAST LINE\n"
	patch := "diff --git a/big.txt b/big.txt\n--- a/big.txt\n+++ b/big.txt\n@@ -0,0 +1 @@\n" +
		strings.ReplaceAll(long, "the harness", "+the patch")
	site.leave(t, "huge", leftBehind{state: jobs.Passed, out: map[string]string{
		"transcript.log": long, "diff.patch": patch,
	}, gate: passed})

	page := site.page(t, "huge")
	if strings.Contains(page, "THE LAST LINE") {
		t.Error("the page drew past its limit")
	}
	for _, name := range []string{"transcript.log", "diff.patch"} {
		link := `<a href="/jobs/huge/files/` + name + `">Read the whole file</a>`
		if !strings.Contains(page, link) {
			t.Errorf("a cut %s should link the whole file", name)
		}
	}
	if !strings.Contains(page, "the page draws the first") {
		t.Error("a cut file should say it was cut")
	}
	if len(page) > 8*drawn {
		t.Errorf("the page is %d bytes", len(page))
	}
}

func TestAShortTranscriptIsDrawnWholeWithItsLineBreaks(t *testing.T) {
	site := open(t)
	site.signIn(t)
	site.leave(t, "short", leftBehind{state: jobs.Passed, out: map[string]string{
		"transcript.log": "I read the tests.\nI changed answer.txt.\n",
	}, gate: passed})
	page := site.page(t, "short")
	if !strings.Contains(page, "<div class=\"transcript\">I read the tests.\nI changed answer.txt.\n</div>") {
		t.Error("the transcript should be drawn as text with its line breaks")
	}
	if strings.Contains(page, "Read the whole file") {
		t.Error("a file drawn whole has nothing more to link")
	}
}

func TestTheGateShowsItsCommandItsExitAndWhatItPrinted(t *testing.T) {
	site := open(t)
	site.signIn(t)
	site.leave(t, "refused", leftBehind{state: jobs.Failed, out: map[string]string{"gate.log": "3 tests failed\n"},
		gate: &evidence.Verdict{Command: []string{"sh", "-c", "go test ./... && exit 3"}, Code: 3}})

	page := site.page(t, "refused")
	for _, want := range []string{
		"<code>sh -c &#39;go test ./... &amp;&amp; exit 3&#39;</code>",
		"<th>Exit code</th><td>3</td>",
		`<span class="state failed">false</span>`,
		"What the gate said", "3 tests failed",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the gate is missing %s", want)
		}
	}
}

// A running job's transcript is the live stream, and its gate has not answered.
func TestARunningJobIsLeftToTheStream(t *testing.T) {
	site := open(t)
	site.signIn(t)
	site.leave(t, "busy", leftBehind{state: jobs.Running, out: map[string]string{"transcript.log": "working\n"}})
	page := site.page(t, "busy")
	if !strings.Contains(page, `id="stream"`) || strings.Contains(page, `class="transcript"`) {
		t.Error("a running job should draw the stream, not the transcript so far")
	}
	if strings.Contains(page, "<h2>The gate</h2>") || strings.Contains(page, `id="verify"`) {
		t.Error("a job with no answer yet has no gate and nothing to verify")
	}
}

func TestAGateIsWrittenSoAShellRunsTheSameArgv(t *testing.T) {
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

var shownCommand = regexp.MustCompile(`<code id="verify">([^<]*)</code>`)

// The command on the page is only worth showing if it works on what the page
// hands over, so this downloads the bundle and runs the command as written,
// with a haas built from this tree.
func TestTheVerifyCommandOnThePageVerifiesTheBundle(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id, _ := site.ranJob(t, wroteTheAnswer)

	found := shownCommand.FindStringSubmatch(site.page(t, id))
	if found == nil {
		t.Fatal("the page gives no command to verify with")
	}
	command := html.UnescapeString(found[1])

	downloads := t.TempDir()
	bundle := site.ask(t, "GET", "/jobs/"+id+"/bundle", nil)
	put(t, filepath.Join(downloads, id+".tar.gz"), bundle.Body.String(), 0o644)

	bin := t.TempDir()
	build := exec.Command("go", "build", "-o", filepath.Join(bin, "haas"), "github.com/getlatentic/haas/cmd/haas")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building haas: %v: %s", err, output)
	}
	run := exec.Command("sh", "-c", command)
	run.Dir = downloads
	run.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := run.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %v: %s", command, err, output)
	}
	if !strings.Contains(string(output), "files match, and the gate still exits 0") {
		t.Errorf("%s said %s", command, output)
	}
}
