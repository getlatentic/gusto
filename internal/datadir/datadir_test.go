package datadir

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withFormats runs a test under a store at a format with steps, and puts the
// real registry back after.
func withFormats(t *testing.T, store Store, format int, steps ...Step) {
	t.Helper()
	was, wasSteps := Current[store], Steps
	Current[store], Steps = format, steps
	t.Cleanup(func() { Current[store], Steps = was, wasSteps })
}

// noted is a step that adds and takes away a marker file, and counts its runs.
func noted(store Store, from int, ran *[]string, reversible bool) Step {
	name := filepath.Join("marker-" + string(rune('a'+from)))
	step := Step{Store: store, From: from, What: "add " + name,
		Up: func(dir string) error {
			*ran = append(*ran, "up:"+name)
			return os.WriteFile(filepath.Join(dir, name), nil, 0o644)
		}}
	if reversible {
		step.Down = func(dir string) error {
			*ran = append(*ran, "down:"+name)
			return os.Remove(filepath.Join(dir, name))
		}
	}
	return step
}

func stampOf(t *testing.T, dir string) Stamp {
	t.Helper()
	stamp, found, err := Read(dir)
	if err != nil || !found {
		t.Fatalf("no stamp in %s: %v", dir, err)
	}
	return stamp
}

func TestANewDirectoryIsStampedAndAnOldOneIsFormatOne(t *testing.T) {
	fresh := filepath.Join(t.TempDir(), "made-by-us")
	if _, err := Ensure(Jobs, fresh, Options{Release: "v1"}); err != nil {
		t.Fatal(err)
	}
	if stamp := stampOf(t, fresh); stamp.Format != Current[Jobs] || stamp.Release != "v1" {
		t.Errorf("stamp %+v", stamp)
	}

	old := t.TempDir()
	if err := os.WriteFile(filepath.Join(old, "20260101-job"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Ensure(Jobs, old, Options{Release: "v1"})
	if err != nil || !result.WasUnstamped || result.From != 1 {
		t.Fatalf("an unstamped directory with data: %+v %v", result, err)
	}
	if stampOf(t, old).Format != Current[Jobs] {
		t.Error("an old directory was not stamped")
	}
}

func TestEnsuringAgainChangesNothing(t *testing.T) {
	dir := t.TempDir()
	Ensure(Jobs, dir, Options{Release: "v1"})
	before, _ := os.ReadFile(filepath.Join(dir, StampFile))
	if result, err := Ensure(Jobs, dir, Options{Release: "v1"}); err != nil || result.Stamped || len(result.Migrated) > 0 {
		t.Fatalf("%+v %v", result, err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, StampFile))
	if string(before) != string(after) {
		t.Error("the stamp was rewritten with nothing to say")
	}
}

func TestADirectoryFromANewerReleaseIsRefusedByNameAndLeftAlone(t *testing.T) {
	withFormats(t, Sessions, 1)
	dir := t.TempDir()
	if err := write(dir, 3, "v9.0.0"); err != nil {
		t.Fatal(err)
	}
	_, err := Ensure(Sessions, dir, Options{Release: "v1"})
	var newer NewerError
	if !errors.As(err, &newer) || newer.Found != 3 || newer.Known != 1 {
		t.Fatalf("%v", err)
	}
	for _, want := range []string{dir, "format 3", "v9.0.0", "reads format 1", "Nothing has been changed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal lacks %q: %v", want, err)
		}
	}
	if stampOf(t, dir).Format != 3 {
		t.Error("a refusal changed the stamp")
	}
}

func TestAnUpgradeRunsEachStepInOrderAndStampsAsItGoes(t *testing.T) {
	var ran []string
	withFormats(t, Setups, 3, noted(Setups, 1, &ran, true), noted(Setups, 2, &ran, true))
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "setup"), nil, 0o644)
	result, err := Ensure(Setups, dir, Options{Release: "v2"})
	if err != nil || len(result.Migrated) != 2 {
		t.Fatalf("%+v %v", result, err)
	}
	if strings.Join(ran, " ") != "up:marker-b up:marker-c" || stampOf(t, dir).Format != 3 {
		t.Errorf("ran %v, stamp %+v", ran, stampOf(t, dir))
	}
}

func TestAStepThatFailsLeavesTheStampWhereItWasAndCanBeRunAgain(t *testing.T) {
	broken := true
	var ran []string
	first := noted(Setups, 1, &ran, true)
	second := Step{Store: Setups, From: 2, What: "the fragile one", Up: func(string) error {
		if broken {
			return errors.New("disk full")
		}
		return nil
	}, Down: func(string) error { return nil }}
	withFormats(t, Setups, 3, first, second)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "setup"), nil, 0o644)

	_, err := Ensure(Setups, dir, Options{Release: "v2"})
	if err == nil || !strings.Contains(err.Error(), "still at format 2") || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("%v", err)
	}
	if stampOf(t, dir).Format != 2 {
		t.Fatalf("the stamp says %d after a failed step", stampOf(t, dir).Format)
	}
	broken = false
	if _, err := Ensure(Setups, dir, Options{Release: "v2"}); err != nil || stampOf(t, dir).Format != 3 {
		t.Fatalf("running again: %v, format %d", err, stampOf(t, dir).Format)
	}
	if strings.Count(strings.Join(ran, " "), "up:marker-b") != 1 {
		t.Errorf("a step that had run ran again: %v", ran)
	}
}

