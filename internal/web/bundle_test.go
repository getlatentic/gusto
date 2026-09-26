package web

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/jobs"
	"github.com/getlatentic/haas/internal/runner"
)

// A harness is a program, so a program that makes the change runs the whole
// sequence with no model, no account and no container. %s is what it does.
const fixture = `{
  "name": "fixture",
  "program": "sh",
  "argv": ["-c", %q],
  "needs": "nothing",
  "stream": "plain"
}`

const wroteTheAnswer = "printf yes > answer.txt; echo wrote the answer"
const didNothing = "echo I did not touch a thing"

// refusal is what the gate prints when the answer is still no, and what a reader
// of a failed job has come to see.
const refusal = "answer.txt does not say yes"

// checkout is a repository whose gate is a script in its own tree, so whether
// the archive carries a mode bit decides whether the gate can be re-run at all.
func checkout(t *testing.T) string {
	t.Helper()
	at := t.TempDir()
	keep(t, filepath.Join(at, "answer.txt"), "no", 0o644)
	keep(t, filepath.Join(at, "gate.sh"),
		"#!/bin/sh\ntest \"$(cat answer.txt)\" = yes || { echo '"+refusal+"'; exit 1; }\n", 0o755)
	for _, args := range [][]string{
		{"init", "--quiet"},
		{"-c", "user.email=t@example.com", "-c", "user.name=T", "add", "answer.txt", "gate.sh"},
		{"-c", "user.email=t@example.com", "-c", "user.name=T", "commit", "--quiet", "-m", "start"},
	} {
		command := exec.Command("git", args...)
		command.Dir = at
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	return at
}

func keep(t *testing.T, path, text string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), mode); err != nil {
		t.Fatal(err)
	}
}

