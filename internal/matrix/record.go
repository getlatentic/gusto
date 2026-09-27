package matrix

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/getlatentic/haas/internal/job"
)

// RecordFile is the matrix's own file. It sits at the top of the matrix, above
// every comparison, so no run can read it.
const RecordFile = "matrix.json"

// RecordVersion is the version of matrix.json this code writes and the newest
// it reads.
const RecordVersion = 1

// Record is a matrix: where it ran, which job counts as the simplest, and a
// cell for every job and pair.
type Record struct {
	Version int `json:"version"`
	// Environment is where every run in this matrix happened: the plan's name
	// for the place, and how the runs were started there. A matrix holds one
	// environment, so two numbers in one row never come from two places.
	Environment string `json:"environment"`
	Simplest    string `json:"simplest"`
	Cells       []Cell `json:"cells"`
}

// Cell is one job run on one pair. Dir is relative to the matrix: the job's
// comparison, then the pair's run inside it.
type Cell struct {
	Job     string `json:"job"`
	Harness string `json:"harness"`
	Model   string `json:"model,omitempty"`
	Dir     string `json:"dir"`
	// StartedUnix and WallSeconds are measured around the run: the clone, the
	// container starting, the harness, and the gate. Null is a cell that has
	// not finished a run yet.
	StartedUnix int64    `json:"startedUnix,omitempty"`
	WallSeconds *float64 `json:"wallSeconds"`
}

// Pair is the harness and model the cell ran.
func (c Cell) Pair() Pair { return Pair{Harness: c.Harness, Model: c.Model} }

// Run is the cell's job directory in the matrix at where.
func (c Cell) Run(where string) job.Dir {
	return job.Dir(filepath.Join(where, c.Dir))
}

// Comparison is the directory of the comparison the cell is a run of.
func (c Cell) Comparison(where string) string {
	return filepath.Join(where, c.Job)
}

// Read the matrix at where.
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

// Is says whether where holds a matrix.
func Is(where string) bool {
	_, err := os.Stat(filepath.Join(where, RecordFile))
	return err == nil
}

// write keeps the record through a file beside it, so a matrix stopped midway
// never leaves half a record, and the cells that did run keep their times.
func write(where string, record Record) error {
	record.Version = RecordVersion
	text, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(where, RecordFile)
	if err := os.WriteFile(path+".new", append(text, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(path+".new", path)
}

// opened is the matrix at where, or a new one for environment when there is
// none yet. A matrix that ran somewhere else is refused rather than extended,
// because its rows would then mix two places.
func opened(where, environment string) (Record, error) {
	record, err := Read(where)
	if errors.Is(err, os.ErrNotExist) {
		return Record{Environment: environment}, os.MkdirAll(where, 0o755)
	}
	if err != nil {
		return Record{}, err
	}
	if record.Environment != environment {
		return Record{}, &elsewhere{where: where, ran: record.Environment, now: environment}
	}
	return record, nil
}

type elsewhere struct{ where, ran, now string }

func (e *elsewhere) Error() string {
	return "the matrix in " + e.where + " ran in " + e.ran + ", not " + e.now +
		": measure this environment into a matrix of its own with -into"
}

// cell finds the record's cell for dir, adding one when the plan names a pair
// the matrix has not held before.
func (r *Record) cell(jobName string, pair Pair) *Cell {
	dir := filepath.Join(jobName, pair.Dir())
	for i := range r.Cells {
		if r.Cells[i].Dir == dir {
			return &r.Cells[i]
		}
	}
	r.Cells = append(r.Cells, Cell{Job: jobName, Harness: pair.Harness, Model: pair.Model, Dir: dir})
	return &r.Cells[len(r.Cells)-1]
}
