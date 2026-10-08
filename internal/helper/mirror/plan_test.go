package mirror

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"
	"time"
)

var moment = time.Date(2026, 10, 8, 14, 30, 5, 0, time.UTC)

func files(pairs ...string) Snapshot {
	out := Snapshot{}
	for i := 0; i+1 < len(pairs); i += 2 {
		out[pairs[i]] = Entry{Hash: pairs[i+1], Size: 1}
	}
	return out
}

func only(t *testing.T, base, local, remote Snapshot) Action {
	t.Helper()
	got := Plan(base, local, remote, moment, nil)
	if len(got) != 1 {
		t.Fatalf("%d actions: %+v", len(got), got)
	}
	return got[0]
}

func TestAChangeOnOneSideGoesToTheOther(t *testing.T) {
	for name, test := range map[string]struct {
		base, local, remote Snapshot
		want                Kind
	}{
		"a new local file is pushed":                      {files(), files("a", "1"), files(), Push},
		"a new cloud file is pulled":                      {files(), files(), files("a", "1"), Pull},
		"a local edit is pushed":                          {files("a", "1"), files("a", "2"), files("a", "1"), Push},
		"a cloud edit is pulled":                          {files("a", "1"), files("a", "1"), files("a", "2"), Pull},
		"a local deletion is carried to the cloud":        {files("a", "1"), files(), files("a", "1"), DeleteRemote},
		"a cloud deletion is carried to the local folder": {files("a", "1"), files("a", "1"), files(), DeleteLocal},
		"the same new file on both sides is settled":      {files(), files("a", "1"), files("a", "1"), Settle},
		"the same edit on both sides is settled":          {files("a", "1"), files("a", "2"), files("a", "2"), Settle},
		"a file nobody has any more is forgotten":         {files("a", "1"), files(), files(), Forget},
		"nothing changed is settled":                      {files("a", "1"), files("a", "1"), files("a", "1"), Settle},
	} {
		if got := only(t, test.base, test.local, test.remote).Kind; got != test.want {
			t.Errorf("%s: %s, want %s", name, got, test.want)
		}
	}
}

func TestAChangeBeatsADeletionOnEitherSide(t *testing.T) {
	if got := only(t, files("a", "1"), files("a", "2"), files()); got.Kind != Push {
		t.Errorf("a local edit against a cloud deletion: %s, and the edit has to survive", got.Kind)
	}
	if got := only(t, files("a", "1"), files(), files("a", "2")); got.Kind != Pull {
		t.Errorf("a cloud edit against a local deletion: %s, and the edit has to survive", got.Kind)
	}
}

func TestWhenBothSidesChangeAFileNeitherVersionIsLostAndTheLocalOneKeepsTheName(t *testing.T) {
	for name, test := range map[string]struct{ base, local, remote Snapshot }{
		"both edited it":                   {files("notes.md", "1"), files("notes.md", "2"), files("notes.md", "3")},
		"both made a new one, differently": {files(), files("notes.md", "2"), files("notes.md", "3")},
	} {
		got := only(t, test.base, test.local, test.remote)
		if got.Kind != Conflict || got.Copy != "notes.conflict-20261008T143005Z.md" {
			t.Errorf("%s: %+v", name, got)
		}
	}
}

func TestAConflictCopyKeepsItsFolderAndExtensionAndNeverTakesAFreeNameTwice(t *testing.T) {
	base, local, remote := files(), files("src/a.tar.gz", "1", "README", "1"), files("src/a.tar.gz", "2", "README", "2")
	got := Plan(base, local, remote, moment, nil)
	if got[0].Copy != "README.conflict-20261008T143005Z" || got[1].Copy != "src/a.tar.conflict-20261008T143005Z.gz" {
		t.Errorf("%+v", got)
	}
	remote["README.conflict-20261008T143005Z"] = Entry{Hash: "9"}
	again := Plan(base, local, remote, moment, func(p string) bool { return p == "src/a.tar.conflict-20261008T143005Z.gz" })
	if again[1].Copy != "README.conflict-20261008T143005Z-1" && again[0].Copy != "README.conflict-20261008T143005Z-1" {
		t.Errorf("a name already taken was used again: %+v", again)
	}
	if again[2].Copy != "src/a.tar.conflict-20261008T143005Z-1.gz" {
		t.Errorf("a name the caller says is taken was used again: %+v", again)
	}
}

