package live

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// write puts text in path whole, as an editor saves a file: written beside it and renamed over it.
func write(t *testing.T, path, text string) {
	t.Helper()
	beside := path + ".writing"
	if err := os.WriteFile(beside, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(beside, path); err != nil {
		t.Fatal(err)
	}
}

// counted is a source whose Apply counts its calls and refuses what refuse says to.
type counted struct {
	path    string
	applied []string
	refuse  bool
}

func (c *counted) source() Source {
	return Source{Name: "policy", Path: c.path, Apply: func() error {
		text, _ := os.ReadFile(c.path)
		c.applied = append(c.applied, string(text))
		if c.refuse {
			return errors.New("not valid")
		}
		return nil
	}}
}

// watching is a watcher of one counted source, and what it was told.
func watching(t *testing.T, text string) (*watcher, *counted, *[]Change) {
	t.Helper()
	c := &counted{path: filepath.Join(t.TempDir(), "policy.json")}
	write(t, c.path, text)
	var heard []Change
	return newWatcher([]Source{c.source()}, func(change Change) { heard = append(heard, change) }), c, &heard
}

func TestAChangeIsAppliedOnceAndWhatWasThereAtStartIsNot(t *testing.T) {
	t.Parallel()
	w, c, heard := watching(t, "one")
	w.look()
	w.look()
	if len(c.applied) != 0 {
		t.Errorf("what was there at the start was applied: %q", c.applied)
	}
	write(t, c.path, "two")
	w.look()
	if len(c.applied) != 0 {
		t.Error("a change seen once was applied, before it could be told from a file half written")
	}
	w.look()
	w.look()
	if len(c.applied) != 1 || len(*heard) != 1 {
		t.Fatalf("applied %q, told %d times", c.applied, len(*heard))
	}
	change := (*heard)[0]
	if change.Err != nil || change.Name != "policy" || change.Was == change.Now || change.Was == "" {
		t.Errorf("told %+v", change)
	}
}

func TestAFileThatIsRefusedStaysRefusedUntilItChangesAndThenIsTakenAgain(t *testing.T) {
	t.Parallel()
	w, c, heard := watching(t, "good")
	c.refuse = true
	write(t, c.path, "bad")
	for range 4 {
		w.look()
	}
	if len(c.applied) != 1 || len(*heard) != 1 || (*heard)[0].Err == nil {
		t.Errorf("the same bad content was tried %d times, told %+v", len(c.applied), *heard)
	}
	c.refuse = false
	write(t, c.path, "fixed")
	w.look()
	w.look()
	if len(*heard) != 2 || (*heard)[1].Err != nil {
		t.Errorf("the fixed file was not taken: %+v", *heard)
	}
}

func TestAFileThatIsEmptiedBeforeItIsFilledIsNotTakenAsTheSetting(t *testing.T) {
	t.Parallel()
	w, c, _ := watching(t, `{"rules":[{"tool":"x","effect":"deny"}]}`)
	write(t, c.path, "")
	w.look()
	write(t, c.path, `{"rules":[{"tool":"y","effect":"deny"}]}`)
	w.look()
	w.look()
	if len(c.applied) != 1 || c.applied[0] != `{"rules":[{"tool":"y","effect":"deny"}]}` {
		t.Errorf("what was read: %q", c.applied)
	}
}

func TestAFolderChangesWhenAnyJSONFileInItDoesAndNotForOtherFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.json"), "1")
	first, _ := Fingerprint(dir)
	write(t, filepath.Join(dir, "notes.txt"), "x")
	if again, _ := Fingerprint(dir); again != first {
		t.Error("a file that is not .json changed the folder's fingerprint")
	}
	write(t, filepath.Join(dir, "b.json"), "2")
	second, _ := Fingerprint(dir)
	write(t, filepath.Join(dir, "a.json"), "11")
	third, _ := Fingerprint(dir)
	if first == second || second == third {
		t.Errorf("fingerprints %s %s %s", first, second, third)
	}
	if missing, err := Fingerprint(filepath.Join(dir, "none")); err != nil || missing != "" {
		t.Errorf("a path that is not there: %q %v", missing, err)
	}
	if none, err := Fingerprint(""); err != nil || none != "" {
		t.Errorf("no path: %q %v", none, err)
	}
}

func TestASourceWithNoPathIsNotWatched(t *testing.T) {
	t.Parallel()
	applied := false
	w := newWatcher([]Source{{Name: "none", Apply: func() error { applied = true; return nil }}}, nil)
	w.look()
	w.look()
	if applied {
		t.Error("a source with no path was applied")
	}
}

func TestWatchStopsWhenItsContextEnds(t *testing.T) {
	t.Parallel()
	ctx, stop := context.WithCancel(context.Background())
	ended := make(chan struct{})
	go func() {
		Watch(ctx, time.Millisecond, []Source{{Name: "policy", Path: filepath.Join(t.TempDir(), "policy.json"), Apply: func() error { return nil }}}, nil)
		close(ended)
	}()
	stop()
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("Watch went on after its context ended")
	}
}

func TestTheSettingsFileIsReadStrictly(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "live.json")
	write(t, good, `{"limitPerson": 2, "limitTeam": 0, "models": ["platform/qwen", "platform/gemma"]}`)
	settings, err := ReadSettings(good)
	if err != nil || settings.LimitPerson == nil || *settings.LimitPerson != 2 || settings.LimitTeam == nil || *settings.LimitTeam != 0 || len(settings.Models) != 2 {
		t.Fatalf("%+v %v", settings, err)
	}
	if settings, err := ReadSettings(filepath.Join(dir, "none.json")); err != nil || settings.LimitPerson != nil {
		t.Errorf("a missing file is no settings: %+v %v", settings, err)
	}
	for name, text := range map[string]string{
		"a misspelt limit":     `{"limitPersn": 2}`,
		"a negative limit":     `{"limitTeam": -1}`,
		"a model with a space": `{"models": ["a b"]}`,
		"an empty model":       `{"models": [""]}`,
		"not JSON":             `{`,
	} {
		bad := filepath.Join(dir, "bad.json")
		write(t, bad, text)
		if _, err := ReadSettings(bad); err == nil {
			t.Errorf("%s was taken", name)
		}
	}
}
