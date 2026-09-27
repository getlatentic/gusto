package catalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A table as a matrix writes it: opencode measured on the simplest job with
// two models on a laptop, one of them also on the box, and a harder job the
// catalog does not judge by.
const measuredTable = `{
  "version": 1,
  "matrix": "/runs/laptop-matrix",
  "simplest": "one-line-fix",
  "rows": [
    {"job": "one-line-fix", "harness": "opencode", "model": "ollama/gpt-oss:20b", "environment": "laptop", "finished": false, "run": "one-line-fix/opencode@ollama-gpt-oss-20b"},
    {"job": "one-line-fix", "harness": "opencode", "model": "ollama/llama3.2:3b", "environment": "laptop", "finished": false, "run": "one-line-fix/opencode@ollama-llama3.2-3b"},
    {"job": "one-line-fix", "harness": "opencode", "model": "ollama/llama3.2:3b", "environment": "box", "finished": true, "run": "one-line-fix/opencode@ollama-llama3.2-3b"},
    {"job": "one-line-fix", "harness": "opencode", "model": "never/ran", "environment": "laptop", "finished": null, "run": "one-line-fix/opencode@never-ran"},
    {"job": "rename-a-module", "harness": "opencode", "model": "big/model", "environment": "laptop", "finished": false, "run": "rename-a-module/opencode@big-model"}
  ]
}`

func tableAt(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "table.json")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAPairThatCannotFinishTheSimplestJobIsMarkedWithWhy(t *testing.T) {
	t.Setenv(MatrixSetting, tableAt(t, measuredTable))
	opencode := entryNamed(t, Survey("", "", []string{"opencode"}), "opencode")

	why, cannot := opencode.CannotFinish("ollama/gpt-oss:20b")
	if !cannot {
		t.Fatal("gpt-oss:20b never finished the simplest job and is not marked")
	}
	for _, want := range []string{"opencode", "ollama/gpt-oss:20b", "one-line-fix", "laptop", "/runs/laptop-matrix/one-line-fix/opencode@ollama-gpt-oss-20b"} {
		if !strings.Contains(why, want) {
			t.Errorf("the mark should say %q, and says %q", want, why)
		}
	}
	if !opencode.Ready {
		t.Error("one model failing is no reason to withhold the harness")
	}
	if got := opencode.Unfinished(); len(got) != 1 || got[0] != "ollama/gpt-oss:20b" {
		t.Errorf("only gpt-oss:20b should be marked, got %v", got)
	}
}

// Finishing somewhere is finishing, a run with no verdict measured nothing, and
// a harder job failing says nothing about the simplest one.
func TestOnlyAMeasuredFailureOnTheSimplestJobMarksAPair(t *testing.T) {
	entries, err := WithMatrix([]Entry{{Name: "opencode"}}, tableAt(t, measuredTable))
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range []string{"ollama/llama3.2:3b", "never/ran", "big/model", "nobody/measured"} {
		if why, cannot := entries[0].CannotFinish(model); cannot {
			t.Errorf("%s should not be marked, and reads %q", model, why)
		}
	}
}

func TestWithNoMatrixTheCatalogIsWhatItWas(t *testing.T) {
	t.Setenv(MatrixSetting, "")
	for _, entry := range Survey("", "", []string{"claude", "codex", "opencode"}) {
		if len(entry.Measured) > 0 {
			t.Errorf("%s carries measurements nobody made", entry.Name)
		}
	}
}

// A table the catalog cannot read is passed over rather than stopping a server:
// the catalog then says what it said before.
func TestAMatrixThatCannotBeReadMarksNothing(t *testing.T) {
	for name, path := range map[string]string{
		"missing":  filepath.Join(t.TempDir(), "gone.json"),
		"not json": tableAt(t, "a table"),
		"too new":  tableAt(t, `{"version": 99, "simplest": "x", "rows": []}`),
	} {
		t.Setenv(MatrixSetting, path)
		opencode := entryNamed(t, Survey("", "", []string{"opencode"}), "opencode")
		if !opencode.Ready || len(opencode.Measured) > 0 {
			t.Errorf("a %s table should change nothing: %+v", name, opencode)
		}
		if _, err := WithMatrix(nil, path); err == nil {
			t.Errorf("a %s table should be refused by name", name)
		}
	}
}

func entryNamed(t *testing.T, entries []Entry, name string) Entry {
	t.Helper()
	for _, entry := range entries {
		if entry.Name == name {
			return entry
		}
	}
	t.Fatalf("%s is not in the catalog", name)
	return Entry{}
}