func TestAnIrreversibleUpgradeIsRefusedUntilAllowed(t *testing.T) {
	var ran []string
	withFormats(t, Comparisons, 2, noted(Comparisons, 1, &ran, false))
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "one.json"), nil, 0o644)

	_, err := Ensure(Comparisons, dir, Options{Release: "v2"})
	var blocked IrreversibleError
	if !errors.As(err, &blocked) || len(ran) != 0 || stampOf2(dir) != 0 {
		t.Fatalf("an irreversible step ran without leave: %v, ran %v", err, ran)
	}
	for _, want := range []string{"cannot be undone", "HAAS_MIGRATE_IRREVERSIBLE=1", "Nothing has been changed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal lacks %q: %v", want, err)
		}
	}
	if _, err := Ensure(Comparisons, dir, Options{Release: "v2", Irreversible: true}); err != nil || len(ran) != 1 {
		t.Fatalf("allowed: %v ran %v", err, ran)
	}
}

// stampOf2 is the stamp's format, or 0 when there is none.
func stampOf2(dir string) int {
	stamp, found, _ := Read(dir)
	if !found {
		return 0
	}
	return stamp.Format
}

func TestRollbackUndoesStepsNewestFirst(t *testing.T) {
	var ran []string
	withFormats(t, Setups, 3, noted(Setups, 1, &ran, true), noted(Setups, 2, &ran, true))
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "setup"), nil, 0o644)
	if _, err := Ensure(Setups, dir, Options{Release: "v2"}); err != nil {
		t.Fatal(err)
	}
	ran = nil
	if err := Rollback(Setups, dir, 1, "v2"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(ran, " ") != "down:marker-c down:marker-b" || stampOf(t, dir).Format != 1 {
		t.Errorf("ran %v, stamp %+v", ran, stampOf(t, dir))
	}
	for _, marker := range []string{"marker-b", "marker-c"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			t.Errorf("%s is still there", marker)
		}
	}
	// The release that wrote format 1 can read it, and the newer one upgrades it again.
	withFormats(t, Setups, 1)
	if _, err := Ensure(Setups, dir, Options{Release: "v1"}); err != nil {
		t.Errorf("the older release cannot read a rolled-back directory: %v", err)
	}
}

func TestARollbackThatCannotBeDoneIsRefusedBeforeAnythingChanges(t *testing.T) {
	var ran []string
	withFormats(t, Setups, 3, noted(Setups, 1, &ran, true), noted(Setups, 2, &ran, false))
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "setup"), nil, 0o644)
	if _, err := Ensure(Setups, dir, Options{Release: "v2", Irreversible: true}); err != nil {
		t.Fatal(err)
	}
	ran = nil
	err := Rollback(Setups, dir, 1, "v2")
	if err == nil || !strings.Contains(err.Error(), "cannot be undone") || !strings.Contains(err.Error(), "Nothing has been changed") {
		t.Fatalf("%v", err)
	}
	if len(ran) != 0 || stampOf(t, dir).Format != 3 {
		t.Errorf("a refused rollback still did something: ran %v, format %d", ran, stampOf(t, dir).Format)
	}
	if CanRollback(Setups, dir, 1) == nil {
		t.Error("the check says a rollback is possible")
	}
	if err := CanRollback(Setups, dir, 3); err != nil {
		t.Errorf("staying where it is was refused: %v", err)
	}
}

// Every format a store has passed through has a step out of it, so no upgrade
// leaves data behind in a format nothing migrates.
func TestEveryFormatBelowTheCurrentOneHasAStep(t *testing.T) {
	for _, store := range Stores {
		for format := 1; format < Current[store]; format++ {
			if _, found := stepFrom(store, format); !found {
				t.Errorf("%s is at format %d and has no step from format %d", store, Current[store], format)
			}
		}
	}
}

func TestTheReleasesTableAgreesWithTheCode(t *testing.T) {
	if len(Releases) == 0 {
		t.Fatal("no release is listed")
	}
	for _, release := range Releases {
		for _, store := range Stores {
			format, listed := release.Formats[store]
			if !listed || format < 1 || format > Current[store] {
				t.Errorf("%s lists %s at format %d, and this release is at %d", release.Name, store, format, Current[store])
			}
		}
	}
	if _, err := FormatsOf("v0.0.0-nope"); err == nil {
		t.Error("an unknown release was accepted")
	}
}
