package audit_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/audit"
	"github.com/getlatentic/gusto/internal/job"
)

const secret = "sk-test-0123456789abcdef"

func strict() audit.Policy {
	return audit.Policy{
		Drop: []string{"out/transcript.log"},
		Redact: audit.Redact{
			Fields:   []string{"brief"},
			Patterns: []audit.Pattern{{Name: "api-key", Match: `sk-[A-Za-z0-9-]{16,}`}},
		},
	}
}

// Redaction changes files, so it has to change them in a way the check
// expects. This also holds the wording Verify excuses to the wording
// evidence.Check writes: if either drifts, a redacted export stops verifying.
func TestARedactedExportStillVerifiesWhatRemains(t *testing.T) {
	export := exported(t, ran(t), audit.Options{Policy: strict()})
	report := verified(t, export)
	if len(report.Problems) != 0 {
		t.Fatalf("a redacted export should verify: %v", report.Problems)
	}
	for _, want := range []string{"out/transcript.log (dropped)", "job.json (rewritten)", "state.json (rewritten)", "out/harness.log (rewritten)"} {
		if !mentions(report.Redacted, want) {
			t.Errorf("want %s among %v", want, report.Redacted)
		}
	}
	if report.Covered == 0 {
		t.Fatal("the files the policy left alone are still covered by the manifest")
	}
	matchesTheSchema(t, export)
}

func TestNothingThePolicyHidesIsLeftInTheExport(t *testing.T) {
	export := exported(t, ran(t), audit.Options{Policy: strict()})
	if _, err := os.Stat(export.OutFile("transcript.log")); !os.IsNotExist(err) {
		t.Fatalf("the dropped transcript is still there: %v", err)
	}
	_ = filepath.WalkDir(export.Path(), func(file string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		text := read(t, file)
		if strings.Contains(text, secret) || strings.Contains(text, brief) {
			t.Errorf("%s still holds what the policy hides", file)
		}
		return nil
	})
	if spec := read(t, export.Spec()); !strings.Contains(spec, `"brief": "[redacted]"`) {
		t.Fatalf("the brief should read as redacted:\n%s", spec)
	}
}

// A reader of the summary learns what was removed, by which rule, and the
// hash of the original, rather than finding a gap.
func TestTheSummarySaysWhatWasRemovedAndWhy(t *testing.T) {
	export := exported(t, ran(t), audit.Options{Policy: strict()})
	record := summary(t, export)
	redactions, _ := record["redactions"].([]any)
	var dropped map[string]any
	for _, item := range redactions {
		if entry, _ := item.(map[string]any); entry["file"] == "out/transcript.log" {
			dropped = entry
		}
	}
	if dropped == nil || dropped["action"] != audit.Dropped || len(dropped["original"].(string)) != 64 {
		t.Fatalf("want the transcript recorded as dropped with its original hash, got %#v", redactions)
	}
	if !sameStrings(dropped["rules"], "drop out/transcript.log") {
		t.Fatalf("the rule that dropped it: %#v", dropped["rules"])
	}
	if got := at(record, "ran", "spec", "brief"); got != "[redacted]" {
		t.Fatalf("ran.spec.brief: %#v", got)
	}
}

// A redacted copy is the exporter's, so the export's own hash is all that
// holds it, and changing it afterwards is still caught by name.
func TestChangingARedactedCopyFails(t *testing.T) {
	export := exported(t, ran(t), audit.Options{Policy: strict()})
	write(t, export.Spec(), strings.Replace(read(t, export.Spec()), "fixture", "another", 1))
	if problems := verified(t, export).Problems; !mentions(problems, "job.json changed since the export") {
		t.Fatalf("want job.json named, got %v", problems)
	}
}

