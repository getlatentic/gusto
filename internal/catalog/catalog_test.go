package catalog

import (
	"strings"
	"testing"

	"github.com/getlatentic/gusto/pkg/harness"
	"os"
	"path/filepath"
)

// A harness answering to one person's own sign-in has no account on a shared
// server, so it is withheld rather than offered and then failing. One that
// takes a key is offered, because the proxy holds the key and the harness is
// handed a token per job.
func TestOnlyAHarnessAnsweringToAPersonIsWithheld(t *testing.T) {
	// No runtime, so every program reads as present: what is left is the
	// credential, which is the whole point of the check.
	entries := Survey("", "", []string{"claude", "codex", "opencode"})
	ready := map[string]Entry{}
	for _, entry := range entries {
		ready[entry.Name] = entry
	}
	for _, entry := range entries {
		switch entry.Needs {
		case harness.NeedsAccount:
			if entry.Ready {
				t.Errorf("%s answers to somebody's own subscription and was offered anyway", entry.Name)
			}
			if !strings.Contains(entry.Why, "person") {
				t.Errorf("%s says %q, which does not tell anybody what to do", entry.Name, entry.Why)
			}
		default:
			if !entry.Ready {
				t.Errorf("%s needs nobody's account and was not offered: %q", entry.Name, entry.Why)
			}
		}
	}
	if ready["opencode"].Needs == harness.NeedsAccount {
		t.Error("opencode is not supposed to need anybody's account")
	}
}

func TestWhatIsReadyComesFirst(t *testing.T) {
	entries := Survey("", "", []string{"claude", "opencode", "codex"})
	if !entries[0].Ready {
		t.Fatalf("the list starts with %s, which is not ready", entries[0].Name)
	}
	seenUnready := false
	for _, entry := range entries {
		if !entry.Ready {
			seenUnready = true
		} else if seenUnready {
			t.Fatal("a harness that is ready is listed after one that is not")
		}
	}
}

func TestWhatEachHarnessWantsIsSaidPlainly(t *testing.T) {
	for needs, want := range map[harness.Needs]string{
		harness.NeedsNothing: "no model at all",
		harness.NeedsLocal:   "a model in the building",
		harness.NeedsAccount: "your own signed-in account",
		harness.NeedsKey:     "a key the proxy holds",
	} {
		if got := (Entry{Needs: needs}).Wants(); got != want {
			t.Errorf("%q reads as %q", needs, got)
		}
	}
}

func TestOnlyWhatIsReadyIsOfferedToAForm(t *testing.T) {
	offered := Survey("", "", []string{"claude", "codex", "opencode"})
	names := Ready(offered)
	for _, entry := range offered {
		named := false
		for _, name := range names {
			named = named || name == entry.Name
		}
		if named != entry.Ready {
			t.Errorf("%s is ready=%v and a form offers it=%v", entry.Name, entry.Ready, named)
		}
	}
}

// On a person's own computer a harness is ready when its program is on the PATH, and one that is not says how to install it.
func TestOnAPersonsComputerAHarnessIsReadyWhenItsProgramIsOnThePath(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "opencode"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	entries := Of(Machine(), []string{"opencode", "claude", "hermes"})
	byName := map[string]Entry{}
	for _, entry := range entries {
		byName[entry.Name] = entry
	}
	if !byName["opencode"].Ready {
		t.Errorf("opencode is on the PATH and not ready: %+v", byName["opencode"])
	}
	if claude := byName["claude"]; claude.Ready || !strings.HasPrefix(claude.Why, "npm install -g @agentclientprotocol/claude-agent-acp@") {
		t.Errorf("claude: %+v", claude)
	}
	// hermes is started through sh, which is on every PATH, and is ready only when hermes itself is.
	t.Setenv("PATH", bin+string(os.PathListSeparator)+"/bin"+string(os.PathListSeparator)+"/usr/bin")
	for _, entry := range Of(Machine(), []string{"hermes"}) {
		if entry.Ready || entry.Why != "npm install -g hermes-agent@0.21.5" {
			t.Errorf("hermes with only sh on the PATH: %+v", entry)
		}
	}
}
