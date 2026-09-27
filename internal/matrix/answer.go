package matrix

import (
	"io"
	"path/filepath"
	"strings"
)

// Asked is what `haas matrix` was asked on the command line.
type Asked struct {
	// Into is where the matrix goes. Empty is beside the plan, named after it.
	Into string
	// Again names the pairs to run again although they finished: "all", or
	// harness@model separated by commas.
	Again string
	// JSON prints the table as the catalog reads it, rather than as markdown.
	JSON bool
	Way  Way
	// Progress is where each run is announced as it starts.
	Progress io.Writer
}

// Answer is the whole of `haas matrix`. A matrix's directory alone prints its
// table again from the evidence; a plan runs whatever is due into its matrix,
// writes the table there, and prints it. Either way the exit code is
// ExitCode's.
func Answer(w io.Writer, named string, asked Asked) (int, error) {
	table, err := answered(named, asked)
	if err != nil {
		return 0, err
	}
	render := Markdown
	if asked.JSON {
		render = JSON
	}
	return ExitCode(table), render(w, table)
}

// answered is the table of the matrix named, run and saved first when what was
// named is a plan. A matrix only read is left as it was.
func answered(named string, asked Asked) (Table, error) {
	if Is(named) {
		return Tabulate(named)
	}
	where, err := measured(named, asked)
	if err != nil {
		return Table{}, err
	}
	table, err := Tabulate(where)
	if err != nil {
		return Table{}, err
	}
	return table, Save(table)
}

// measured runs the plan at path into its matrix and says where that is.
func measured(path string, asked Asked) (string, error) {
	plan, err := ReadPlan(path)
	if err != nil {
		return "", err
	}
	where := asked.Into
	if where == "" {
		where = Beside(path)
	}
	_, err = Measure(plan, where, asked.Way, AgainFor(asked.Again), asked.Progress)
	return where, err
}

// Beside is where a plan's matrix goes when nothing says: next to the plan,
// named after it, so running the same plan again finds the same matrix.
func Beside(plan string) string {
	base := strings.TrimSuffix(filepath.Base(plan), filepath.Ext(plan))
	return filepath.Join(filepath.Dir(plan), base+"-matrix")
}