func TestAPlanIsTheSameForTheSamePicturesWhateverTheOrderTheyWereBuiltIn(t *testing.T) {
	local := files("z", "1", "a", "1", "m", "2")
	remote := files("m", "3", "q", "1")
	first := Plan(Snapshot{}, local, remote, moment, nil)
	for i := 0; i < 20; i++ {
		if fmt.Sprint(Plan(Snapshot{}, local, remote, moment, nil)) != fmt.Sprint(first) {
			t.Fatal("two plans of the same pictures differ")
		}
	}
	if !sort.SliceIsSorted(first, func(i, j int) bool { return first[i].Path < first[j].Path }) {
		t.Errorf("the plan is not in the order of the paths: %+v", first)
	}
}

// both is a pair of folders that a plan is carried out on, the way the real thing carries it out, so a property can be asked of
// what comes of it.
type both struct{ local, remote map[string]string }

func (b *both) carry(actions []Action) {
	for _, act := range actions {
		switch act.Kind {
		case Pull:
			b.local[act.Path] = b.remote[act.Path]
		case Push:
			b.remote[act.Path] = b.local[act.Path]
		case DeleteLocal:
			delete(b.local, act.Path)
		case DeleteRemote:
			delete(b.remote, act.Path)
		case Conflict:
			b.local[act.Copy] = b.remote[act.Path]
			b.remote[act.Path] = b.local[act.Path]
			b.remote[act.Copy] = b.local[act.Copy]
		}
	}
}

func snapshotOf(content map[string]string) Snapshot {
	out := Snapshot{}
	for name, hash := range content {
		out[name] = Entry{Hash: hash, Size: 1}
	}
	return out
}

// Whatever the pictures, a plan carried out leaves the sides agreeing, a second plan has nothing left to do, and nothing that
// anybody had changed since the last agreement is gone from both sides.
func TestAPlanCarriedOutAlwaysConvergesAndLosesNoChange(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	names := []string{"a", "b", "dir/c"}
	contents := []string{"", "x", "y", "z"} // "" is a file that is not there
	pick := func() map[string]string {
		out := map[string]string{}
		for _, name := range names {
			if c := contents[rng.Intn(len(contents))]; c != "" {
				out[name] = c
			}
		}
		return out
	}
	for i := 0; i < 5000; i++ {
		base, local, remote := pick(), pick(), pick()
		world := &both{local: copyOf(local), remote: copyOf(remote)}
		world.carry(Plan(snapshotOf(base), snapshotOf(local), snapshotOf(remote), moment, nil))

		for _, act := range Plan(snapshotOf(base), snapshotOf(world.local), snapshotOf(world.remote), moment, nil) {
			if act.Kind != Settle && act.Kind != Forget {
				t.Fatalf("case %d: base %v local %v remote %v left %v / %v, and a second plan still has %+v", i, base, local, remote, world.local, world.remote, act)
			}
		}
		if fmt.Sprint(world.local) != fmt.Sprint(world.remote) {
			t.Fatalf("case %d: base %v local %v remote %v: the sides end different: %v / %v", i, base, local, remote, world.local, world.remote)
		}
		// A version somebody wrote since the last agreement is still on a side. A version is what a side held that was not
		// what the base held at that path, and the person's and the agent's both count.
		kept := map[string]bool{}
		for _, content := range world.local {
			kept[content] = true
		}
		for _, side := range []map[string]string{local, remote} {
			for name, content := range side {
				if content != base[name] && !kept[content] {
					t.Fatalf("case %d: base %v local %v remote %v: the change %q to %s is gone, the sides end %v", i, base, local, remote, content, name, world.local)
				}
			}
		}
	}
}

func copyOf(in map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func TestWhatIsRecordedAsAgreedFollowsWhatWasDone(t *testing.T) {
	base := files("keep", "1", "gone", "1", "edited", "1")
	local := files("keep", "1", "edited", "2", "new", "5", "both", "7")
	remote := files("keep", "1", "gone", "1", "edited", "1", "both", "8", "fromcloud", "6")
	actions := Plan(base, local, remote, moment, nil)
	next := Settled(base, local, remote, actions)
	want := []string{"both.conflict-20261008T143005Z=8", "both=7", "edited=2", "fromcloud=6", "keep=1", "new=5"}
	var got []string
	for name, entry := range next {
		got = append(got, name+"="+entry.Hash)
	}
	sort.Strings(got)
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("recorded %v, want %v", got, want)
	}
	// An action that did not happen leaves the old record, and is planned again the same.
	if kept := Settled(base, local, remote, nil); fmt.Sprint(kept) != fmt.Sprint(base) {
		t.Errorf("nothing was done, and the record changed: %v", kept)
	}
}
