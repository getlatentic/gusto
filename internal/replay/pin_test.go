package replay_test

import (
	"flag"
	"reflect"
	"testing"

	"github.com/getlatentic/gusto/internal/replay"
)

// A task naming a branch checks out a local branch of that name, and a harness
// that commits moves it. The clone's record of the remote branch does not move.
func TestABranchRefIsPinnedWhereTheRemoteHadItAtCloneTime(t *testing.T) {
	hermetic(t)
	bare := remote(t)
	started := git(t, bare, "rev-parse", "main")
	workingFolder := t.TempDir()
	git(t, workingFolder, "clone", "--quiet", bare, ".")
	git(t, workingFolder, "checkout", "--quiet", "main")
	write(t, workingFolder+"/answer.txt", "yes")
	git(t, workingFolder, "-c", "user.email=h@example.com", "-c", "user.name=H", "commit", "--quiet", "-am", "the harness's own")

	pin, err := replay.Pinned(workingFolder, "main")

	if err != nil {
		t.Fatal(err)
	}
	if pin.Commit != started || pin.By != "refs/remotes/origin/main" || pin.Committed != 1 {
		t.Fatalf("want %s from refs/remotes/origin/main with one harness commit, got %+v", started, pin)
	}
}

func TestACommitIDRefIsItsOwnPin(t *testing.T) {
	hermetic(t)
	bare := remote(t)
	started := git(t, bare, "rev-parse", "main")
	workingFolder := t.TempDir()
	git(t, workingFolder, "clone", "--quiet", bare, ".")

	pin, err := replay.Pinned(workingFolder, started)

	if err != nil || pin.Commit != started {
		t.Fatalf("want %s, got %+v, %v", started, pin, err)
	}
}

func TestAWorkingFolderThatNoLongerNamesTheRefIsRefused(t *testing.T) {
	hermetic(t)
	workingFolder := t.TempDir()
	git(t, workingFolder, "clone", "--quiet", remote(t), ".")

	if _, err := replay.Pinned(workingFolder, "no-such-branch"); err == nil {
		t.Fatal("a ref the clone never saw has no commit to pin")
	}
}

func TestFlagsAreReadWhereverTheySitAmongTheDirectories(t *testing.T) {
	flags := flag.NewFlagSet("replay", flag.ContinueOnError)
	here := flags.Bool("here", false, "")
	into := flags.String("into", "", "")

	named, err := replay.Interspersed(flags, []string{"a", "-here", "b", "-into", "there", "c"})

	if err != nil || !*here || *into != "there" || !reflect.DeepEqual(named, []string{"a", "b", "c"}) {
		t.Fatalf("got %v, here %t, into %q, %v", named, *here, *into, err)
	}
}
