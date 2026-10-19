package catalog

import (
	"errors"
	"strings"
	"testing"
)

// runtime stands in for docker or podman: it answers inspect with a label and
// run with whichever of the asked programs it holds, and keeps every question.
type runtime struct {
	label   string
	holds   map[string]bool
	missing bool
	asked   [][]string
}

func (r *runtime) ask(args ...string) (string, error) {
	r.asked = append(r.asked, args)
	if r.missing {
		return "", errors.New("exit status 125")
	}
	if args[0] == "image" {
		return r.label + "\n", nil
	}
	var found []string
	for _, program := range programsIn(args) {
		if r.holds[program] {
			found = append(found, program)
		}
	}
	return strings.Join(found, "\n"), nil
}

// programsIn is what follows the script and its $0 in a run.
func programsIn(run []string) []string {
	return run[scriptAt(run)+2:]
}

func scriptAt(run []string) int {
	for at, arg := range run {
		if arg == "-c" {
			return at + 1
		}
	}
	return len(run)
}

func surveyed(r *runtime) map[string]Entry {
	entries := Of(container{ask: r.ask, image: "gusto:dev"}, []string{"claude", "codex", "opencode"})
	byName := map[string]Entry{}
	for _, entry := range entries {
		byName[entry.Name] = entry
	}
	return byName
}

func TestAHarnessTheImageLacksSaysHowToCarryIt(t *testing.T) {
	entries := surveyed(&runtime{label: "opencode=opencode-ai", holds: map[string]bool{"opencode": true}})
	if !entries["opencode"].Ready || !entries["opencode"].Declared {
		t.Fatalf("opencode is declared and on the PATH, and reads %+v", entries["opencode"])
	}
	codex := entries["codex"]
	if codex.Ready || codex.Declared {
		t.Fatalf("codex is neither declared nor carried, and reads %+v", codex)
	}
	if !strings.Contains(codex.Why, "does not carry codex") || !strings.Contains(codex.Why, "HARNESSES") {
		t.Fatalf("codex says %q, which does not say what to do", codex.Why)
	}
}

func TestTheLabelIsAClaimAndThePathIsTheAnswer(t *testing.T) {
	entries := surveyed(&runtime{
		label: "opencode=opencode-ai codex=@openai/codex",
		holds: map[string]bool{"claude-agent-acp": true},
	})
	if entries["opencode"].Ready {
		t.Fatal("opencode is declared and not on the PATH, and was offered")
	}
	if !strings.Contains(entries["opencode"].Why, "says it carries opencode") {
		t.Fatalf("a label that lies should be named, got %q", entries["opencode"].Why)
	}
	// claude is on the PATH without being declared, and the PATH is the answer:
	// an image carrying a harness its label forgot still runs it.
	if !entries["claude"].Ready || entries["claude"].Declared {
		t.Fatalf("claude reads %+v", entries["claude"])
	}
}

func TestTheImageIsAskedOnceForEveryHarness(t *testing.T) {
	asked := &runtime{label: "opencode=opencode-ai", holds: map[string]bool{"opencode": true}}
	surveyed(asked)
	if len(asked.asked) != 2 {
		t.Fatalf("want one inspect and one container, got %d questions: %v", len(asked.asked), asked.asked)
	}
	run := asked.asked[1]
	if strings.Contains(run[scriptAt(run)], "opencode") || len(programsIn(run)) != 3 {
		t.Fatalf("each program belongs in the arguments, not in the script: %v", run)
	}
}

func TestAnImageThatCannotBeAskedOffersNothingAndSaysWhy(t *testing.T) {
	for name, entry := range surveyed(&runtime{missing: true}) {
		if entry.Ready {
			t.Errorf("%s was offered from an image nobody could ask", name)
		}
		if !strings.Contains(entry.Why, "could not be asked") {
			t.Errorf("%s says %q", name, entry.Why)
		}
	}
}

func TestTheLabelNamesTheHarnessesNotThePackages(t *testing.T) {
	for label, want := range map[string]string{
		"opencode=opencode-ai claude=@anthropic-ai/claude-code": "opencode claude",
		"codex=@openai/codex@0.1.0":                             "codex",
		"<no value>":                                            "",
		"":                                                      "",
	} {
		if got := strings.Join(Declared(label), " "); got != want {
			t.Errorf("%q declares %q, want %q", label, got, want)
		}
	}
}

func TestWhatAnImageAnswersIsItsProgramsAndTheBuildsItRecorded(t *testing.T) {
	onPath, versions := Answered("opencode\ncodex\n--\nopencode=opencode-ai@1.19.3\ncodex=@openai/codex@0.154.0\n")
	if !onPath["opencode"] || !onPath["codex"] || len(onPath) != 2 || versions["codex"] != "@openai/codex@0.154.0" || len(versions) != 2 {
		t.Errorf("got %v and %v", onPath, versions)
	}
	onPath, versions = Answered("opencode\n")
	if !onPath["opencode"] || len(versions) != 0 {
		t.Errorf("an image that recorded no builds: got %v and %v", onPath, versions)
	}
}
