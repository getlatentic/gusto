package audit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/signing"
)

// Report is what Verify found. Problems empty means the export holds.
type Report struct {
	// Covered counts the files whose bytes the run's own manifest vouches for.
	Covered int
	// Redacted names each file the policy changed, which can only be matched
	// to the run by somebody holding the original.
	Redacted []string
	Problems []string
}

// stored is the part of audit.json that Verify needs typed; the rest it
// compares as written.
type stored struct {
	Version    int         `json:"version"`
	Job        string      `json:"job"`
	Carried    []Carried   `json:"carried"`
	Redactions []Redaction `json:"redactions"`
	Policy     Policy      `json:"policy"`
	Evidence   Evidence    `json:"evidence"`
}

// Verify checks an export in three steps, and names the file behind every
// problem it finds.
//
//  1. The manifest check, through evidence.Check, so whatever that check comes
//     to hold, such as a signature, an export is held to as well. A redacted
//     file is expected to differ, and only in the way its redaction says.
//  2. Every file against the hash the export recorded, which covers what the
//     manifest does not: the server's state, the request lines, and the
//     redacted copies.
//  3. audit.json against a summary derived again from those files, with the
//     signer named by the verifier's own keyring.
func Verify(dir job.Dir, keyring signing.Keyring) (Report, error) {
	written, record, err := readSummary(dir)
	if err != nil {
		return Report{}, err
	}
	manifest, err := evidence.Read(dir)
	if err != nil {
		return Report{}, err
	}
	complaints, err := evidence.Check(dir)
	if err != nil {
		return Report{}, err
	}
	report := Report{Redacted: redactedNames(record.Redactions)}
	report.Problems = append(report.Problems, unexcused(complaints, record.Redactions)...)
	report.Problems = append(report.Problems, falseOriginals(manifest, record.Redactions)...)
	carried, err := carriedIn(dir, manifest)
	if err != nil {
		return Report{}, err
	}
	report.Problems = append(report.Problems, compareCarried(record.Carried, carried)...)
	derived, err := Summarize(dir, record.Job, record.Redactions, record.Policy, keyring)
	if err != nil {
		return Report{}, err
	}
	derived = derived.asWritten(record.Version, manifest)
	report.Problems = append(report.Problems, signerDisagreement(record.Evidence.Signer, derived.Evidence.Signer)...)
	// The signer was compared above, by name; the rest of evidence still is.
	derived.Evidence.Signer = record.Evidence.Signer
	report.Problems = append(report.Problems, disagreements(written, derived)...)
	report.Covered = covered(carried)
	return report, nil
}

func readSummary(dir job.Dir) (map[string]json.RawMessage, stored, error) {
	text, err := os.ReadFile(filepath.Join(dir.Path(), SummaryFile))
	if err != nil {
		return nil, stored{}, err
	}
	var record stored
	if err := json.Unmarshal(text, &record); err != nil {
		return nil, stored{}, fmt.Errorf("%s: %w", SummaryFile, err)
	}
	if err := job.Readable(SummaryFile, record.Version, Version); err != nil {
		return nil, stored{}, err
	}
	var written map[string]json.RawMessage
	err = json.Unmarshal(text, &written)
	return written, record, err
}

// unexcused drops the complaints a redaction accounts for. A dropped file is
// missing and a rewritten one changed; anything else about either file, and
// every complaint about any other, stands. The wording matched is the wording
// evidence.Check writes, and a test holds the two together.
func unexcused(complaints []string, redactions []Redaction) []string {
	expected := map[string]bool{}
	for _, redaction := range redactions {
		expected[expectedComplaint(redaction)] = true
	}
	var left []string
	for _, complaint := range complaints {
		if !expected[complaint] {
			left = append(left, complaint)
		}
	}
	return left
}

func expectedComplaint(redaction Redaction) string {
	name := path.Base(redaction.File)
	if redaction.Action == Dropped {
		return name + " is missing"
	}
	return name + " changed"
}

// falseOriginals refuses a redaction that points at a hash the run never
// recorded, which would tie the redacted file to somebody else's original.
func falseOriginals(manifest evidence.Manifest, redactions []Redaction) []string {
	var wrong []string
	for _, redaction := range redactions {
		original, named := originalOf(manifest, redaction.File)
		if !named {
			original = NotRecorded
		}
		if original != redaction.Original {
			wrong = append(wrong, fmt.Sprintf("%s: the redaction names an original the manifest does not hold", redaction.File))
		}
	}
	return wrong
}

func compareCarried(recorded, found []Carried) []string {
	now := map[string]string{}
	for _, file := range found {
		now[file.Name] = file.SHA256
	}
	was := map[string]bool{}
	var wrong []string
	for _, file := range recorded {
		was[file.Name] = true
		switch sum, ok := now[file.Name]; {
		case !ok:
			wrong = append(wrong, fmt.Sprintf("%s is missing from the export", file.Name))
		case sum != file.SHA256:
			wrong = append(wrong, fmt.Sprintf("%s changed since the export", file.Name))
		}
	}
	for _, file := range found {
		if !was[file.Name] {
			wrong = append(wrong, fmt.Sprintf("%s is not part of the export", file.Name))
		}
	}
	return wrong
}

// disagreements compares audit.json with the summary its files give, one
// section at a time, so the problem names the section. exportedAt is the one
// field no file holds, and carried was compared file by file already.
func disagreements(written map[string]json.RawMessage, derived Record) []string {
	text, err := json.Marshal(derived)
	if err != nil {
		return []string{fmt.Sprintf("%s: %v", SummaryFile, err)}
	}
	var again map[string]json.RawMessage
	if err := json.Unmarshal(text, &again); err != nil {
		return []string{fmt.Sprintf("%s: %v", SummaryFile, err)}
	}
	var wrong []string
	for _, section := range sections(written, again) {
		if section == "exportedAt" || section == "carried" {
			continue
		}
		if !sameJSON(written[section], again[section]) {
			wrong = append(wrong, fmt.Sprintf("%s: %s is not what the files in the export say", SummaryFile, section))
		}
	}
	return wrong
}

func sections(a, b map[string]json.RawMessage) []string {
	seen := map[string]bool{}
	for name := range a {
		seen[name] = true
	}
	for name := range b {
		seen[name] = true
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// sameJSON compares two values by what they say rather than how they are
// spaced, by decoding both and writing them again in one canonical form.
func sameJSON(a, b json.RawMessage) bool {
	return bytes.Equal(canonical(a), canonical(b))
}

func canonical(text json.RawMessage) []byte {
	if text == nil {
		return nil
	}
	reader := json.NewDecoder(bytes.NewReader(text))
	reader.UseNumber()
	var value any
	if err := reader.Decode(&value); err != nil {
		return text
	}
	written, _ := json.Marshal(value)
	return written
}

func redactedNames(redactions []Redaction) []string {
	names := []string{}
	for _, redaction := range redactions {
		names = append(names, redaction.File+" ("+redaction.Action+")")
	}
	return names
}

func covered(carried []Carried) int {
	count := 0
	for _, file := range carried {
		if file.Covered {
			count++
		}
	}
	return count
}
