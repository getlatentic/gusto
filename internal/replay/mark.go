package replay

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"

	"github.com/getlatentic/haas/internal/job"
)

// MarkFile is what tells a replay from an original, in the replay's own out/.
// It is written before the run, so the manifest hashes it with the rest of the
// evidence and a replay cannot later be passed off as an original, or the
// other way round. The harness sees only which run it repeats, never that
// run's verdict.
const MarkFile = "replay.json"

// Mark names the run a replay repeats.
type Mark struct {
	Version int `json:"version"`
	// Of is the original job's directory, as an absolute path.
	Of string `json:"of"`
	// Manifest is the SHA-256 of the original's out/manifest.json, which names
	// one run of that directory even if it is run again.
	Manifest string `json:"manifest"`
	// Commit is where the replay's workspace starts: the commit the original
	// started from, and the ref job.json now carries.
	Commit string `json:"commit"`
	// PinnedBy is the reference in the original workspace the commit was read off.
	PinnedBy string `json:"pinnedBy"`
	// HarnessCommits counts what the original harness committed on top of the
	// commit. Its diff.patch does not show those changes.
	HarnessCommits int `json:"harnessCommits"`
}

// MarkVersion is the version of replay.json this code writes.
const MarkVersion = 1

func writeMark(dir job.Dir, mark Mark) error {
	mark.Version = MarkVersion
	text, err := json.MarshalIndent(mark, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(dir.OutFile(MarkFile), append(text, '\n'), 0o644)
}

// Marked reads the mark, and says whether the directory is a replay at all.
func Marked(dir job.Dir) (Mark, bool, error) {
	text, err := os.ReadFile(dir.OutFile(MarkFile))
	if errors.Is(err, fs.ErrNotExist) {
		return Mark{}, false, nil
	}
	if err != nil {
		return Mark{}, false, err
	}
	var mark Mark
	if err := json.Unmarshal(text, &mark); err != nil {
		return Mark{}, false, err
	}
	if err := job.Readable(MarkFile, mark.Version, MarkVersion); err != nil {
		return Mark{}, false, err
	}
	return mark, true, nil
}
