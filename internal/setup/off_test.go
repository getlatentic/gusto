package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/job"
)

func TestASetupThatIsOffStartsNoJobAndTurningItOnStartsOne(t *testing.T) {
	store, _ := Open(t.TempDir())
	store.Save(Setup{Name: "mine", Off: true}, memory())
	if _, err := store.Carry("mine", memory(), job.Dir(t.TempDir())); err == nil || !strings.Contains(err.Error(), "is off") {
		t.Fatalf("err = %v", err)
	}
	if err := store.SetOff("mine", false); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Carry("mine", memory(), job.Dir(t.TempDir())); err != nil {
		t.Errorf("a setup that was turned on was refused: %v", err)
	}
	store.SetOff("mine", true)
	if held, _ := store.Get("mine"); !held.Off {
		t.Error("turning it off did not keep")
	}
	if err := store.SetOff("nothing", true); err == nil {
		t.Error("a setup that is not there was turned off")
	}
}

func TestASetupKeptBeforeThereWasAnOffSwitchIsOn(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "old", "skills"), 0o755)
	os.WriteFile(filepath.Join(root, "old", "setup.json"), []byte(`{"name":"old","madeAt":1}`), 0o644)
	store, _ := Open(root)
	if held, found := store.Get("old"); !found || held.Off {
		t.Fatalf("%+v %v", held, found)
	}
	if _, err := store.Carry("old", Servers{}, job.Dir(t.TempDir())); err != nil {
		t.Errorf("a setup kept before the switch is off after an upgrade: %v", err)
	}
}

func TestEarlierVersionsOfASkillAreKeptEvenWhenItIsDroppedAndOneCanBePutBack(t *testing.T) {
	store, _ := Open(t.TempDir())
	store.Save(Setup{Name: "reviewer"}, Servers{})
	store.Write("reviewer", Skill{Name: "review", Description: "d", Body: "one"})
	first := store.Fingerprint("reviewer", "review")
	store.Write("reviewer", Skill{Name: "review", Description: "d", Body: "two"})
	store.Write("reviewer", Skill{Name: "review", Description: "d", Body: "two"})
	second := store.Fingerprint("reviewer", "review")
	if first == second || first == "none" {
		t.Fatalf("fingerprints %q %q", first, second)
	}
	store.Drop("reviewer", "review")
	if got := store.Fingerprint("reviewer", "review"); got != "none" {
		t.Errorf("a dropped skill has the fingerprint %q", got)
	}
	earlier := store.Earlier("reviewer")
	if len(earlier) != 2 || earlier[0].Fingerprint != second[:12] || earlier[1].Fingerprint != first[:12] || earlier[0].Present {
		t.Fatalf("earlier = %+v: the unchanged rewrite should not be kept twice", earlier)
	}
	if err := store.RestoreSkill("reviewer", "review", earlier[1].At); err != nil {
		t.Fatal(err)
	}
	held, _ := store.Get("reviewer")
	if len(held.Skills) != 1 || held.Skills[0].Body != "one" {
		t.Errorf("after putting the first back: %+v", held.Skills)
	}
	store.Write("reviewer", Skill{Name: "review", Description: "d", Body: "three"})
	if err := store.RestoreSkill("reviewer", "review", 12345); err == nil {
		t.Error("a version that was never kept was restored")
	}
	if err := store.RestoreSkill("../x", "review", 1); err == nil {
		t.Error("a name that climbs was taken")
	}
}

func TestOnlyTheLastFewVersionsOfASkillAreKept(t *testing.T) {
	store, _ := Open(t.TempDir())
	store.Save(Setup{Name: "reviewer"}, Servers{})
	for i := range mostEarlier + 6 {
		store.Write("reviewer", Skill{Name: "review", Description: "d", Body: strings.Repeat("x", i+1)})
	}
	if got := len(store.Earlier("reviewer")); got != mostEarlier {
		t.Errorf("%d versions are kept, and %d is the most", got, mostEarlier)
	}
}
