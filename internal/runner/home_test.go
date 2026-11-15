package runner

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// A kept home is the person's: the agent is handed only what a task brings to it, the server takes back only the conversation's
// state, and the rest is left as the agent left it. A home that is not kept is handed over and taken back whole.
func TestAKeptHomeIsHandedOverAndTakenBackOnlyInPart(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, each := range []string{".config", ".local/share", ".cache/npm"} {
		if err := os.MkdirAll(filepath.Join(home, each), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	kept := agent{present: true, keptHome: true, skills: filepath.Join(home, ".agents", "skills")}
	if got, want := kept.homeToGive(), []string{filepath.Join(home, ".config"), filepath.Join(home, ".local", "share")}; !slices.Equal(got, want) {
		t.Errorf("handed over %v, want %v: skills not yet placed are not there to hand over", got, want)
	}
	if got, want := kept.homeToTakeBack(), []string{filepath.Join(home, ".local", "share")}; !slices.Equal(got, want) {
		t.Errorf("took back %v, want %v", got, want)
	}
	whole := agent{present: true}
	if got := whole.homeToGive(); !slices.Equal(got, []string{home}) {
		t.Errorf("a home that is not kept is handed over whole, not %v", got)
	}
	if got := whole.homeToTakeBack(); !slices.Equal(got, []string{home}) {
		t.Errorf("a home that is not kept is taken back whole, not %v", got)
	}
}

// Each task in a kept home starts with the image's harness settings, over what an older image left, and with none of the
// skills an earlier task was given. What the person made there stays.
func TestAKeptHomeIsGivenTheImagesSettingsAndNoEarlierSkills(t *testing.T) {
	home, image := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	old := harnessSettings
	harnessSettings = image
	t.Cleanup(func() { harnessSettings = old })
	write(t, filepath.Join(image, "opencode", "opencode.json"), "new")
	write(t, filepath.Join(home, ".config", "opencode", "opencode.json"), "old")
	write(t, filepath.Join(home, ".config", "git", "config"), "theirs")
	write(t, filepath.Join(home, ".agents", "skills", "last-task", "SKILL.md"), "earlier")
	write(t, filepath.Join(home, ".cache", "npm", "index"), "theirs")
	kept := agent{present: true, keptHome: true, skills: filepath.Join(home, ".agents", "skills")}
	if err := kept.freshenHome(); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		".config/opencode/opencode.json": "new",
		".config/git/config":             "theirs",
		".cache/npm/index":               "theirs",
	} {
		if got, err := os.ReadFile(filepath.Join(home, path)); err != nil || string(got) != want {
			t.Errorf("%s holds %q (%v), want %q", path, got, err, want)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills")); !os.IsNotExist(err) {
		t.Errorf("an earlier task's skills are still there: %v", err)
	}
	if err := (agent{present: true}).freshenHome(); err != nil {
		t.Errorf("a home that is not kept is left as the image made it: %v", err)
	}
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
