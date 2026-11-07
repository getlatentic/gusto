package ship

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/getlatentic/gusto/internal/record"
	"github.com/getlatentic/gusto/pkg/task"
)

// Shippable returns the manifest of a task that may become a pull request, or says why it may
// not. Three things have to hold: the record is the record the run left,
// the check passed by both records of it, and there is a patch to carry.
//
// Both records are read because the manifest does not hash itself. Editing
// manifest.json to say passed leaves every hash intact, and gates.json, which
// the manifest does hash, still holds the check's real answer.
func Shippable(dir task.Dir) (record.Manifest, error) {
	if err := untouched(dir); err != nil {
		return record.Manifest{}, err
	}
	manifest, err := record.Read(dir)
	if err != nil {
		return record.Manifest{}, err
	}
	if manifest.Check == nil {
		return record.Manifest{}, fmt.Errorf("%w: this task had no check, so nothing says its work is right", ErrRefused)
	}
	if err := passed(dir, *manifest.Check); err != nil {
		return record.Manifest{}, err
	}
	return manifest, carriesChange(dir)
}

// untouched re-hashes the record the way `gusto verify` does. Shipping a patch
// that changed after its check passed would put unchecked code in front of a
// reviewer under a passing verdict, which is the worst thing this can do.
func untouched(dir task.Dir) error {
	wrong, err := record.Check(dir)
	if err != nil {
		return fmt.Errorf("%w: the record cannot be read: %w", ErrRefused, err)
	}
	if len(wrong) > 0 {
		return fmt.Errorf("%w: the record changed since the run: %s", ErrRefused, strings.Join(wrong, "; "))
	}
	return nil
}

func passed(dir task.Dir, claimed record.Verdict) error {
	recorded, err := checks(dir)
	if err != nil {
		return fmt.Errorf("%w: gates.json cannot be read: %w", ErrRefused, err)
	}
	if !recorded.Passed || recorded.Code != 0 {
		return fmt.Errorf("%w: the check did not pass, it exited %d", ErrRefused, recorded.Code)
	}
	if !claimed.Passed || claimed.Code != recorded.Code {
		return fmt.Errorf("%w: the manifest and gates.json disagree about the check", ErrRefused)
	}
	return nil
}

func checks(dir task.Dir) (record.Verdict, error) {
	text, err := os.ReadFile(dir.OutFile("gates.json"))
	if err != nil {
		return record.Verdict{}, err
	}
	var verdict record.Verdict
	err = json.Unmarshal(text, &verdict)
	return verdict, err
}

func carriesChange(dir task.Dir) error {
	patch, err := os.ReadFile(dir.OutFile("diff.patch"))
	if err != nil {
		return fmt.Errorf("%w: the task left no patch: %w", ErrRefused, err)
	}
	if len(strings.TrimSpace(string(patch))) == 0 {
		return fmt.Errorf("%w: the patch is empty, so there is nothing to open a pull request for", ErrRefused)
	}
	return nil
}
