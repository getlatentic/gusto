package web

import (
	"os"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/pkg/job"
)

func failedTurn(t *testing.T, files map[string]string) job.Dir {
	t.Helper()
	dir := job.Dir(t.TempDir())
	if err := os.MkdirAll(dir.Out(), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range files {
		if err := os.WriteFile(dir.OutFile(name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// A failed turn always has a line to show: the agent's last words when it left some, past blank lines and terminal codes, and
// a plain one when it left none (getlatentic/planning#727).
func TestAFailedTurnAlwaysSaysWhy(t *testing.T) {
	for name, tc := range map[string]struct {
		files map[string]string
		want  string
	}{
		"its last words":             {map[string]string{"stderr.log": "starting\nError: no model was chosen\n"}, "Error: no model was chosen"},
		"past blank lines and codes": {map[string]string{"stderr.log": "Error: 401 from the provider\n\n\x1b[0m\r\n  \n"}, "Error: 401 from the provider"},
		"the transcript after an empty error log": {map[string]string{"stderr.log": "\n", "transcript.log": "gusto refused this task\n"},
			"gusto refused this task"},
		"nothing at all": {nil, noReason},
	} {
		if got := stopped(failedTurn(t, tc.files)); got != tc.want {
			t.Errorf("%s: the turn says %q, want %q", name, got, tc.want)
		}
	}
}

// A long log is read from its end, and its last line is still found.
func TestTheReasonIsFoundAtTheEndOfALongLog(t *testing.T) {
	dir := failedTurn(t, map[string]string{"stderr.log": strings.Repeat("noise\n", 20000) + "the last word\n"})
	if got := stopped(dir); got != "the last word" {
		t.Errorf("the turn says %q", got)
	}
}

// When the proxy saw the model never answer, that is the reason, whatever the agent said on its way out (planning#676).
func TestATurnWhoseModelNeverAnsweredSaysSo(t *testing.T) {
	dir := failedTurn(t, map[string]string{"stderr.log": "done\n"})
	if err := evidence.Write(dir, evidence.Manifest{Job: "j", Harness: "pi", Signer: "unsigned", AtUnix: 1,
		ModelUnanswered: &evidence.Unanswered{Calls: 2, Reason: "dial tcp: connect: connection refused"}}); err != nil {
		t.Fatal(err)
	}
	if got := stopped(dir); got != "the model did not answer: dial tcp: connect: connection refused" {
		t.Errorf("the turn says %q", got)
	}
}
