package harness

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/getlatentic/haas/internal/job"
)

func TestAConfiguredHarnessOpensWithItsAdapterAndDigest(t *testing.T) {
	mine := t.TempDir()
	configuredFixture(t, mine)
	t.Setenv("HAAS_HARNESSES", mine)
	configured, err := Open("", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if configured.Adapter.Name != "fixture" || configured.Digest != fixtureDigest {
		t.Fatalf("got %s with digest %s", configured.Adapter.Name, configured.Digest)
	}
}

// The adapter reaches the rest of its harness through {harnessDir}: a prompt,
// skills or servers are files a harness is pointed at.
func TestAnAdapterIsToldWhereItsHarnessIs(t *testing.T) {
	mine := t.TempDir()
	folder := configuredFixture(t, mine)
	t.Setenv("HAAS_HARNESSES", mine)
	adapter, err := Load("", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	command := adapter.Command(context.Background(), job.Spec{}, t.TempDir())
	if want := "cat " + folder + "/prompt.md"; command.Args[2] != want {
		t.Fatalf("argv %q, want %q", command.Args, want)
	}
}

// The job carries the whole configured harness, and what it carries digests as
// the original did, so the digest can be taken from the job and not the host.
func TestKeepCarriesTheWholeConfiguredHarness(t *testing.T) {
	mine := t.TempDir()
	configuredFixture(t, mine)
	t.Setenv("HAAS_HARNESSES", mine)
	dir := job.Dir(t.TempDir())
	if err := Keep(dir, "fixture"); err != nil {
		t.Fatal(err)
	}
	carried, found, err := Carrying(dir, "fixture")
	if err != nil || !found {
		t.Fatalf("found %v: %v", found, err)
	}
	if carried.Digest != fixtureDigest {
		t.Fatalf("the carried copy digests as %s, want %s", carried.Digest, fixtureDigest)
	}
	if _, err := os.Stat(filepath.Join(Carried(dir), "fixture", ".git")); err == nil {
		t.Fatal("version control metadata is not part of the harness and should not travel")
	}
}

// A job kept again holds the harness as it is now, and never the old form
// beside the new one.
func TestKeepReplacesWhatTheJobHeld(t *testing.T) {
	mine := t.TempDir()
	put(t, mine, "fixture.json", fixtureAdapter, 0o644)
	t.Setenv("HAAS_HARNESSES", mine)
	dir := job.Dir(t.TempDir())
	if err := Keep(dir, "fixture"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(mine, "fixture.json")); err != nil {
		t.Fatal(err)
	}
	configuredFixture(t, mine)
	if err := Keep(dir, "fixture"); err != nil {
		t.Fatal(err)
	}
	configured, err := Open(dir, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	if configured.Digest != fixtureDigest {
		t.Fatalf("digest %s, want %s", configured.Digest, fixtureDigest)
	}
}

// A built-in name overridden in HAAS_HARNESSES is what a runner here would run,
// so it is what the job carries.
func TestKeepCarriesAnOverrideOfABuiltInHarness(t *testing.T) {
	mine := t.TempDir()
	put(t, mine, "opencode.json", `{"name":"opencode","program":"true","needs":"nothing"}`, 0o644)
	t.Setenv("HAAS_HARNESSES", mine)
	dir := job.Dir(t.TempDir())
	if err := Keep(dir, "opencode"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(Carried(dir), "opencode.json")); err != nil {
		t.Fatalf("the override should travel: %v", err)
	}
}

func TestAHarnessOfferedAsBothAFileAndADirectoryIsRefused(t *testing.T) {
	mine := t.TempDir()
	configuredFixture(t, mine)
	put(t, mine, "fixture.json", fixtureAdapter, 0o644)
	t.Setenv("HAAS_HARNESSES", mine)
	if _, err := Open("", "fixture"); !errors.Is(err, errBoth) {
		t.Fatalf("want the ambiguity refused, got %v", err)
	}
}

func TestADirectoryWithoutAnAdapterIsRefused(t *testing.T) {
	mine := t.TempDir()
	put(t, mine, "empty/prompt.md", "Answer yes.\n", 0o644)
	t.Setenv("HAAS_HARNESSES", mine)
	if _, err := Open("", "empty"); err == nil {
		t.Fatal("a harness with no adapter has nothing to run")
	}
}

// A built-in harness digests as the bare adapter it is, so a job that carries
// a copy of it and one that does not name the same harness.
func TestABuiltInHarnessDigestsAsItsCarriedCopyWould(t *testing.T) {
	built, err := Open("", "opencode")
	if err != nil {
		t.Fatal(err)
	}
	text, err := builtIn.ReadFile("adapters/opencode.json")
	if err != nil {
		t.Fatal(err)
	}
	dir := job.Dir(t.TempDir())
	put(t, Carried(dir), "opencode.json", string(text), 0o644)
	carried, err := Open(dir, "opencode")
	if err != nil {
		t.Fatal(err)
	}
	if built.Digest != carried.Digest {
		t.Fatalf("%s and %s", built.Digest, carried.Digest)
	}
}

func TestNamesOffersAConfiguredHarnessDirectory(t *testing.T) {
	mine := t.TempDir()
	configuredFixture(t, mine)
	put(t, mine, "notes/README.md", "not a harness\n", 0o644)
	t.Setenv("HAAS_HARNESSES", mine)
	names := Names()
	if !slices.Contains(names, "fixture") || slices.Contains(names, "notes") {
		t.Fatalf("names %v", names)
	}
}
