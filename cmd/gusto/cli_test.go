package main

import (
	"strings"
	"testing"

	"github.com/getlatentic/gusto/cmd/internal/cmdline/clitest"
)

func TestTheCommandLineKeepsTheContract(t *testing.T) {
	clitest.Verify(t, program())
}

func TestAWrongUseGivesTheUsageLineAndExitCode2(t *testing.T) {
	for _, args := range [][]string{
		{"new"}, {"run"}, {"run", "a", "b"}, {"inside"}, {"submit"}, {"show"}, {"rerun"}, {"compare", "-here"},
		{"benchmark"}, {"verify"}, {"verify", "dir", "extra"}, {"verify", "dir", "-nonsense"}, {"export"},
		{"pr"}, {"pr", "open"}, {"pr", "verify", "repo"}, {"local"}, {"local", "install", "extra"}, {"local", "use"},
		{"local", "sandbox"}, {"doctor", "extra"}, {"serve", "extra"}, {"version", "extra"}, {"agents", "extra"},
		{"keygen"}, {"keys", "extra"}, {"changes"}, {"changes", "verify"}, {"data"}, {"data", "rollback"},
		{"spend", "extra"}, {"retention", "-dir", ""},
	} {
		out, errs, code := clitest.Capture(t, func() int { return run(args) })
		if code != 2 {
			t.Errorf("%v: exit code %d, want 2", args, code)
		}
		if out != "" || !strings.Contains(errs, "usage: gusto ") || !strings.Contains(errs, "Run 'gusto help ") {
			t.Errorf("%v: stdout %q, stderr %q", args, out, errs)
		}
	}
}

func TestAnUnknownSubcommandSuggestsTheNearest(t *testing.T) {
	_, errs, code := clitest.Capture(t, func() int { return run([]string{"pr", "openn"}) })
	if code != 2 || !strings.Contains(errs, `unknown subcommand "openn" of gusto pr. Did you mean "open"?`) ||
		!strings.Contains(errs, "Run 'gusto help pr' for the subcommands.") {
		t.Errorf("exit code %d, stderr %q", code, errs)
	}
}

func TestTheOldCommandsAreGone(t *testing.T) {
	for _, old := range []string{"harnesses", "replay", "matrix", "ship", "check", "survey", "owner", "cell"} {
		if _, errs, code := clitest.Capture(t, func() int { return run([]string{old}) }); code != 2 || !strings.Contains(errs, "unknown command") {
			t.Errorf("gusto %s: exit code %d, stderr %q", old, code, errs)
		}
	}
	for _, args := range [][]string{{"cloud", "computers"}, {"local", "wall"}} {
		if _, _, code := clitest.Capture(t, func() int { return run(args) }); code != 2 {
			t.Errorf("gusto %v: exit code %d, want 2", args, code)
		}
	}
}

func TestFlagsTakeOneDashOrTwoAndHelpEndsAtTheDoubleDash(t *testing.T) {
	out, _, code := clitest.Capture(t, func() int { return verify([]string{"dir", "--help"}) })
	if code != 0 || !strings.Contains(out, "gusto verify:") {
		t.Errorf("--help on a command's own flag set: exit code %d, stdout %q", code, out)
	}
	out, _, code = clitest.Capture(t, func() int { return run([]string{"version", "--version"}) })
	if code != 2 || out != "" {
		t.Errorf("version took an argument: exit code %d, stdout %q", code, out)
	}
	if out, _, code := clitest.Capture(t, func() int { return run([]string{"--version"}) }); code != 0 || !strings.HasPrefix(out, "gusto ") {
		t.Errorf("--version: exit code %d, stdout %q", code, out)
	}
}

func TestSandboxRunIsHiddenButAnswersHelp(t *testing.T) {
	out, _, code := clitest.Capture(t, func() int { return run([]string{"help"}) })
	if code != 0 || strings.Contains(out, "sandbox-run") {
		t.Errorf("sandbox-run is in the global help: %q", out)
	}
	if out, _, code := clitest.Capture(t, func() int { return run([]string{"sandbox-run", "--help"}) }); code != 0 || !strings.Contains(out, "Landlock") {
		t.Errorf("sandbox-run --help: exit code %d, stdout %q", code, out)
	}
}
