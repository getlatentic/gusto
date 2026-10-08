package live

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

type told struct {
	mu      sync.Mutex
	changes []Change
}

func (c *told) add(change Change) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.changes = append(c.changes, change)
}
func (c *told) all() []Change {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Change(nil), c.changes...)
}

func waitFor(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("no %s in 5 seconds", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAChangeIsAppliedOnceAndWhatWasThereAtStartIsNot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.json")
	write(t, path, "one")
	var applied int
	var mu sync.Mutex
	heard := &told{}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go Watch(ctx, 5*time.Millisecond, []Source{{Name: "policy", Path: path, Apply: func() error { mu.Lock(); applied++; mu.Unlock(); return nil }}}, heard.add)
	time.Sleep(60 * time.Millisecond)
	mu.Lock()
	if applied != 0 {
		t.Errorf("what was there at the start was applied %d times", applied)
	}
	mu.Unlock()
	write(t, path, "two")
	waitFor(t, "a change", func() bool { return len(heard.all()) == 1 })
	time.Sleep(60 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	change := heard.all()[0]
	if applied != 1 || change.Err != nil || change.Name != "policy" || change.Was == change.Now || change.Was == "" || len(heard.all()) != 1 {
		t.Errorf("applied %d times, change %+v, told %d times", applied, change, len(heard.all()))
	}
}

func TestAFileThatIsRefusedStaysRefusedUntilItChangesAndThenIsTakenAgain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.json")
	write(t, path, "good")
	var mu sync.Mutex
	tried := 0
	refuse := true
	heard := &told{}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go Watch(ctx, 5*time.Millisecond, []Source{{Name: "policy", Path: path, Apply: func() error {
		mu.Lock()
		defer mu.Unlock()
		tried++
		if refuse {
			return errors.New("not valid")
		}
		return nil
	}}}, heard.add)
	time.Sleep(30 * time.Millisecond)
	write(t, path, "bad")
	waitFor(t, "a refusal", func() bool { return len(heard.all()) == 1 })
	time.Sleep(80 * time.Millisecond)
	mu.Lock()
	if tried != 1 || heard.all()[0].Err == nil {
		t.Errorf("the same bad content was tried %d times", tried)
	}
	refuse = false
	mu.Unlock()
	write(t, path, "fixed")
	waitFor(t, "the fix", func() bool { return len(heard.all()) == 2 })
	if heard.all()[1].Err != nil {
		t.Errorf("the fixed file was refused: %v", heard.all()[1].Err)
	}
}

func TestAFileThatIsEmptiedBeforeItIsFilledIsNotTakenAsTheSetting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "policy.json")
	write(t, path, `{"rules":[{"tool":"x","effect":"deny"}]}`)
	var mu sync.Mutex
	var read []string
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	go Watch(ctx, 20*time.Millisecond, []Source{{Name: "policy", Path: path, Apply: func() error {
		text, _ := os.ReadFile(path)
		mu.Lock()
		read = append(read, string(text))
		mu.Unlock()
		return nil
	}}}, nil)
	time.Sleep(30 * time.Millisecond)
	write(t, path, "")
	time.Sleep(5 * time.Millisecond)
	write(t, path, `{"rules":[{"tool":"y","effect":"deny"}]}`)
	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if len(read) != 1 || read[0] != `{"rules":[{"tool":"y","effect":"deny"}]}` {
		t.Errorf("what was read: %q", read)
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
	applied := false
	ctx, stop := context.WithCancel(context.Background())
	go Watch(ctx, 2*time.Millisecond, []Source{{Name: "none", Apply: func() error { applied = true; return nil }}}, nil)
	time.Sleep(30 * time.Millisecond)
	stop()
	if applied {
		t.Error("a source with no path was applied")
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
