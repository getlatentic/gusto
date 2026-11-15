package budget

import (
	"bytes"
	"flag"
	"os"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "write the functions over the budget into baseline.txt")

const baselineFile = "baseline.txt"

const header = `# The functions over the budget of 50 lines and complexity 10 when it was first checked, each held to what it measured.
# A function here may shrink and not grow. Bring one within the budget and take it out; a new one is split, not added.
# A line is "lines complexity name", and a # after it says why the function is over. Rewrite it, keeping the reasons, with:
#   go test ./internal/budget -update
`

// No function grows past the budget: a new one over it is split, and one already over it shrinks or stays as it is.
func TestNoFunctionGrowsPastTheBudget(t *testing.T) {
	funcs, err := Measure("../..")
	if err != nil {
		t.Fatal(err)
	}
	if dup := duplicated(funcs); dup != "" {
		t.Fatalf("two functions are both called %s", dup)
	}
	text, err := os.ReadFile(baselineFile)
	if err != nil && !(*update && os.IsNotExist(err)) {
		t.Fatal(err)
	}
	baseline, err := ReadBaseline(bytes.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		var written bytes.Buffer
		if err := baseline.Write(&written, header, funcs); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(baselineFile, written.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if problems := baseline.Check(funcs); len(problems) > 0 {
		t.Errorf("%d functions are not as the budget says:\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
}

func duplicated(funcs []Func) string {
	seen := map[string]bool{}
	for _, f := range funcs {
		if seen[f.Name] && !strings.HasSuffix(f.Name, ":init") {
			return f.Name
		}
		seen[f.Name] = true
	}
	return ""
}

// The check finds a new function over the budget, one that grew, one that shrank or now fits, and one that is gone.
func TestTheCheckSaysWhatChanged(t *testing.T) {
	baseline := Baseline{
		"a.go:grew":   {Func: Func{Name: "a.go:grew", Lines: 60, Complexity: 5}},
		"a.go:shrank": {Func: Func{Name: "a.go:shrank", Lines: 60, Complexity: 5}},
		"a.go:fits":   {Func: Func{Name: "a.go:fits", Lines: 60, Complexity: 5}},
		"a.go:same":   {Func: Func{Name: "a.go:same", Lines: 60, Complexity: 5}},
		"a.go:gone":   {Func: Func{Name: "a.go:gone", Lines: 60, Complexity: 5}},
	}
	problems := baseline.Check([]Func{
		{Name: "a.go:new", Lines: 10, Complexity: 11},
		{Name: "a.go:grew", Lines: 61, Complexity: 5},
		{Name: "a.go:shrank", Lines: 55, Complexity: 5},
		{Name: "a.go:fits", Lines: 50, Complexity: 10},
		{Name: "a.go:same", Lines: 60, Complexity: 5},
		{Name: "a.go:small", Lines: 5, Complexity: 1},
	})
	joined := strings.Join(problems, "\n")
	for _, want := range []string{"a.go:new is 10 lines and complexity 11", "a.go:grew grew", "a.go:shrank shrank", "a.go:fits is within", "a.go:gone is in the baseline and is gone"} {
		if !strings.Contains(joined, want) {
			t.Errorf("%q is not in:\n%s", want, joined)
		}
	}
	if len(problems) != 5 {
		t.Errorf("want 5 problems, got %d:\n%s", len(problems), joined)
	}
}

// A baseline keeps the reason a function is over when it is written again.
func TestABaselineKeepsItsReasons(t *testing.T) {
	baseline, err := ReadBaseline(strings.NewReader("# a header\n\n70 12 a.go:long # a table of cases\n"))
	if err != nil {
		t.Fatal(err)
	}
	var written bytes.Buffer
	if err := baseline.Write(&written, "", []Func{{Name: "a.go:long", Lines: 70, Complexity: 12}, {Name: "a.go:small", Lines: 3, Complexity: 1}}); err != nil {
		t.Fatal(err)
	}
	if got := written.String(); got != "70 12 a.go:long # a table of cases\n" {
		t.Errorf("got %q", got)
	}
}
