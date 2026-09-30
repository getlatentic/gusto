package atomicfile

import (
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
