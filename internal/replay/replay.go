// Package replay runs a finished task again, unchanged, and compares verdicts.
//
// A rerun is a fresh task directory made from the old one's job.json and
// adapter, pinned to the commit the old run started from. It runs like any
// other task and leaves its own evidence. The old directory is only read: its
// record is what the new verdict is compared against, so touching it would
// spoil the comparison.
package replay

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/job"
)

// Prepare makes the rerun of old under parent and returns its directory. It
// refuses a task with nothing to compare against: one that never finished, one
// whose record changed since, and a turn in a conversation, which has no check
// and no checkout of its own.
func Prepare(old job.Dir, parent string) (job.Dir, error) {
	if _, err := Known(old); err != nil {
		return "", err
	}
	spec, err := job.Read(old)
	if err != nil {
		return "", err
	}
	if spec.Session != "" {
		return "", fmt.Errorf("%s is a turn in a conversation, which has no check to rerun", old)
	}
	pin, err := Pinned(old.Workspace(), spec.Ref)
	if err != nil {
		return "", err
	}
	mark, err := marking(old, pin)
	if err != nil {
		return "", err
	}
	fresh, err := claim(old, parent)
	if err != nil {
		return "", err
	}
	if err := fill(old, fresh, spec.Harness, mark); err != nil {
		return "", errors.Join(err, os.RemoveAll(fresh.Path()))
	}
	return fresh, nil
}

// Known is the old run's manifest, once its record checks and it carries a
// verdict to compare against.
func Known(old job.Dir) (evidence.Manifest, error) {
	wrong, err := evidence.Check(old)
	if err != nil {
		return evidence.Manifest{}, fmt.Errorf("%s has no record to rerun against: %w", old, err)
	}
	if len(wrong) > 0 {
		return evidence.Manifest{}, fmt.Errorf("%s changed since its run (%s), so its verdict is not the one to compare against", old, wrong[0])
	}
	manifest, err := evidence.Read(old)
	if err != nil {
		return evidence.Manifest{}, err
	}
	if manifest.Gate == nil {
		return evidence.Manifest{}, fmt.Errorf("%s had no check, so there is no verdict to compare", old)
	}
	return manifest, nil
}

func marking(old job.Dir, pin Pin) (Mark, error) {
	of, err := filepath.Abs(old.Path())
	if err != nil {
		return Mark{}, err
	}
	digest, err := fileHash(old.OutFile("manifest.json"))
	if err != nil {
		return Mark{}, err
	}
	return Mark{Of: of, Manifest: digest, Commit: pin.Commit, PinnedBy: pin.By, HarnessCommits: pin.Committed}, nil
}

// fill writes everything a replay starts with: the spec, the adapter, and the
// mark.
func fill(old, fresh job.Dir, harness string, mark Mark) error {
	if err := fresh.Make(); err != nil {
		return err
	}
	if err := Rewrite(old, fresh, map[string]any{"ref": mark.Commit}); err != nil {
		return err
	}
	if err := Carry(old, fresh, harness); err != nil {
		return err
	}
	return writeMark(fresh, mark)
}

// claim makes the replay's directory beside the others under parent, named for
// the old job and numbered so a second replay of the same job never lands in
// the first one's directory. Mkdir either makes it or finds it taken, so two
// sweeps at once cannot share one.
func claim(old job.Dir, parent string) (job.Dir, error) {
	if parent == "" {
		parent = filepath.Dir(filepath.Clean(old.Path()))
	}
	base := filepath.Base(filepath.Clean(old.Path()))
	for n := 1; ; n++ {
		path := filepath.Join(parent, fmt.Sprintf("%s-replay-%d", base, n))
		err := os.Mkdir(path, 0o755)
		if err == nil {
			return job.Dir(path), nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
}

func fileHash(path string) (string, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(text)
	return hex.EncodeToString(sum[:]), nil
}
