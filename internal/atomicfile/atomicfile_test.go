package atomicfile

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestAFileIsReplacedWholeAndNothingIsLeftBesideIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "one.json")
	for _, value := range []map[string]int{{"a": 1}, {"a": 2}} {
		if err := Write(path, value, 0o640); err != nil {
			t.Fatal(err)
		}
	}
	text, _ := os.ReadFile(path)
	if string(text) != "{\n  \"a\": 2\n}\n" {
		t.Errorf("%q", text)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o640 {
		t.Errorf("mode %v", info.Mode())
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("a finished file was left beside it: %v", entries)
	}
}

func TestAValueThatCannotBeWrittenLeavesTheFileAsItWas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "one.json")
	if err := Write(path, "kept", 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, make(chan int), 0o600); err == nil {
		t.Fatal("a channel was written")
	}
	if text, _ := os.ReadFile(path); string(text) != "\"kept\"\n" {
		t.Errorf("%q", text)
	}
}

// A reader that reads while the file is replaced again and again finds one version or the other, whole, every time.
func TestAReaderNeverFindsHalfAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	versions := [][]byte{bytes.Repeat([]byte("a"), 1<<20), bytes.Repeat([]byte("b"), 1<<20)}
	if err := WriteFile(path, versions[0], 0o644); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 100 {
			if err := WriteFile(path, versions[i%2], 0o644); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	for {
		select {
		case <-done:
			return
		default:
		}
		text, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(text, versions[0]) && !bytes.Equal(text, versions[1]) {
			t.Fatalf("a reader found %d bytes that are neither version", len(text))
		}
	}
}
