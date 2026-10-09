package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// A folder is kept when it is a folder, and not when it is a whole home or disk, a file, or named by a path from nowhere.
func TestAFolderIsRegisteredOnlyWhenItIsOneToKeep(t *testing.T) {
	dir := t.TempDir()
	folders := &FolderSync{Dir: filepath.Join(dir, "settings")}
	if err := folders.Open(); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(dir, "work", "thesis")
	os.MkdirAll(work, 0o755)
	file := filepath.Join(dir, "a-file")
	os.WriteFile(file, []byte("x"), 0o644)
	home, _ := os.UserHomeDir()

	key := "0123456789abcdef"
	if err := folders.Register(key, work); err != nil {
		t.Fatalf("a folder was refused: %v", err)
	}
	for name, path := range map[string]string{"a file": file, "a missing one": filepath.Join(dir, "gone"), "a relative path": "work/thesis", "the home": home, "the root": string(filepath.Separator)} {
		if err := folders.Register("fedcba9876543210", path); err == nil {
			t.Errorf("%s was taken", name)
		}
	}
	if err := folders.Register("nope", work); err == nil {
		t.Error("a key the app does not make was taken")
	}

	again := &FolderSync{Dir: folders.Dir}
	if err := again.Open(); err != nil {
		t.Fatal(err)
	}
	if got := again.Registered()[key]; got != work {
		t.Errorf("after a restart the folder is %q, not %q", got, work)
	}
}
