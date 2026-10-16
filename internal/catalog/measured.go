package catalog

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/getlatentic/gusto/internal/job"
)

// MatrixSetting names the table a matrix wrote, table.json, for the catalog to
// read when the server starts. Without it the catalog says nothing about
// models, which is what it said before a matrix existed.
const MatrixSetting = "GUSTO_MATRIX"

// matrixVersion is the newest table version this catalog reads.
const matrixVersion = 1

// Measured is one run of a harness and model on the simplest job a matrix
// measured: the job a pair has to finish before it is worth offering.
type Measured struct {
	Model       string
	Environment string
	Job         string
	Finished    bool
	// WallSeconds and CostUSD are what the run took and what its calls cost,
	// nil when the run did not leave them.
	WallSeconds *float64
	CostUSD     *float64
	// Run is the run's directory, so the claim can be checked.
	Run string
}

// CannotFinish says why the harness should not be offered with this model: it
// was measured on the simplest job and finished it nowhere. A model nobody
// measured is not marked, because not knowing is not the same as failing.
func (e Entry) CannotFinish(model string) (string, bool) {
	var failed []Measured
	for _, run := range e.Measured {
		if run.Model != model {
			continue
		}
		if run.Finished {
			return "", false
		}
		failed = append(failed, run)
	}
	if len(failed) == 0 {
		return "", false
	}
	first := failed[0]
	return fmt.Sprintf("%s with %s did not finish %s, the simplest job measured, %s; the run is %s",
		e.Name, modelName(model), first.Job, first.Environment, first.Run), true
}

// Unfinished is every model this harness was measured failing the simplest job
// with, in the order the matrix ran them.
func (e Entry) Unfinished() []string {
	var models []string
	seen := map[string]bool{}
	for _, run := range e.Measured {
		if _, cannot := e.CannotFinish(run.Model); cannot && !seen[run.Model] {
			models = append(models, run.Model)
			seen[run.Model] = true
		}
	}
	return models
}

// WithMatrix adds what the table at path measured on its simplest job to each
// entry. An empty path leaves the entries as they are.
func WithMatrix(entries []Entry, path string) ([]Entry, error) {
	if path == "" {
		return entries, nil
	}
	runs, err := simplestRuns(path)
	if err != nil {
		return entries, err
	}
	for i := range entries {
		entries[i].Measured = runs[entries[i].Name]
	}
	return entries, nil
}

// measuredFromSetting is WithMatrix on the table the setting names. A table
// that cannot be read is said and passed over: the catalog then offers what
// it offered before, rather than a server refusing to start over a report.
func measuredFromSetting(entries []Entry) []Entry {
	path := os.Getenv(MatrixSetting)
	marked, err := WithMatrix(entries, path)
	if err != nil {
		slog.Warn("the matrix was not read, so no model is marked", "file", path, "error", err)
	}
	return marked
}

// table is the part of the matrix's table.json the catalog reads.
type table struct {
	Version  int    `json:"version"`
	Matrix   string `json:"matrix"`
	Simplest string `json:"simplest"`
	Rows     []struct {
		Job         string   `json:"job"`
		Harness     string   `json:"harness"`
		Model       string   `json:"model"`
		Environment string   `json:"environment"`
		Finished    *bool    `json:"finished"`
		WallSeconds *float64 `json:"wallSeconds"`
		CostUSD     *float64 `json:"costUsd"`
		Run         string   `json:"run"`
	} `json:"rows"`
}

// simplestRuns is every run on the simplest job that left a verdict, by
// harness. A run with no verdict measured nothing, and marks nothing.
func simplestRuns(path string) (map[string][]Measured, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var read table
	if err := json.Unmarshal(text, &read); err != nil {
		return nil, fmt.Errorf("%s is not a matrix's table: %w", path, err)
	}
	if err := job.Readable(filepath.Base(path), read.Version, matrixVersion); err != nil {
		return nil, err
	}
	runs := map[string][]Measured{}
	for _, row := range read.Rows {
		if row.Job != read.Simplest || row.Finished == nil {
			continue
		}
		runs[row.Harness] = append(runs[row.Harness], Measured{
			Model: row.Model, Environment: row.Environment, Job: row.Job,
			Finished: *row.Finished, WallSeconds: row.WallSeconds, CostUSD: row.CostUSD,
			Run: filepath.Join(read.Matrix, row.Run),
		})
	}
	return runs, nil
}

func modelName(model string) string {
	if model == "" {
		return "its own default model"
	}
	return model
}
