package evidence

import (
	"fmt"

	"github.com/getlatentic/haas/internal/harness"
	"github.com/getlatentic/haas/internal/job"
)

// NoDigest is what a manifest says in place of a digest it never recorded. A
// record from before the digest names its harness only by name, and inventing
// a digest for it would claim a configuration nobody measured.
const NoDigest = "no digest recorded"

// ConfiguredHarness names the configured harness that ran: its digest, or the
// words saying there is none.
func (m Manifest) ConfiguredHarness() string {
	if m.HarnessDigest == "" {
		return NoDigest
	}
	return m.HarnessDigest
}

// Comparable says whether two runs ran the same configured harness, so their
// results stand for one thing, and why in words. The same digest is the same
// harness. The same name is not: two teams both saying "opencode" can mean two
// prompts, two sets of skills and two model policies.
func Comparable(a, b Manifest) (bool, string) {
	switch {
	case a.HarnessDigest == "" || b.HarnessDigest == "":
		return false, fmt.Sprintf("%s is %s and %s is %s: a name alone does not say how a harness was configured",
			a.Harness, a.ConfiguredHarness(), b.Harness, b.ConfiguredHarness())
	case a.HarnessDigest == b.HarnessDigest:
		return true, fmt.Sprintf("both ran %s as %s", a.Harness, a.HarnessDigest)
	case a.Harness == b.Harness:
		return false, fmt.Sprintf("both are called %s but were configured differently: %s and %s",
			a.Harness, a.HarnessDigest, b.HarnessDigest)
	}
	return false, fmt.Sprintf("they ran different harnesses: %s as %s, and %s as %s",
		a.Harness, a.HarnessDigest, b.Harness, b.HarnessDigest)
}

// harnessComplaints is whether the configured harness the job carries is still
// the one the manifest names. The digest was taken before the harness started,
// so a harness that wrote to its own files during the run shows here, as a
// changed harness, rather than in the manifest. A job that carries none ran a
// harness built into the runner, and an export leaves the harness out; there
// is nothing here to check either against.
func harnessComplaints(dir job.Dir, manifest Manifest) []string {
	if manifest.HarnessDigest == "" {
		return nil
	}
	// A job.json that no longer reads is already reported by its own hash.
	spec, err := job.Read(dir)
	if err != nil {
		return nil
	}
	carried, found, err := harness.Carrying(dir, spec.Harness)
	switch {
	case err != nil:
		return []string{fmt.Sprintf("harnesses/%s cannot be read: %v", spec.Harness, err)}
	case found && carried.Digest != manifest.HarnessDigest:
		return []string{fmt.Sprintf("harnesses/%s changed", spec.Harness)}
	}
	return nil
}