// A redaction that points at a hash the run never recorded would tie the
// redacted file to an original that was not this run's.
func TestARedactionNamingTheWrongOriginalFails(t *testing.T) {
	export := exported(t, ran(t), audit.Options{Policy: strict()})
	file := filepath.Join(export.Path(), audit.SummaryFile)
	record := summary(t, export)
	original := ""
	for _, item := range record["redactions"].([]any) {
		if entry := item.(map[string]any); entry["file"] == "out/transcript.log" {
			original = entry["original"].(string)
		}
	}
	write(t, file, strings.ReplaceAll(read(t, file), original, strings.Repeat("0", 64)))
	if problems := verified(t, export).Problems; !mentions(problems, "out/transcript.log: the redaction names an original") {
		t.Fatalf("want transcript.log named, got %v", problems)
	}
}

func TestAPolicyThatWouldBreakVerificationIsRefused(t *testing.T) {
	for _, policy := range []string{
		`{"drop": ["out/manifest.json"]}`,
		`{"drop": ["*.json"]}`,
		`{"redact": {"patterns": [{"name": "broken", "match": "("}]}}`,
		`{"redact": {"patterns": [{"match": "sk-"}]}}`,
	} {
		file := filepath.Join(t.TempDir(), "policy.json")
		write(t, file, policy)
		if _, err := audit.ReadPolicy(file); err == nil {
			t.Errorf("want refused: %s", policy)
		}
	}
}

func TestNoPolicyKeepsEverything(t *testing.T) {
	policy, err := audit.ReadPolicy("")
	if err != nil {
		t.Fatal(err)
	}
	export := exported(t, ran(t), audit.Options{Policy: policy})
	if !strings.Contains(read(t, export.OutFile("transcript.log")), secret) {
		t.Fatal("with no policy the transcript is exported as it was")
	}
}

// Only the request lines whose path names the job are carried, byte for byte.
func TestTheRequestLogIsCutToTheJob(t *testing.T) {
	from := ran(t)
	id := filepath.Base(from.Path())
	log := filepath.Join(t.TempDir(), "requests.log")
	write(t, log, strings.Join([]string{
		`{"time":"2026-09-26T09:00:01Z","level":"INFO","msg":"request","method":"GET","path":"/jobs/` + id + `","status":200,"remote":"10.0.0.7","caller":"cookie"}`,
		`{"time":"2026-09-26T09:00:02Z","level":"INFO","msg":"request","method":"GET","path":"/jobs/another-job","status":200,"remote":"10.0.0.8","caller":"cookie"}`,
		`not json: something journald kept`,
		`{"time":"2026-09-26T09:00:03Z","level":"INFO","msg":"request","method":"GET","path":"/jobs/` + id + `/bundle","status":200,"remote":"10.0.0.7","caller":"cookie"}`,
	}, "\n")+"\n")
	export := exported(t, from, audit.Options{RequestLog: log, Policy: audit.Policy{Redact: audit.Redact{Fields: []string{"remote"}}}})
	if problems := verified(t, export).Problems; len(problems) != 0 {
		t.Fatal(problems)
	}
	requests, _ := summary(t, export)["requests"].([]any)
	if len(requests) != 2 {
		t.Fatalf("want the two lines naming %s, got %#v", id, requests)
	}
	if got := at(requests[1], "path"); got != "/jobs/"+id+"/bundle" {
		t.Fatalf("second line: %#v", requests[1])
	}
	if got := at(requests[0], "remote"); got != "[redacted]" {
		t.Fatalf("the address should be redacted: %#v", got)
	}
	matchesTheSchema(t, export)
}

func TestAnExportOfAJobWithoutState(t *testing.T) {
	from := ran(t)
	if err := os.Remove(filepath.Join(from.Path(), "state.json")); err != nil {
		t.Fatal(err)
	}
	export := exported(t, from, audit.Options{})
	if problems := verified(t, export).Problems; len(problems) != 0 {
		t.Fatal(problems)
	}
	if got := at(summary(t, export), "outcome", "state"); got != audit.NotRecorded {
		t.Fatalf("a job made on the command line has no server state: %#v", got)
	}
	matchesTheSchema(t, job.Dir(export))
}
