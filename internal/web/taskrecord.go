package web

import (
	"regexp"
	"strings"

	"github.com/getlatentic/gusto/internal/record"
	"github.com/getlatentic/gusto/internal/tasks"
	"github.com/getlatentic/gusto/pkg/task"
)

// The task page reads a finished task's record for the reviewer: the patch as a
// diff, the transcript as text, the check's verdict beside what it printed, and
// the command that checks all of it without this server. Every string here was
// written by a harness nobody trusts, and it reaches the page only through
// html/template, which escapes it.

// patchView is diff.patch drawn file by file.
type patchView struct {
	excerpt
	Files []diffFile
}

func readPatch(dir task.Dir, id string) (patchView, bool) {
	patch, found := readExcerpt(dir, id, "diff.patch", drawn)
	if !found {
		return patchView{}, false
	}
	return patchView{excerpt: patch, Files: parseDiff(patch.Text)}, true
}

// checkView is the check's answer. The verdict comes from the manifest, so a run
// that never reached its check has a log at most and no verdict to show.
type checkView struct {
	Verdict *record.Verdict
	Command string
	Log     *excerpt
	// Explains is a check that did not pass. Its output is then the answer the
	// reader came for and is drawn open; a pass has nothing to explain and
	// keeps its output folded away.
	Explains bool
}

func readCheck(entry tasks.Record, dir task.Dir, manifest *record.Manifest) checkView {
	check := checkView{Explains: entry.State != tasks.Passed}
	if manifest != nil && manifest.Check != nil {
		check.Verdict = manifest.Check
		check.Command = shellLine(manifest.Check.Command)
	}
	if log, found := readExcerpt(dir, entry.ID, "gate.log", drawn); found && !log.Empty() {
		check.Log = &log
	}
	return check
}

// bare is what a POSIX shell reads as one word without quotes.
var bare = regexp.MustCompile(`^[A-Za-z0-9_./:=@%+,-]+$`)

// shellLine writes a command so that pasting it into a shell runs the same
// argv the check ran. Joining with spaces would split `sh -c "go test ./..."`
// into words the check never had.
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

// drawRecord adds to the task page what the run left behind. The transcript
// and the check wait for the task to finish, because until then the live stream
// is the transcript and the check has not answered.
func drawRecord(page map[string]any, entry tasks.Record, dir task.Dir, manifest *record.Manifest) {
	if manifest != nil {
		page["Manifest"] = manifest
		page["Build"] = manifest.HarnessVersion
	}
	if patch, found := readPatch(dir, entry.ID); found {
		page["Patch"] = patch
	}
	if !entry.State.Done() {
		page["Live"] = true
		return
	}
	if transcript, found := readExcerpt(dir, entry.ID, "transcript.log", drawn); found {
		page["Transcript"] = transcript
	}
	page["Home"] = homeFullOf(dir)
	page["Check"] = readCheck(entry, dir, manifest)
}

// readManifest is the run's manifest, or nil for a task that has not written
// one: queued, running, or stopped before it got that far.
func readManifest(dir task.Dir) *record.Manifest {
	manifest, err := record.Read(dir)
	if err != nil {
		return nil
	}
	return &manifest
}

func entriesOf(manifest *record.Manifest) []record.Entry {
	if manifest == nil {
		return nil
	}
	return manifest.Files
}
