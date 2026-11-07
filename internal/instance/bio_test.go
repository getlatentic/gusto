package instance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestABioIsAPersonsOwnAndSurvivesARestart(t *testing.T) {
	store := opened(t)
	if err := store.SetBio(" Ada@Example.com ", "  I run the launch.\r\nBased in Lagos.  "); err != nil {
		t.Fatal(err)
	}
	if got := store.Bio("ada@example.com"); got != "I run the launch.\nBased in Lagos." {
		t.Errorf("the bio is %q", got)
	}
	if store.Bio("bob@example.com") != "" {
		t.Error("ada's bio is bob's")
	}
	again, err := Open(store.Root())
	if err != nil {
		t.Fatal(err)
	}
	if again.Bio("ADA@example.com") == "" {
		t.Error("the bio was not kept across a restart")
	}
	if err := store.SetBio("ada@example.com", ""); err != nil {
		t.Fatal(err)
	}
	if store.Bio("ada@example.com") != "" {
		t.Error("an empty bio did not clear it")
	}
}

func TestABioIsCappedInCharacters(t *testing.T) {
	store := opened(t)
	if err := store.SetBio("ada@example.com", strings.Repeat("é", MaxBio)); err != nil {
		t.Errorf("a bio at the cap was refused: %v", err)
	}
	if err := store.SetBio("ada@example.com", strings.Repeat("é", MaxBio+1)); err == nil {
		t.Error("a bio over the cap was kept")
	}
	if got := store.Bio("ada@example.com"); len([]rune(got)) != MaxBio {
		t.Errorf("the refused bio changed the kept one to %d characters", len([]rune(got)))
	}
}

// A write that fails is no change.
func TestABioThatCouldNotBeWrittenIsNotKept(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes where it is told not to")
	}
	store := opened(t)
	if err := store.SetBio("ada@example.com", "first"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(store.Root(), 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(store.Root(), 0o755) })
	if err := store.SetBio("ada@example.com", "second"); err == nil {
		t.Fatal("a write that failed was reported as done")
	}
	if got := store.Bio("ada@example.com"); got != "first" {
		t.Errorf("the bio in memory is %q, ahead of its file", got)
	}
}

func TestADamagedBiosFileIsSetAsideAndTheStoreOpens(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, biosFile), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if store.Bio("ada@example.com") != "" {
		t.Error("a damaged file gave a bio")
	}
	if kept, _ := filepath.Glob(filepath.Join(root, biosFile+".damaged-*")); len(kept) != 1 {
		t.Errorf("the damaged file was not kept: %v", kept)
	}
	if err := store.SetBio("ada@example.com", "fresh"); err != nil {
		t.Fatal(err)
	}
}
