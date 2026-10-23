package audit_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/audit"
	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/signing"
	"github.com/getlatentic/gusto/pkg/job"
	"github.com/getlatentic/gusto/schema"
)

func exported(t *testing.T, from job.Dir, options audit.Options) job.Dir {
	t.Helper()
	if options.Now == nil {
		options.Now = func() int64 { return 200 }
	}
	to := filepath.Join(t.TempDir(), "export")
	if _, err := audit.Export(from, to, options); err != nil {
		t.Fatal(err)
	}
	return job.Dir(to)
}

// verified checks an export with no keyring, which names no signer.
func verified(t *testing.T, dir job.Dir) audit.Report {
	t.Helper()
	return verifiedWith(t, dir, signing.Keyring{})
}

func verifiedWith(t *testing.T, dir job.Dir, keyring signing.Keyring) audit.Report {
	t.Helper()
	report, err := audit.Verify(dir, keyring)
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func summary(t *testing.T, dir job.Dir) map[string]any {
	t.Helper()
	var record map[string]any
	if err := json.Unmarshal([]byte(read(t, filepath.Join(dir.Path(), audit.SummaryFile))), &record); err != nil {
		t.Fatal(err)
	}
	return record
}

// The summary is only worth something if it validates against the schema a
// stranger reads, so the test validates the bytes the export wrote.
func matchesTheSchema(t *testing.T, dir job.Dir) {
	t.Helper()
	published, err := os.ReadFile("../../schema/audit/v2.json")
	if err != nil {
		t.Fatal(err)
	}
	written := read(t, filepath.Join(dir.Path(), audit.SummaryFile))
	if err := schema.Validate(published, []byte(written)); err != nil {
		t.Fatalf("audit.json does not match its schema:\n%v\n%s", err, written)
	}
}

func TestAnExportVerifiesAndSaysWhatItDoesNotKnow(t *testing.T) {
	export := exported(t, ran(t), audit.Options{})
	report := verified(t, export)
	if len(report.Problems) != 0 || len(report.Redacted) != 0 {
		t.Fatalf("a fresh export should verify with nothing redacted: %+v", report)
	}
	matchesTheSchema(t, export)
	record := summary(t, export)
	for _, tc := range []struct {
		path []string
		want any
	}{
		{[]string{"asked", "who"}, audit.NotRecorded},
		{[]string{"asked", "how"}, "the shared sign-in"},
		{[]string{"approval", "by"}, audit.NotRecorded},
		{[]string{"requests"}, audit.NotRecorded},
		{[]string{"spend", "tokens"}, audit.NotRecorded},
		{[]string{"model", "asked"}, "the harness's default"},
		{[]string{"outcome", "state"}, "passed"},
		{[]string{"ran", "harness"}, "fixture"},
		{[]string{"ran", "session"}, audit.None},
		{[]string{"evidence", "signer"}, evidence.Unsigned},
	} {
		if got := at(record, tc.path...); got != tc.want {
			t.Errorf("%s: got %#v, want %#v", strings.Join(tc.path, "."), got, tc.want)
		}
	}
	if files := at(record, "changed", "files"); !sameStrings(files, "answer.txt") {
		t.Errorf("changed.files: got %#v", files)
	}
	if credentials := at(record, "model", "credentials"); !sameStrings(credentials, "the platform credential") {
		t.Errorf("model.credentials: got %#v", credentials)
	}
}

// The manifest check is what an export is held to, so the file the check
// names is the file a reader is told about.
func TestATamperedExportFailsNamingTheFile(t *testing.T) {
	for _, tc := range []struct {
		file, text, named string
	}{
		{"out/transcript.log", "nothing happened\n", "transcript.log changed"},
		{"out/diff.patch", "", "diff.patch changed"},
		{"state.json", `{"state": "failed"}`, "state.json changed since the export"},
		{"out/smuggled.log", "added later", "smuggled.log is not in the manifest"},
	} {
		export := exported(t, ran(t), audit.Options{})
		write(t, filepath.Join(export.Path(), tc.file), tc.text)
		if problems := verified(t, export).Problems; !mentions(problems, tc.named) {
			t.Errorf("after changing %s, want %q among %v", tc.file, tc.named, problems)
		}
	}
}

// Deleting a file and claiming nothing about it is tampering, not redaction.
func TestAFileRemovedWithoutARedactionFails(t *testing.T) {
	export := exported(t, ran(t), audit.Options{})
	if err := os.Remove(export.OutFile("transcript.log")); err != nil {
		t.Fatal(err)
	}
	if problems := verified(t, export).Problems; !mentions(problems, "transcript.log is missing") {
		t.Fatalf("want transcript.log reported missing, got %v", problems)
	}
}

// A summary edited by hand disagrees with the files it claims to describe.
func TestASummaryThatSaysMoreThanItsFilesFails(t *testing.T) {
	export := exported(t, ran(t), audit.Options{})
	file := filepath.Join(export.Path(), audit.SummaryFile)
	write(t, file, strings.Replace(read(t, file), `"by": "not recorded"`, `"by": "somebody senior"`, 1))
	if problems := verified(t, export).Problems; !mentions(problems, "audit.json: approval") {
		t.Fatalf("want the approval section named, got %v", problems)
	}
}

func TestAJobThatDoesNotVerifyIsNotExported(t *testing.T) {
	from := ran(t)
	write(t, from.OutFile("gate.log"), "a different verdict")
	_, err := audit.Export(from, filepath.Join(t.TempDir(), "export"), audit.Options{Now: func() int64 { return 0 }})
	if err == nil || !strings.Contains(err.Error(), "gate.log changed") {
		t.Fatalf("want the export refused naming gate.log, got %v", err)
	}
}

func TestAnExportNeverWritesOverAnother(t *testing.T) {
	from := ran(t)
	to := t.TempDir()
	if _, err := audit.Export(from, to, audit.Options{Now: func() int64 { return 0 }}); err == nil {
		t.Fatal("an existing directory should be refused")
	}
}

func at(value any, path ...string) any {
	for _, name := range path {
		object, _ := value.(map[string]any)
		value = object[name]
	}
	return value
}

func sameStrings(value any, want ...string) bool {
	list, _ := value.([]any)
	if len(list) != len(want) {
		return false
	}
	for i := range list {
		if list[i] != want[i] {
			return false
		}
	}
	return true
}

func mentions(problems []string, want string) bool {
	for _, problem := range problems {
		if strings.Contains(problem, want) {
			return true
		}
	}
	return false
}

// A request log that cannot be read stops the export after job files were
// copied, and nothing of it is left behind.
func TestAFailedExportLeavesNothing(t *testing.T) {
	to := filepath.Join(t.TempDir(), "export")
	options := audit.Options{RequestLog: filepath.Join(t.TempDir(), "absent.log"), Now: func() int64 { return 0 }}
	if _, err := audit.Export(ran(t), to, options); err == nil {
		t.Fatal("an unreadable request log should fail the export")
	}
	if _, err := os.Stat(to); !os.IsNotExist(err) {
		t.Fatalf("the half-written export is still there: %v", err)
	}
}
