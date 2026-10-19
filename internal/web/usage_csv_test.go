package web

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/usage"
)

func decode(got *httptest.ResponseRecorder, into any) error {
	return json.Unmarshal(got.Body.Bytes(), into)
}

func records(t *testing.T, got *httptest.ResponseRecorder) [][]string {
	t.Helper()
	if got.Code != http.StatusOK {
		t.Fatalf("%d %s", got.Code, got.Body)
	}
	read, err := csv.NewReader(got.Body).ReadAll()
	if err != nil {
		t.Fatalf("not CSV: %v\n%s", err, got.Body)
	}
	return read
}

func TestTheCsvHasAHeaderAndACallPerLine(t *testing.T) {
	unpriced := aCall("task-a", 2, "alice-session", alice, 2)
	unpriced.USD, unpriced.OutputTokens = nil, nil
	here, _ := withUsage(t, aCall("task-a", 0, "alice-session", alice, 0), aCall("task-a", 1, "alice-session", alice, 1), unpriced, aCall("task-b", 0, "bob-session", bob, 3))
	got := here.call("GET", "/api/usage.csv", here.secrets["finance"])
	if kind := got.Header().Get("Content-Type"); !strings.HasPrefix(kind, "text/csv") || !strings.Contains(got.Header().Get("Content-Disposition"), "attachment") {
		t.Errorf("the headers were %v", got.Header())
	}
	lines := records(t, got)
	if len(lines) != 5 || strings.Join(lines[0], ",") != strings.Join(usageCSVHeader, ",") {
		t.Fatalf("the export was %v", lines)
	}
	column := map[string]int{}
	for i, name := range lines[0] {
		column[name] = i
	}
	byTask := map[string][]string{}
	for _, line := range lines[1:] {
		byTask[line[column["task"]]+"/"+line[column["line"]]] = line
	}
	priced, free := byTask["task-a/0"], byTask["task-a/2"]
	if priced[column["cost_usd"]] != "0.25" || priced[column["input_tokens"]] != "1000" || priced[column["person"]] != alice || priced[column["time"]] != "2026-10-14T12:00:00Z" {
		t.Errorf("a priced call was %v", priced)
	}
	if free[column["cost_usd"]] != "" || free[column["output_tokens"]] != "" {
		t.Errorf("a call with no price or count should leave the cell empty, not 0: %v", free)
	}
}

func TestTheCsvFollowsTheFilterAndKeepsEveryCallOfEverySession(t *testing.T) {
	var rows []usage.Row
	for line := 0; line < 450; line++ {
		rows = append(rows, aCall("task-big", line, "big-session", alice, 0))
		rows[line].At = noon.Add(time.Duration(line) * time.Second)
	}
	other := aCall("task-o", 0, "other-session", alice, 9)
	other.Model = "other-model"
	here, _ := withUsage(t, append(rows, other, aCall("task-b", 0, "bob-session", bob, 1))...)
	if lines := records(t, here.call("GET", "/api/usage.csv", here.secrets["finance"])); len(lines) != 1+450+1+1 {
		t.Errorf("the export held %d lines for 452 calls", len(lines)-1)
	}
	if lines := records(t, here.call("GET", "/api/usage.csv?model=other-model", here.secrets["finance"])); len(lines) != 2 {
		t.Errorf("a filter on the model left %d calls", len(lines)-1)
	}
	if lines := records(t, here.call("GET", "/api/usage.csv?q=big&limit=3&sort=cost", here.secrets["finance"])); len(lines) != 451 {
		t.Errorf("a search left %d calls, and a page size must not cut an export", len(lines)-1)
	}
	if lines := records(t, here.call("GET", "/api/usage.csv?person="+url.QueryEscape(bob), here.secrets["finance"])); len(lines) != 2 || lines[1][4] != bob {
		t.Errorf("a filter on the person left %v", lines)
	}
	if got := here.call("GET", "/api/usage.csv?from=never", here.secrets["finance"]); got.Code != http.StatusBadRequest {
		t.Errorf("a bad date answered %d", got.Code)
	}
}

func TestACellThatStartsAFormulaIsQuotedSoASpreadsheetDoesNotRunIt(t *testing.T) {
	for _, c := range []struct{ in, out string }{
		{"=SUM(A1)", "'=SUM(A1)"}, {"+1+1", "'+1+1"}, {"-2", "'-2"}, {"@cmd", "'@cmd"}, {"\tcmd", "'\tcmd"}, {"fine=ok", "fine=ok"}, {"", ""}, {"alice@example.com", "alice@example.com"},
	} {
		if got := safeCell(c.in); got != c.out {
			t.Errorf("%q became %q, not %q", c.in, got, c.out)
		}
	}
	nasty := aCall("task-a", 0, "=HYPERLINK(\"http://x\")", "@sum", 0)
	nasty.Agent, nasty.Model, nasty.Credential = "-cmd|' /C calc'!A0", "a,b", "he said \"hi\"\nthen left"
	here, _ := withUsage(t, nasty)
	got := here.call("GET", "/api/usage.csv", here.secrets["finance"])
	text := got.Body.String()
	lines := records(t, got)
	if len(lines) != 2 {
		t.Fatalf("the export was %v", lines)
	}
	row := strings.Join(lines[1], "|")
	for _, want := range []string{"'=HYPERLINK(\"http://x\")", "'@sum", "'-cmd|' /C calc'!A0", "a,b", "he said \"hi\"\nthen left"} {
		if !strings.Contains(row, want) {
			t.Errorf("the line %q lacks %q", row, want)
		}
	}
	for _, cell := range lines[1] {
		if cell != "" && strings.ContainsRune("=+-@", rune(cell[0])) {
			t.Errorf("the cell %q would be run as a formula", cell)
		}
	}
	if !strings.Contains(text, `"he said ""hi""`) {
		t.Errorf("quotes should be doubled inside a quoted cell:\n%s", text)
	}
}
