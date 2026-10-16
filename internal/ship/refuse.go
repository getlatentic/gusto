package ship

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/job"
)

// Shippable returns the manifest of a job that may ship, or says why it may
// not. Three things have to hold: the evidence is the evidence the run left,
// the gate passed by both records of it, and there is a patch to carry.
//
// Both records are read because the manifest does not hash itself. Editing
// manifest.json to say passed leaves every hash intact, and gates.json, which
// the manifest does hash, still holds the gate's real answer.
func Shippable(dir job.Dir) (evidence.Manifest, error) {
	if err := untouched(dir); err != nil {
		return evidence.Manifest{}, err
	}
	manifest, err := evidence.Read(dir)
	if err != nil {
		return evidence.Manifest{}, err
	}
	if manifest.Gate == nil {
		return evidence.Manifest{}, fmt.Errorf("%w: this job had no gate, so nothing says its work is right", ErrRefused)
	}
	if err := passed(dir, *manifest.Gate); err != nil {
		return evidence.Manifest{}, err
	}
	return manifest, carriesChange(dir)
}

// untouched re-hashes the evidence the way `gusto verify` does. Shipping a patch
// that changed after its gate passed would put unchecked code in front of a
// reviewer under a passing verdict, which is the worst thing this can do.
func untouched(dir job.Dir) error {
	wrong, err := evidence.Check(dir)
	if err != nil {
		return fmt.Errorf("%w: the evidence cannot be read: %v", ErrRefused, err)
	}
	if len(wrong) > 0 {
		return fmt.Errorf("%w: the evidence changed since the run: %s", ErrRefused, strings.Join(wrong, "; "))
	}
	return nil
}

func passed(dir job.Dir, claimed evidence.Verdict) error {
	recorded, err := gates(dir)
	if err != nil {
		return fmt.Errorf("%w: gates.json cannot be read: %v", ErrRefused, err)
	}
	if !recorded.Passed || recorded.Code != 0 {
		return fmt.Errorf("%w: the gate did not pass, it exited %d", ErrRefused, recorded.Code)
	}
	if !claimed.Passed || claimed.Code != recorded.Code {
		return fmt.Errorf("%w: the manifest and gates.json disagree about the gate", ErrRefused)
	}
	return nil
}

func gates(dir job.Dir) (evidence.Verdict, error) {
	text, err := os.ReadFile(dir.OutFile("gates.json"))
	if err != nil {
		return evidence.Verdict{}, err
	}
	var verdict evidence.Verdict
	err = json.Unmarshal(text, &verdict)
	return verdict, err
}

func carriesChange(dir job.Dir) error {
	patch, err := os.ReadFile(dir.OutFile("diff.patch"))
	if err != nil {
		return fmt.Errorf("%w: the job left no patch: %v", ErrRefused, err)
	}
	if len(strings.TrimSpace(string(patch))) == 0 {
		return fmt.Errorf("%w: the patch is empty, so there is nothing to ship", ErrRefused)
	}
	return nil
}
