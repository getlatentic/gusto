package cloud

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A front door of a newer release finds the servers on a host apart still running, as the release that started them,
// and starts each again as its own. One already of its release is left running.
func TestAServerOfAnotherReleaseIsStartedAgainAsTheFrontDoorsRelease(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		before := controlled(t, s)
		org := made(t, s, "ada@acme.com", "Acme")
		if err := before.Provision(org.Slug); err != nil {
			t.Fatal(err)
		}
		address, _ := before.Route(org.Slug)
		first := whoIs(t, address)

		same := controlled(t, s)
		same.Cell = before.Cell
		if err := same.Provision(org.Slug); err != nil {
			t.Fatal(err)
		}
		if whoIs(t, address)["pid"] != first["pid"] {
			t.Fatal("a front door of the same release restarted the server")
		}

		newer := controlled(t, s)
		newer.Cell = before.Cell
		newer.Command = append(newer.Command, "--newer")
		if err := newer.Provision(org.Slug); err != nil {
			t.Fatal(err)
		}
		again, state := newer.Route(org.Slug)
		if state != Ready || whoIs(t, again)["pid"] == first["pid"] {
			t.Fatalf("the server of the older release still answers: %q", state)
		}
		if runtime, _ := s.Runtime(org.Slug); runtime.Command != newer.Command[0]+" serve --newer" {
			t.Errorf("what the server was started with is not written down: %q", runtime.Command)
		}
	})
}

func TestAReleaseIsSharedOnceUnderWhatItHoldsAndOldOnesAreRemoved(t *testing.T) {
	dir := t.TempDir()
	program := filepath.Join(t.TempDir(), "gusto")
	os.WriteFile(program, []byte("release one"), 0o755)
	shared, err := ShareRelease(program, dir)
	if err != nil {
		t.Fatal(err)
	}
	if text, _ := os.ReadFile(shared); string(text) != "release one" {
		t.Fatalf("the copy holds %q", text)
	}
	if info, _ := os.Stat(shared); info.Mode().Perm()&0o111 == 0 {
		t.Errorf("the copy cannot be run: %v", info.Mode())
	}
	if again, err := ShareRelease(program, dir); err != nil || again != shared {
		t.Errorf("the same release is put in another place: %q %v", again, err)
	}

	old := time.Now().Add(-2 * keptFor)
	var releases []string
	for _, text := range []string{"release two", "release three", "release four"} {
		os.WriteFile(program, []byte(text), 0o755)
		path, err := ShareRelease(program, dir)
		if err != nil {
			t.Fatal(err)
		}
		releases = append(releases, filepath.Dir(path))
	}
	// The first two are past the newest few, and only the first is more than a day old.
	os.Chtimes(filepath.Dir(shared), old, old)
	recent := time.Now().Add(-time.Hour)
	os.Chtimes(releases[0], recent, recent)
	os.WriteFile(program, []byte("release five"), 0o755)
	if _, err := ShareRelease(program, dir); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if _, err := os.Stat(filepath.Dir(shared)); !os.IsNotExist(err) || len(entries) != 4 {
		t.Errorf("an old release outside the newest few is kept, or a newer one removed: %d left", len(entries))
	}
}
