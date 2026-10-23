package replay

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/getlatentic/gusto/pkg/harness"
	"github.com/getlatentic/gusto/pkg/job"
)

// Rewrite copies from's job.json into to with the named fields set, and nothing
// else changed. A rerun sets ref to the pinned commit; a comparison sets the
// agent as well.
//
// The file is copied field by field rather than through job.Spec, so a field
// this build does not know survives into the copy instead of being dropped, and
// the copy asks for exactly what the original asked for. from and to may be the
// same directory.
func Rewrite(from, to job.Dir, set map[string]any) error {
	text, err := os.ReadFile(from.Spec())
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(text, &fields); err != nil {
		return err
	}
	for name, value := range set {
		if fields[name], err = json.Marshal(value); err != nil {
			return err
		}
	}
	if err := anchor(fields, from); err != nil {
		return err
	}
	out, err := json.MarshalIndent(fields, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(to.Spec(), append(out, '\n'), 0o644); err != nil {
		return err
	}
	_, err = job.Read(to)
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

// Carry gives the new task the adapters the old one carried, so it runs a
// agent the same way. When none of them is the named agent's, the old task
// ran a built-in adapter or one from GUSTO_HARNESSES, and harness.Keep carries
// the one this machine has now, so a container that has never heard of it
// still runs it.
func Carry(old, fresh job.Dir, name string) error {
	if err := copyCarried(old, fresh); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(harness.Carried(fresh), name+".json")); err == nil {
		return nil
	}
	return harness.Keep(fresh, name)
}

func copyCarried(old, fresh job.Dir) error {
	listing, err := os.ReadDir(harness.Carried(old))
	if os.IsNotExist(err) {
		return nil
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
