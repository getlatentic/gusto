package matrix

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The table's two files, written into the matrix each time it runs. The
// markdown goes into a README as it is; the JSON is what the catalog reads.
const (
	MarkdownFile = "table.md"
	JSONFile     = "table.json"
)

// Markdown writes the table the way the README's own tables are written: one
// row a job and pair, and a word rather than a zero where a number is missing.
func Markdown(w io.Writer, table Table) error {
	fmt.Fprintf(w, "Measured %s. Every row is a run in `%s`, with its evidence kept there.\n\n",
		table.Environment, filepath.Base(table.Matrix))
	fmt.Fprintln(w, "| harness and model | job | finished | wall clock | billed tokens | cost | evidence |")
	fmt.Fprintln(w, "| --- | --- | --- | --- | --- | --- | --- |")
	for _, row := range table.Rows {
		if _, err := fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %s | %s |\n", pairOf(row), jobOf(row, table),
			finished(row), wallClock(row), tokens(row), cost(row), strings.ReplaceAll(row.Evidence, "|", `\|`)); err != nil {
			return err
		}
	}
	return nil
}

// JSON writes the table for the catalog, and for anybody else reading it.
func JSON(w io.Writer, table Table) error {
	text, err := json.MarshalIndent(table, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(text, '\n'))
	return err
}

// Save writes both renderings into the matrix.
func Save(table Table) error {
	for name, render := range map[string]func(io.Writer, Table) error{MarkdownFile: Markdown, JSONFile: JSON} {
		file, err := os.Create(filepath.Join(table.Matrix, name))
		if err != nil {
			return err
		}
		err = render(file, table)
		if closed := file.Close(); err == nil {
			err = closed
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func pairOf(row Row) string {
	if row.Model == "" {
		return "`" + row.Harness + "`, its own default model"
	}
	return "`" + row.Harness + "`, `" + row.Model + "`"
}

func jobOf(row Row, table Table) string {
	if row.Job == table.Simplest {
		return row.Job + " (simplest)"
	}
	return row.Job
}

func finished(row Row) string {
	switch {
	case row.Finished == nil:
		return "did not run"
	case *row.Finished:
		return "yes"
	case row.StoppedAtBudget:
		return "no, stopped at the budget"
	}
	return "no"
}

func wallClock(row Row) string {
	if row.WallSeconds == nil {
		return "unmeasured"
	}
	took := time.Duration(*row.WallSeconds * float64(time.Second)).Round(time.Second)
	if took < time.Second {
		return "under 1s"
	}
	return took.String()
}

func tokens(row Row) string {
	if row.Tokens == nil {
		return "unmeasured"
	}
	return grouped(*row.Tokens)
}

// cost says where the figure came from, and says a floor is a floor.
func cost(row Row) string {
	switch {
	case row.CostUSD == nil && row.UnpricedCalls > 0:
		return fmt.Sprintf("unmeasured, %d calls unpriced", row.UnpricedCalls)
	case row.CostUSD == nil:
		return "unmeasured"
	case row.UnpricedCalls > 0:
		return fmt.Sprintf("at least $%.4f (%s), %d calls unpriced", *row.CostUSD, row.CostFrom, row.UnpricedCalls)
	}
	return fmt.Sprintf("$%.4f (%s)", *row.CostUSD, row.CostFrom)
}

// grouped writes a count with thousands separated, as the README does.
func grouped(n int64) string {
	digits := strconv.FormatInt(n, 10)
	sign := ""
	if n < 0 {
		sign, digits = "-", digits[1:]
	}
	for i := len(digits) - 3; i > 0; i -= 3 {
		digits = digits[:i] + "," + digits[i:]
	}
	return sign + digits
}
