package harness

import (
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/job"
)

// jobWithSetup is a job carrying the configured fixture and a setup: a skill
// and a server definition, where the setup store puts them.
func jobWithSetup(t *testing.T, skill string) job.Dir {
	t.Helper()
	dir := job.Dir(t.TempDir())
	configuredFixture(t, Carried(dir))
	put(t, dir.SetupDir(), "skills/review/SKILL.md", skill, 0o644)
	put(t, dir.SetupDir(), "servers.json", `[{"name":"memory","command":"mcp-memory"}]`, 0o600)
	return dir
}

// The same adapter given two setups is two configured harnesses.
func TestTheSetupAJobCarriesIsPartOfItsDigest(t *testing.T) {
	bare, err := Open(job.Dir(""), "opencode")
	if err != nil {
		t.Fatal(err)
	}
	first, err := Open(jobWithSetup(t, "Review for bugs.\n"), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Open(jobWithSetup(t, "Review for style.\n"), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if first.Digest == fixtureDigest || first.Digest == second.Digest || first.Digest == bare.Digest {
		t.Fatalf("a setup should move the digest: %s, %s", first.Digest, second.Digest)
	}
}

// The digest of a harness with a setup is the digest of one directory holding
// both, the setup under setup/, so the steps in docs/FORMATS.md still hold.
func TestAHarnessAndItsSetupDigestAsOneDirectory(t *testing.T) {
	configured, err := Open(jobWithSetup(t, "Review for bugs.\n"), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	one := configuredFixture(t, t.TempDir())
	put(t, one, "setup/skills/review/SKILL.md", "Review for bugs.\n", 0o644)
	put(t, one, "setup/servers.json", `[{"name":"memory","command":"mcp-memory"}]`, 0o644)
	tree, err := walk(one)
	if err != nil {
		t.Fatal(err)
	}
	if configured.Digest != tree.Digest() {
		t.Fatalf("%s and %s", configured.Digest, tree.Digest())
	}
}

func TestAHarnessDirectoryMayNotHoldASetupOfItsOwn(t *testing.T) {
	mine := t.TempDir()
	folder := configuredFixture(t, mine)
	put(t, folder, "setup/servers.json", "[]", 0o644)
	t.Setenv("HAAS_HARNESSES", mine)
	_, err := Open("", "fixture")
	if err == nil || !strings.Contains(err.Error(), "has a setup folder") {
		t.Fatalf("want the setup folder refused, got %v", err)
	}
}
