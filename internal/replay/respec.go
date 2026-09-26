package replay

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
)

// respec copies the old job.json with one change: ref becomes the pinned commit.
//
// The file is copied field by field rather than through job.Spec, so a field
// this build does not know survives into the replay instead of being dropped,
// and the replay asks for exactly what the original asked for.
func respec(old, fresh job.Dir, commit string) error {
	text, err := os.ReadFile(old.Spec())
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(text, &fields); err != nil {
		return err
	}
	if fields["ref"], err = json.Marshal(commit); err != nil {
		return err
	}
	if err := anchor(fields, old); err != nil {
		return err
	}
	out, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(fresh.Spec(), append(out, '\n'), 0o644); err != nil {
		return err
	}
	_, err = job.Read(fresh)
	return err
}

// anchor makes a relative repository path absolute. The runner clones from
// inside the job directory, so a path like ../repo meant a place relative to
// the old directory and would mean another one from the replay's.
func anchor(fields map[string]json.RawMessage, old job.Dir) error {
	var repo string
	if err := json.Unmarshal(fields["repo"], &repo); err != nil || repo == "" || filepath.IsAbs(repo) {
		return nil
	}
	there := filepath.Join(old.Path(), repo)
	if _, err := os.Stat(there); err != nil {
		return nil
	}
	absolute, err := filepath.Abs(there)
	if err != nil {
		return err
	}
	fields["repo"], err = json.Marshal(absolute)
	return err
}

// carry gives the replay the adapter the old job carried, so it runs the
// harness the same way. An old job that carried none ran a built-in adapter or
// one from HAAS_HARNESSES, and harness.Keep carries the one this machine has
// now, so a container that has never heard of it still runs it.
func carry(old, fresh job.Dir, name string) error {
	listing, err := os.ReadDir(harness.Carried(old))
	if os.IsNotExist(err) {
		return harness.Keep(fresh, name)
	}
	if err != nil {
		return err
	}
	if err := os.MkdirAll(harness.Carried(fresh), 0o755); err != nil {
		return err
	}
	for _, entry := range listing {
		if entry.IsDir() {
			continue
		}
		text, err := os.ReadFile(filepath.Join(harness.Carried(old), entry.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(harness.Carried(fresh), entry.Name()), text, 0o644); err != nil {
			return err
		}
	}
	return nil
}
