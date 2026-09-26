package audit

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
)

// Options is what shapes one export besides the job itself.
type Options struct {
	Policy Policy
	// RequestLog is the server's request log, one JSON line a request, as
	// journald keeps it. Empty means none was given, and the export says the
	// requests are not recorded rather than that there were none.
	RequestLog string
	// Now stamps exportedAt.
	Now func() int64
}

// Export writes the audit export of the job in from into a new directory to.
//
// A job whose evidence does not verify is refused, naming what is wrong: an
// export vouches for what it carries, and it cannot vouch for a run whose own
// record already disagrees with its files.
func Export(from job.Dir, to string, options Options) (Record, error) {
	if err := options.Policy.check(); err != nil {
		return Record{}, err
	}
	manifest, err := verified(from)
	if err != nil {
		return Record{}, err
	}
	if err := fresh(to); err != nil {
		return Record{}, err
	}
	record, err := write(from, job.Dir(to), manifest, options)
	if err != nil {
		// Half an export would pass for a whole one with files missing, and
		// would stand in the way of the next attempt.
		os.RemoveAll(to)
	}
	return record, err
}

func write(from, to job.Dir, manifest evidence.Manifest, options Options) (Record, error) {
	id := filepath.Base(filepath.Clean(from.Path()))
	redactions, err := carryAll(from, to, id, manifest, options)
	if err != nil {
		return Record{}, err
	}
	record, err := Summarize(to, id, redactions, options.Policy)
	if err != nil {
		return Record{}, err
	}
	record.ExportedAt = options.Now()
	return record, writeSummary(to.Path(), record)
}

func verified(from job.Dir) (evidence.Manifest, error) {
	wrong, err := evidence.Check(from)
	if err != nil {
		return evidence.Manifest{}, fmt.Errorf("%s has no evidence to export: %w", from.Path(), err)
	}
	if len(wrong) > 0 {
		return evidence.Manifest{}, fmt.Errorf("%s does not verify, so it is not exported: %s", from.Path(), strings.Join(wrong, "; "))
	}
	return evidence.Read(from)
}

// fresh makes the export's directory, refusing one that is already there, so
// an export never mixes with the files of another.
func fresh(to string) error {
	if _, err := os.Stat(to); !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%s is already there; an export goes into a new directory", to)
	}
	return os.MkdirAll(filepath.Join(to, "out"), 0o755)
}

// carryAll copies the job's files through the policy, then the request lines
// that name the job, and returns what the policy changed.
func carryAll(from, to job.Dir, id string, manifest evidence.Manifest, options Options) ([]Redaction, error) {
	names, err := sourceFiles(from)
	if err != nil {
		return nil, err
	}
	carrier := carrier{to: to, policy: options.Policy, scrub: scrubberFor(options.Policy.Redact), manifest: manifest}
	for _, name := range names {
		text, err := os.ReadFile(filepath.Join(from.Path(), filepath.FromSlash(name)))
		if err != nil {
			return nil, err
		}
		if err := carrier.carry(name, text); err != nil {
			return nil, err
		}
	}
	if options.RequestLog == "" {
		return carrier.redactions, nil
	}
	requests, err := requestsNaming(options.RequestLog, id)
	if err != nil {
		return nil, err
	}
	return carrier.redactions, carrier.carry(RequestsFile, requests)
}

// sourceFiles are the files of a job an export carries: job.json, the
// server's state.json when there is one, and everything in out/. The
// workspace stays behind: what changed in it is out/diff.patch.
func sourceFiles(from job.Dir) ([]string, error) {
	names, err := filesIn(from)
	if err != nil {
		return nil, err
	}
	var carried []string
	for _, name := range names {
		if _, inOut := cutOut(name); inOut || name == SpecFile || name == StateFile {
			carried = append(carried, name)
		}
	}
	return carried, nil
}

// carrier writes files into the export, applying the policy on the way.
type carrier struct {
	to         job.Dir
	policy     Policy
	scrub      *scrubber
	manifest   evidence.Manifest
	redactions []Redaction
}

func (c *carrier) carry(name string, text []byte) error {
	if rule, dropped := c.policy.drops(name); dropped {
		c.note(name, Dropped, []string{rule})
		return nil
	}
	if name != ManifestFile {
		var rules []string
		if text, rules = c.scrub.file(name, text); len(rules) > 0 {
			c.note(name, Rewritten, rules)
		}
	}
	return os.WriteFile(filepath.Join(c.to.Path(), filepath.FromSlash(name)), text, 0o644)
}

func (c *carrier) note(name, action string, rules []string) {
	original, named := originalOf(c.manifest, name)
	if !named {
		original = NotRecorded
	}
	c.redactions = append(c.redactions, Redaction{File: name, Action: action, Rules: rules, Original: original})
}

func writeSummary(to string, record Record) error {
	text, err := encode(record, true)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(to, SummaryFile), text, 0o644)
}
