package web

import (
	"regexp"
	"strings"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/jobs"
)

// The job page reads a finished job's evidence for the reviewer: the patch as a
// diff, the transcript as text, the gate's verdict beside what it printed, and
// the command that checks all of it without this server. Every string here was
// written by a harness nobody trusts, and it reaches the page only through
// html/template, which escapes it.

// patchView is diff.patch drawn file by file.
type patchView struct {
	excerpt
	Files []diffFile
}

func readPatch(dir job.Dir, id string) (patchView, bool) {
	patch, found := readExcerpt(dir, id, "diff.patch", drawn)
	if !found {
		return patchView{}, false
	}
	return patchView{excerpt: patch, Files: parseDiff(patch.Text)}, true
}

// gateView is the gate's answer. The verdict comes from the manifest, so a run
// that never reached its gate has a log at most and no verdict to show.
type gateView struct {
	Verdict *evidence.Verdict
	Command string
	Log     *excerpt
	// Explains is a gate that did not pass. Its output is then the answer the
	// reader came for and is drawn open; a pass has nothing to explain and
	// keeps its output folded away.
	Explains bool
}

func readGate(record jobs.Record, dir job.Dir, manifest *evidence.Manifest) gateView {
	gate := gateView{Explains: record.State != jobs.Passed}
	if manifest != nil && manifest.Gate != nil {
		gate.Verdict = manifest.Gate
		gate.Command = shellLine(manifest.Gate.Command)
	}
	if log, found := readExcerpt(dir, record.ID, "gate.log", drawn); found && !log.Empty() {
		gate.Log = &log
	}
	return gate
}

// bare is what a POSIX shell reads as one word without quotes.
var bare = regexp.MustCompile(`^[A-Za-z0-9_./:=@%+,-]+$`)

// shellLine writes a command so that pasting it into a shell runs the same
// argv the gate ran. Joining with spaces would split `sh -c "go test ./..."`
// into words the gate never had.
func shellLine(words []string) string {
	quoted := make([]string, len(words))
	for i, word := range words {
		quoted[i] = shellWord(word)
	}
	return strings.Join(quoted, " ")
}

func shellWord(word string) string {
	if bare.MatchString(word) {
		return word
	}
	return "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
}

// drawEvidence adds to the job page what the run left behind. The transcript
// and the gate wait for the job to finish, because until then the live stream
// is the transcript and the gate has not answered.
func drawEvidence(page map[string]any, record jobs.Record, dir job.Dir, manifest *evidence.Manifest) {
	if manifest != nil {
		page["Manifest"] = manifest
	}
	if patch, found := readPatch(dir, record.ID); found {
		page["Patch"] = patch
	}
	if !record.State.Done() {
		page["Live"] = true
		return
	}
	if transcript, found := readExcerpt(dir, record.ID, "transcript.log", drawn); found {
		page["Transcript"] = transcript
	}
	page["Gate"] = readGate(record, dir, manifest)
}

// readManifest is the run's manifest, or nil for a job that has not written
// one: queued, running, or stopped before it got that far.
func readManifest(dir job.Dir) *evidence.Manifest {
	manifest, err := evidence.Read(dir)
	if err != nil {
		return nil
	}
	return &manifest
}

func entriesOf(manifest *evidence.Manifest) []evidence.Entry {
	if manifest == nil {
		return nil
	}
	return manifest.Files
}
