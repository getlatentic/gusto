package matrix

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/getlatentic/gusto/internal/compare"
	"github.com/getlatentic/gusto/internal/job"
)

// TableVersion is the version of the table's JSON this code writes and the
// newest the catalog reads.
const TableVersion = 1

// Table is the benchmark as a person, or the catalog, reads it: a row for every
// task and pair, each read off its run's evidence.
type Table struct {
	Version int `json:"version"`
	// Benchmark is where the runs are, so every row's run can be found and
	// verified.
	Matrix      string `json:"matrix"`
	Environment string `json:"environment"`
	// Simplest is the task a pair has to finish to be offered.
	Simplest string `json:"simplest"`
	Rows     []Row  `json:"rows"`
}

// Row is one task on one pair. Every number in it is null when the run did not
// leave it, because a zero would read as free, or instant, or failed.
type Row struct {
	Job     string `json:"job"`
	Harness string `json:"harness"`
	// Model is what the run asked for, read off its job.json. Empty is a run
	// that left the model to the harness.
	Model       string `json:"model"`
	Environment string `json:"environment"`
	// Finished is the check's verdict. Null is a run that left none.
	Finished        *bool    `json:"finished"`
	GateCode        *int     `json:"gateCode"`
	StoppedAtBudget bool     `json:"stoppedAtBudget"`
	WallSeconds     *float64 `json:"wallSeconds"`
	Tokens          *int64   `json:"tokens"`
	CostUSD         *float64 `json:"costUsd"`
	UnpricedCalls   int      `json:"unpricedCalls"`
	CostFrom        string   `json:"costFrom,omitempty"`
	// Run is the run's directory, relative to the matrix.
	Run string `json:"run"`
	// Record is "checks", the first thing that changed since the run, or
	// why there is no record to check.
	Evidence string `json:"evidence"`
}

// Tabulate reads the benchmark at where back from its runs' evidence.
func Tabulate(where string) (Table, error) {
	record, err := Read(where)
	if err != nil {
		return Table{}, err
	}
	absolute, err := filepath.Abs(where)
	if err != nil {
		return Table{}, err
	}
	table := Table{Version: TableVersion, Matrix: absolute, Environment: record.Environment, Simplest: record.Simplest}
	for _, cell := range record.Cells {
		row, err := rowOf(where, record.Environment, cell)
		if err != nil {
			return Table{}, err
		}
		table.Rows = append(table.Rows, row)
	}
	return table, nil
}

// rowOf reads one cell through compare, the way `gusto compare` reads a side, so
// a matrix and a comparison cannot disagree about a run.
func rowOf(where, environment string, cell Cell) (Row, error) {
	row := Row{Job: cell.Job, Harness: cell.Harness, Model: cell.Model, Environment: environment, Run: cell.Dir}
	if spec, err := job.Read(cell.Run(where)); err == nil {
		row.Model = spec.Model
	}
	side := sideOf(where, cell)
	row.Evidence = checked(side)
	if side.Err != nil || side.Manifest.Gate == nil {
		return row, nil
	}
	passed, code := side.Manifest.Gate.Passed, side.Manifest.Gate.Code
	row.Finished, row.GateCode, row.StoppedAtBudget = &passed, &code, side.Manifest.StoppedAtBudget
	row.WallSeconds = cell.WallSeconds
	cost, err := spentBy(cell.Run(where), side.Manifest)
	if err != nil {
		return Row{}, err
	}
	row.CostUSD, row.Tokens, row.UnpricedCalls, row.CostFrom = cost.USD, cost.Tokens, cost.Unpriced, cost.From
	return row, nil
}

func sideOf(where string, cell Cell) compare.Side {
	run := compare.Run{Harness: cell.Harness, Dir: filepath.Base(cell.Dir)}
	return compare.Sides(cell.Comparison(where), compare.Record{Runs: []compare.Run{run}})[0]
}

func checked(side compare.Side) string {
	switch {
	case errors.Is(side.Err, os.ErrNotExist):
		return "not run yet"
	case side.Err != nil:
		return "none: " + side.Err.Error()
	case side.Manifest.Gate == nil:
		return "no verdict"
	case len(side.Wrong) > 0:
		return side.Wrong[0]
	}
	return "checks"
}

// ExitCode is 0 when every run left a verdict and its record still checks,
// whichever pairs finished, and 2 otherwise. Which pairs finish is what the
// table says, not a failure of the matrix.
func ExitCode(table Table) int {
	for _, row := range table.Rows {
		if row.Finished == nil || row.Evidence != "checks" {
			return 2
		}
	}
	return 0
}
