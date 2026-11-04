package mirror

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Two engines of one pair of folders, as the helper's and a sync beside it are, each read the record the other left: a file one
// carried here and the person then deleted is deleted in the cloud by the other, and not carried back.
func TestTwoEnginesOfOneFolderEachReadWhatTheOtherLeft(t *testing.T) {
	p := newPair(t)
	p.remote.agent("notes.md", "draft")
	first, second := p.fresh(), p.fresh()
	if _, err := first.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if text, found := p.read("notes.md"); !found || text != "draft" {
		t.Fatalf("the first engine did not carry notes.md here: %q", text)
	}
	if err := os.Remove(filepath.Join(p.dir, "notes.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Once(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, found := p.read("notes.md"); found {
		t.Error("the second engine carried back a file the person deleted")
	}
	p.remote.mu.Lock()
	_, still := p.remote.files["notes.md"]
	p.remote.mu.Unlock()
	if still {
		t.Error("the file the person deleted is still in the cloud")
	}
}