// ran does a real run in the server's own jobs directory, so the pages and the
// bundle are read off evidence a runner wrote rather than off a fixture of the
// shape somebody expected.
func (s *site) ranJob(t *testing.T, does string) (string, job.Dir) {
	t.Helper()
	harnesses := t.TempDir()
	keep(t, filepath.Join(harnesses, "fixture.json"), fmt.Sprintf(fixture, does), 0o644)
	t.Setenv("HAAS_HARNESSES", harnesses)
	t.Setenv("HAAS_SIGNER", "a-test")

	id := "20260101-0900-make-the-answer-yes"
	dir := job.Dir(filepath.Join(s.index.Root(), id))
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{
		Brief:   "make the answer yes",
		Repo:    checkout(t),
		Gate:    []string{"./gate.sh"},
		Harness: "fixture",
	}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}
	exit, err := runner.Inside(dir)
	if err != nil {
		t.Fatal(err)
	}
	state := jobs.Failed
	if exit == 0 {
		state = jobs.Passed
	}
	if err := s.index.Add(jobs.Record{
		ID: id, State: state, Brief: spec.Brief, Repo: spec.Repo,
		Harness: spec.Harness, Exit: exit, Queued: jobs.Now(), Ended: jobs.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	return id, dir
}

// unpack is what `tar -xzf` does: every entry at the mode the archive gives it.
func unpack(t *testing.T, archive io.Reader, into string) map[string]int64 {
	t.Helper()
	deflated, err := gzip.NewReader(archive)
	if err != nil {
		t.Fatalf("the bundle is not gzip: %v", err)
	}
	modes := map[string]int64{}
	reader := tar.NewReader(deflated)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("the bundle is not a whole tar: %v", err)
		}
		modes[header.Name] = header.Mode
		where := filepath.Join(into, filepath.FromSlash(header.Name))
		if header.Typeflag == tar.TypeDir {
			if err := os.MkdirAll(where, os.FileMode(header.Mode)); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(where), 0o755); err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(where, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(file, reader); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return modes
}

// verify is the two things `haas verify` does: re-hash the evidence against the
// manifest, and re-run the gate in the workspace and compare the answer.
func verify(t *testing.T, dir job.Dir) (string, bool) {
	t.Helper()
	wrong, err := evidence.Check(dir)
	if err != nil {
		return err.Error(), false
	}
	if len(wrong) > 0 {
		return strings.Join(wrong, ", "), false
	}
	manifest, err := evidence.Read(dir)
	if err != nil {
		return err.Error(), false
	}
	spec, err := job.Read(dir)
	if err != nil {
		return err.Error(), false
	}
	again, _, err := runner.Ask(spec, dir)
	if err != nil {
		return err.Error(), false
	}
	if again.Passed != manifest.Gate.Passed || again.Code != manifest.Gate.Code {
		return fmt.Sprintf("the gate now exits %d, and the manifest says %d", again.Code, manifest.Gate.Code), false
	}
	return fmt.Sprintf("%d files match, and the gate still exits %d", len(manifest.Files), again.Code), true
}

func TestTheBundleUnpacksIntoAJobThatVerifies(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id, _ := site.ranJob(t, wroteTheAnswer)

	got := site.ask(t, "GET", "/jobs/"+id+"/bundle", nil)
	if got.Code != http.StatusOK {
		t.Fatalf("the bundle gave %d", got.Code)
	}
	if got.Header().Get("Content-Disposition") != `attachment; filename="`+id+`.tar.gz"` {
		t.Errorf("a download should be named after the job, got %q", got.Header().Get("Content-Disposition"))
	}
	into := t.TempDir()
	unpack(t, got.Body, into)

	said, passed := verify(t, job.Dir(filepath.Join(into, id)))
	if !passed {
		t.Fatalf("verify refused the unpacked bundle: %s", said)
	}
	t.Log(said)
}

func TestTheBundleCarriesWhatVerifyReadsAndKeepsTheGateRunnable(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id, dir := site.ranJob(t, wroteTheAnswer)

	got := site.ask(t, "GET", "/jobs/"+id+"/bundle", nil)
	modes := unpack(t, got.Body, t.TempDir())
	for _, want := range []string{"job.json", "out/manifest.json", "out/gate.log", "workspace/answer.txt"} {
		if _, carried := modes[id+"/"+want]; !carried {
			t.Errorf("the bundle is missing %s", want)
		}
	}
	if _, carried := modes[id+"/state.json"]; carried {
		t.Error("state.json is this server's own cache and not evidence")
	}
	// A gate that is a script in the checkout is only re-runnable while the mode
	// bit survives the archive.
	if mode := modes[id+"/workspace/gate.sh"]; mode&0o111 == 0 {
		t.Errorf("gate.sh came out of the bundle as %#o and cannot be run", mode)
	}

	manifest, err := evidence.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range manifest.Files {
		if _, carried := modes[id+"/out/"+entry.Name]; !carried {
			t.Errorf("the manifest names %s and the bundle does not carry it", entry.Name)
		}
	}
}

// A runner destroyed before the gate leaves a job with no manifest, and an
// archive of it would be a bundle nobody can check.
func TestAJobWithNoManifestHasNoBundle(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id := "20260101-0900-nothing-ran"
	if err := job.Dir(filepath.Join(site.index.Root(), id)).Make(); err != nil {
		t.Fatal(err)
	}
	if err := site.index.Add(jobs.Record{
		ID: id, State: jobs.Interrupted, Brief: "nothing ran", Queued: jobs.Now(), Ended: jobs.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	got := site.ask(t, "GET", "/jobs/"+id+"/bundle", nil)
	if got.Code != http.StatusNotFound {
		t.Fatalf("a job with nothing to verify gave %d", got.Code)
	}
	if !strings.Contains(got.Body.String(), "nothing to verify") {
		t.Errorf("the refusal should say why, got %q", got.Body.String())
	}
}

func TestTheFileListShowsTheHashTheManifestClaims(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id, dir := site.ranJob(t, wroteTheAnswer)
	manifest, err := evidence.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) == 0 {
		t.Fatal("the run left no files to list")
	}

	page := site.ask(t, "GET", "/jobs/"+id, nil).Body.String()
	for _, entry := range manifest.Files {
		if !strings.Contains(page, entry.SHA256) {
			t.Errorf("the page does not show the hash of %s, so nobody can check it against the manifest", entry.Name)
		}
	}
	// manifest.json cannot hash itself, and saying so beats an empty cell.
	if !strings.Contains(page, "not in the manifest") {
		t.Error("a file the manifest does not name should say so")
	}
	if !strings.Contains(page, "/jobs/"+id+"/bundle") {
		t.Error("the bundle must be reachable from the job page")
	}
}

func TestAFailedJobShowsWhatTheGateSaid(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id, dir := site.ranJob(t, didNothing)
	record, _ := site.index.Get(id)
	if record.State != jobs.Failed {
		t.Fatalf("a harness that changed nothing should fail the gate, state %q", record.State)
	}
	log, err := os.ReadFile(dir.OutFile("gate.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(log), refusal) {
		t.Fatalf("the gate log does not hold the refusal: %q", log)
	}

	page := site.ask(t, "GET", "/jobs/"+id, nil).Body.String()
	if !strings.Contains(page, refusal) {
		t.Error("a failed job must show the gate's output inline, because that is the useful part")
	}
	if !strings.Contains(page, "What the gate said") {
		t.Error("the gate's output needs a heading a reader can find")
	}
}

func TestAPassingJobDoesNotRepeatTheGateOutput(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id, _ := site.ranJob(t, wroteTheAnswer)
	if page := site.ask(t, "GET", "/jobs/"+id, nil).Body.String(); strings.Contains(page, "What the gate said") {
		t.Error("a gate that passed has nothing to explain")
	}
}

// The hash on the page is only worth showing if it is the hash of the bytes the
// same server hands over, which is what an auditor does with it.
func TestTheHashOnThePageIsTheHashOfTheFileItServes(t *testing.T) {
	site := open(t)
	site.signIn(t)
	id, _ := site.ranJob(t, wroteTheAnswer)
	page := site.ask(t, "GET", "/jobs/"+id, nil).Body.String()

	for _, name := range []string{"gate.log", "diff.patch", "transcript.log"} {
		got := site.ask(t, "GET", "/jobs/"+id+"/files/"+name, nil)
		if got.Code != http.StatusOK {
			t.Fatalf("%s gave %d", name, got.Code)
		}
		sum := fmt.Sprintf("%x", sha256.Sum256(got.Body.Bytes()))
		if !strings.Contains(page, sum) {
			t.Errorf("%s hashes to %s and the page does not say so", name, sum)
		}
	}
}
