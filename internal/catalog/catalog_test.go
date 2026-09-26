package catalog

import (
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/harness"
)

func TestAHarnessThatSignsInAsAPersonIsNotOfferedByAServer(t *testing.T) {
	// No runtime, so every program reads as present: what is left is the
	// credential, which is the whole point of the check.
	entries := Survey("", "", []string{"claude", "codex", "opencode"})
	ready := map[string]Entry{}
	for _, entry := range entries {
		ready[entry.Name] = entry
	}
	for _, name := range []string{"claude", "codex"} {
		if ready[name].Ready {
			t.Errorf("%s answers to somebody's own subscription and was offered anyway", name)
		}
		if !strings.Contains(ready[name].Why, "person") {
			t.Errorf("%s says %q, which does not tell anybody what to do", name, ready[name].Why)
		}
	}
	if !ready["opencode"].Ready {
		t.Fatalf("opencode needs nobody's account and was not offered: %q", ready["opencode"].Why)
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
	names := Ready(Survey("", "", []string{"claude", "codex", "opencode"}))
	if len(names) != 1 || names[0] != "opencode" {
		t.Fatalf("a form would offer %v", names)
	}
}

func TestTheModelsAServerOffersComeFromItsDeployment(t *testing.T) {
	models := Models(" platform/qwen.qwen3-vl-235b-a22b-instruct , ollama/llama3.2:3b ,, ")
	if len(models) != 2 || models[0] != "platform/qwen.qwen3-vl-235b-a22b-instruct" {
		t.Fatalf("got %v", models)
	}
}
