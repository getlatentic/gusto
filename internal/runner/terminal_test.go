package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/job"
)

// A terminal is typed into and drawn from, and both sides are recorded: a run
// somebody took over says what they typed as well as what it showed.
func TestATerminalIsDrawnTypedIntoAndRecorded(t *testing.T) {
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	var screen strings.Builder
	command := exec.Command("sh", "-c", `read answer; printf 'got %s, in %s columns\n' "$answer" "$(stty size | cut -d" " -f2)"; exit 3`)
	exit, err := terminal(context.Background(), command, dir, strings.NewReader("héllo\n"), &screen)
	if err != nil {
		t.Fatal(err)
	}
	if exit != 3 {
		t.Errorf("exit %d, want the program's own 3", exit)
	}
	if !strings.Contains(screen.String(), "got héllo, in 120 columns") {
		t.Errorf("the screen showed %q", screen.String())
	}
	file, err := os.Open(dir.OutFile("pane.cast"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	lines := bufio.NewScanner(file)
	lines.Scan()
	var header struct{ Version, Width, Height int }
	if err := json.Unmarshal(lines.Bytes(), &header); err != nil || header.Version != 2 || header.Width != PaneCols {
		t.Fatalf("header %s", lines.Text())
	}
	kinds := map[string]string{}
	for lines.Scan() {
		var event []any
		if err := json.Unmarshal(lines.Bytes(), &event); err != nil {
			t.Fatalf("an event is not JSON: %s", lines.Text())
		}
		kinds[event[1].(string)] += event[2].(string)
	}
	if !strings.Contains(kinds["i"], "héllo") || !strings.Contains(kinds["o"], "got héllo") {
		t.Errorf("recorded %q typed and %q shown", kinds["i"], kinds["o"])
	}
}

// A read can end inside a character. The record holds the start of it until
// the rest comes, so no event carries half a character.
func TestACharacterCutAcrossReadsIsRecordedWhole(t *testing.T) {
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	record, err := openCast(dir.OutFile("pane.cast"), PaneCols, PaneRows)
	if err != nil {
		t.Fatal(err)
	}
	e := []byte("é")
	if err := record.event("o", []byte{'a', e[0]}); err != nil {
		t.Fatal(err)
	}
	if err := record.event("o", []byte{e[1], 'b'}); err != nil {
		t.Fatal(err)
	}
	record.Close()
	text, _ := os.ReadFile(dir.OutFile("pane.cast"))
	if strings.Contains(string(text), "\\ufffd") || !strings.Contains(string(text), `"éb"`) {
		t.Errorf("recorded %s", text)
	}
}
