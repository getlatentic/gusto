package compare

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/getlatentic/gusto/internal/job"
)

// RecordFile is the comparison's own file. It sits in the comparison's
// directory, above the runs, so no run can read it and learn what the other
// harness is or did.
const RecordFile = "comparison.json"

// RecordVersion is the version of comparison.json this code writes and the
// newest it reads.
const RecordVersion = 1

// Record names the runs of one comparison and what they share.
type Record struct {
	Version int `json:"version"`
	// Submission is the job the runs were made from, as an absolute path.
	Submission string `json:"submission"`
	// Commit is where every run's workspace starts, and PinnedBy the reference
	// it was resolved from.
	Commit   string `json:"commit"`
	PinnedBy string `json:"pinnedBy"`
	// Held names every job.json field the runs share. It is read off the files
	// rather than promised, so a field the submission left out, such as model,
	// is not claimed as held.
	Held []string `json:"held"`
	Runs []Run    `json:"runs"`
}

// Run is one side of a comparison. Dir is relative to the comparison, so the
// comparison can be moved or copied whole.
type Run struct {
	Harness string `json:"harness"`
	Dir     string `json:"dir"`
}

// Job is the run's directory under the comparison at where.
func (r Run) Job(where string) job.Dir {
	return job.Dir(filepath.Join(where, r.Dir))
}

func writeRecord(where string, record Record) error {
	record.Version = RecordVersion
	text, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(where, RecordFile), append(text, '\n'), 0o644)
}

// Read the record of the comparison at where.
func Read(where string) (Record, error) {
	text, err := os.ReadFile(filepath.Join(where, RecordFile))
	if err != nil {
		return Record{}, err
	}
	var record Record
	if err := json.Unmarshal(text, &record); err != nil {
		return Record{}, err
	}
	if err := job.Readable(RecordFile, record.Version, RecordVersion); err != nil {
		return Record{}, err
	}
	return record, nil
}

// Is says whether where holds a comparison.
func Is(where string) bool {
	_, err := os.Stat(filepath.Join(where, RecordFile))
	return err == nil
}
