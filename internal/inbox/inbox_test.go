package inbox

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func item(kind string) Item {
	return Item{At: time.Now(), Who: "ada@acme.com", Kind: kind, What: map[string]string{"email": "bob@acme.com"}}
}

func TestItemsComeOutOldestFirstWhateverTheirNames(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "inbox")
	kinds := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	for _, kind := range kinds {
		if err := Drop(dir, item(kind)); err != nil {
			t.Fatal(err)
		}
	}
	waiting, err := Pending(dir)
	if err != nil || len(waiting) != len(kinds) {
		t.Fatalf("%v %v", waiting, err)
	}
	for i, each := range waiting {
		if each.Err != nil || each.Item.Kind != kinds[i] {
			t.Errorf("place %d is %q, and was dropped %d-th: %+v", i, each.Item.Kind, i, each)
		}
	}
}

func TestAnUnfinishedFileIsNotAnItemAndWhatIsNotOneIsSaidSo(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".write-123"), []byte(`{"kind":"x","who":"y"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "00000000000000000000-bad.json"), []byte("nonsense"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "00000000000000000001-empty.json"), []byte(`{"who":"ada@acme.com"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Drop(dir, item("good")); err != nil {
		t.Fatal(err)
	}
	waiting, _ := Pending(dir)
	if len(waiting) != 3 {
		t.Fatalf("the unfinished file was taken for an item, or an item was missed: %+v", waiting)
	}
	if waiting[0].Err == nil || waiting[1].Err == nil || waiting[2].Err != nil || waiting[2].Item.Kind != "good" {
		t.Errorf("%+v", waiting)
	}
}

func TestAFileSetAsideIsNotMetAgainAndDoneForgetsWhatIsAlreadyGone(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "00000000000000000000-bad.json"), []byte("nonsense"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SetAside(dir, "00000000000000000000-bad"); err != nil {
		t.Fatal(err)
	}
	if waiting, _ := Pending(dir); len(waiting) != 0 {
		t.Errorf("a file that was set aside is still waiting: %+v", waiting)
	}
	if _, err := os.Stat(filepath.Join(dir, "00000000000000000000-bad.rejected")); err != nil {
		t.Errorf("it is kept where a person can read it: %v", err)
	}
	if err := Done(dir, "never-was"); err != nil {
		t.Errorf("finishing what is gone is not a failure: %v", err)
	}
}

func TestAnInboxThatIsNotThereHoldsNothing(t *testing.T) {
	if waiting, err := Pending(filepath.Join(t.TempDir(), "none")); err != nil || len(waiting) != 0 {
		t.Errorf("%v %v", waiting, err)
	}
}
