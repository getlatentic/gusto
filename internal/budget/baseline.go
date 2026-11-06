package budget

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Baseline is the functions already over the budget, each held to what it measured when it was written down, so none grows
// and the list only shrinks. A line is "lines complexity name", and anything after a # on it is the reason it is over.
type Baseline map[string]Entry

// Entry is one function of a baseline, and why it is over the budget when somebody said.
type Entry struct {
	Func
	Reason string
}

// ReadBaseline reads a baseline as Write writes it. Blank lines and lines that start with # are left out.
func ReadBaseline(r io.Reader) (Baseline, error) {
	baseline := Baseline{}
	scanner := bufio.NewScanner(r)
	for line := 1; scanner.Scan(); line++ {
		text, reason, _ := strings.Cut(scanner.Text(), "#")
		fields := strings.Fields(text)
		if len(fields) == 0 {
			continue
		}
		f, err := parseLine(fields)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		baseline[f.Name] = Entry{Func: f, Reason: strings.TrimSpace(reason)}
	}
	return baseline, scanner.Err()
}

func parseLine(fields []string) (Func, error) {
	if len(fields) != 3 {
		return Func{}, fmt.Errorf("%q is not \"lines complexity name\"", strings.Join(fields, " "))
	}
	lines, err := strconv.Atoi(fields[0])
	if err != nil {
		return Func{}, err
	}
	complexity, err := strconv.Atoi(fields[1])
	if err != nil {
		return Func{}, err
	}
	return Func{Name: fields[2], Lines: lines, Complexity: complexity}, nil
}

// Write writes every function of funcs that is over the budget, by name, under the header, with the reason the baseline gave
// it.
func (b Baseline) Write(w io.Writer, header string, funcs []Func) error {
	var over []Func
	for _, f := range funcs {
		if f.Over() {
			over = append(over, f)
		}
	}
	sort.Slice(over, func(i, j int) bool { return over[i].Name < over[j].Name })
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}
	for _, f := range over {
		line := fmt.Sprintf("%d %d %s", f.Lines, f.Complexity, f.Name)
		if reason := b[f.Name].Reason; reason != "" {
			line += " # " + reason
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

// Check is what is wrong with funcs against the baseline: a function over the budget that is not in it, one in it that grew,
// shrank or now fits, and one in it that is gone.
func (b Baseline) Check(funcs []Func) []string {
	var problems []string
	seen := map[string]bool{}
	for _, f := range funcs {
		seen[f.Name] = true
		if problem := b.checkOne(f); problem != "" {
			problems = append(problems, problem)
		}
	}
	for name := range b {
		if !seen[name] {
			problems = append(problems, name+" is in the baseline and is gone: take it out")
		}
	}
	sort.Strings(problems)
	return problems
}

func (b Baseline) checkOne(f Func) string {
	was, listed := b[f.Name]
	switch {
	case !listed && f.Over():
		return fmt.Sprintf("%s is %d lines and complexity %d, over the budget of %d and %d: split it", f.Name, f.Lines, f.Complexity, MaxLines, MaxComplexity)
	case !listed:
		return ""
	case !f.Over():
		return f.Name + " is within the budget now: take it out of the baseline"
	case f.Lines > was.Lines || f.Complexity > was.Complexity:
		return fmt.Sprintf("%s grew from %d lines and complexity %d to %d and %d", f.Name, was.Lines, was.Complexity, f.Lines, f.Complexity)
	case f.Lines < was.Lines || f.Complexity < was.Complexity:
		return fmt.Sprintf("%s shrank to %d lines and complexity %d: write that in the baseline", f.Name, f.Lines, f.Complexity)
	}
	return ""
}
