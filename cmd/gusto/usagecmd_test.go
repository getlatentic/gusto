package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/cmd/internal/cmdline/clitest"
	"github.com/getlatentic/gusto/internal/instance"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/usage"
	"github.com/getlatentic/gusto/internal/web"
)

func withUsageData(t *testing.T) (data string) {
	t.Helper()
	data = t.TempDir()
	t.Setenv("GUSTO_JOBS", filepath.Join(data, "jobs"))
	t.Setenv("GUSTO_SESSIONS", filepath.Join(data, "sessions"))
	t.Setenv("GUSTO_DATABASE_URL", "")
	t.Setenv("GUSTO_REGISTRY", "")
	return data
}

func num(n int64) *int64 { return &n }

func usageRow(task string, line int, session, person, model string, at time.Time, usd float64) usage.Row {
	cost := usd
	return usage.Row{
		Task: task, Line: line, At: at, Session: session, Person: person, Engine: "Claude Code", Provider: "anthropic", Model: model,
		InputTokens: num(2000), OutputTokens: num(500), CachedTokens: num(1000), USD: &cost,
	}
}

func seedUsage(t *testing.T, data string, rows ...usage.Row) {
	t.Helper()
	store, err := usage.Open(context.Background(), "", data)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Add(context.Background(), rows); err != nil {
		t.Fatal(err)
	}
}

func TestTheUsageCommandsHelpSaysWhatTheyReadAndHowTheyEnd(t *testing.T) {
	for _, path := range [][]string{{"usage"}, {"usage", "sessions"}, {"usage", "reindex"}} {
		out, _, code := clitest.Capture(t, func() int { return run(append([]string{"help"}, path...)) })
		if code != 0 {
			t.Fatalf("%v: exit code %d", path, code)
		}
		if len(path) == 2 {
			for _, want := range []string{"GUSTO_DATABASE_URL", "GUSTO_JOBS", "Examples", "Exit codes"} {
				if !strings.Contains(out, want) {
					t.Errorf("%v: help lacks %q:\n%s", path, want, out)
				}
			}
		}
	}
	out, _, _ := clitest.Capture(t, func() int { return run([]string{"help", "usage", "sessions"}) })
	for _, flag := range []string{"-person", "-agent", "-engine", "-provider", "-model", "-month", "-limit", "-json"} {
		if !strings.Contains(out, flag) {
			t.Errorf("sessions help lacks %s", flag)
		}
	}
	out, _, _ = clitest.Capture(t, func() int { return run([]string{"help"}) })
	if !strings.Contains(out, "usage") {
		t.Errorf("the global help does not list usage:\n%s", out)
	}
}

func TestUsageSessionsPrintsATableAndFiltersByMonthAndPerson(t *testing.T) {
	data := withUsageData(t)
	oct := time.Date(2026, 10, 5, 9, 30, 0, 0, time.UTC)
	seedUsage(t, data,
		usageRow("t1", 0, "s1", "ada@acme.com", "claude-sonnet", oct, 1.25),
		usageRow("t1", 1, "s1", "ada@acme.com", "claude-sonnet", oct.Add(time.Minute), 0.75),
		usageRow("t2", 0, "s2", "bob@acme.com", "gpt-5", oct.AddDate(-1, 0, 0), 3),
	)
	out, errs, code := clitest.Capture(t, func() int { return run([]string{"usage", "sessions"}) })
	if code != 0 || errs != "" {
		t.Fatalf("exit %d, stderr %q", code, errs)
	}
	for _, want := range []string{"LAST USED", "CACHE", "s1", "ada@acme.com", "$2.00", "anthropic", "claude-sonnet", "5.0k", "50%", "2026-10-05 09:31", "s2", "$3.00"} {
		if !strings.Contains(out, want) {
			t.Errorf("the table lacks %q:\n%s", want, out)
		}
	}
	out, _, code = clitest.Capture(t, func() int { return run([]string{"usage", "sessions", "--month", "2026-10"}) })
	if code != 0 || strings.Contains(out, "s2") || !strings.Contains(out, "s1") {
		t.Errorf("-month 2026-10 gave %d:\n%s", code, out)
	}
	out, _, _ = clitest.Capture(t, func() int { return run([]string{"usage", "sessions", "-person", "bob@acme.com"}) })
	if strings.Contains(out, "s1") || !strings.Contains(out, "s2") {
		t.Errorf("-person gave:\n%s", out)
	}
	out, _, _ = clitest.Capture(t, func() int { return run([]string{"usage", "sessions", "-limit", "1"}) })
	if strings.Count(out, "\n") != 2 {
		t.Errorf("-limit 1 gave:\n%s", out)
	}
	out, _, code = clitest.Capture(t, func() int { return run([]string{"usage", "sessions", "-model", "nothing"}) })
	if code != 0 || !strings.Contains(out, "No session matches.") {
		t.Errorf("no match gave %d:\n%s", code, out)
	}
}

func TestUsageSessionsAsJSONAndAnUnpricedSessionIsNotShownAsFree(t *testing.T) {
	data := withUsageData(t)
	at := time.Date(2026, 10, 5, 9, 30, 0, 0, time.UTC)
	unpriced := usageRow("t1", 0, "s1", "ada@acme.com", "local-model", at, 0)
	unpriced.USD = nil
	seedUsage(t, data, unpriced)
	out, _, code := clitest.Capture(t, func() int { return run([]string{"usage", "sessions"}) })
	if code != 0 || !strings.Contains(out, "n/a") {
		t.Errorf("an unpriced session gave %d:\n%s", code, out)
	}
	out, _, code = clitest.Capture(t, func() int { return run([]string{"usage", "sessions", "-json"}) })
	var sessions []usage.Session
	if err := json.Unmarshal([]byte(out), &sessions); err != nil || code != 0 || len(sessions) != 1 || sessions[0].Unmeasured != 1 {
		t.Errorf("-json gave %d, %v:\n%s", code, err, out)
	}
}

func TestUsageSessionsRefusesWhatItCannotRead(t *testing.T) {
	withUsageData(t)
	for _, args := range [][]string{{"usage", "sessions", "-month", "october"}, {"usage", "sessions", "-limit", "0"}, {"usage", "sessions", "extra"}, {"usage", "reindex", "extra"}, {"usage"}} {
		_, errs, code := clitest.Capture(t, func() int { return run(args) })
		if code != 2 || !strings.Contains(errs, "Run 'gusto help usage") {
			t.Errorf("%v: exit %d, stderr %q", args, code, errs)
		}
	}
}

func writeTask(t *testing.T, data, id, asked, lines string) {
	t.Helper()
	dir := filepath.Join(data, "jobs", id)
	if err := os.MkdirAll(filepath.Join(dir, "out"), 0o755); err != nil {
		t.Fatal(err)
	}
	state := `{"id":"` + id + `","state":"passed","brief":"b","harness":"fixture","asked":"` + asked + `","team":"platform"}`
	for name, text := range map[string]string{"state.json": state, "out/proxy.jsonl": lines} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

const modelLine = `{"path":"/v1/chat/completions","status":200,"tookMs":10,"credential":"platform","atUnix":1790000000,"model":"m1","inputTokens":100,"outputTokens":10,"usd":0.5}` + "\n"

func TestUsageReindexFillsTheIndexFromTheRecordsAndIsRepeatable(t *testing.T) {
	data := withUsageData(t)
	writeTask(t, data, "20261001-0900-a", "ada@acme.com", modelLine+modelLine+"not a call\n")
	writeTask(t, data, "20261001-0901-b", "bob@acme.com", modelLine)
	out, _, code := clitest.Capture(t, func() int { return run([]string{"usage", "reindex"}) })
	if code != 0 || !strings.Contains(out, "added 3 call(s)") || !strings.Contains(out, "passed over 1 line(s)") {
		t.Fatalf("reindex gave %d:\n%s", code, out)
	}
	out, _, _ = clitest.Capture(t, func() int { return run([]string{"usage", "sessions"}) })
	for _, want := range []string{"20261001-0900-a", "ada@acme.com", "bob@acme.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("sessions lacks %q:\n%s", want, out)
		}
	}
	out, _, code = clitest.Capture(t, func() int { return run([]string{"usage", "reindex"}) })
	if code != 0 || !strings.Contains(out, "added 0 call(s)") {
		t.Errorf("the second reindex gave %d:\n%s", code, out)
	}
}

func TestUsageReindexFromScratchSaysHowManyRowsItRemovedAndRebuildsThem(t *testing.T) {
	data := withUsageData(t)
	writeTask(t, data, "20261001-0900-a", "ada@acme.com", modelLine+modelLine)
	clitest.Capture(t, func() int { return run([]string{"usage", "reindex"}) })
	out, _, code := clitest.Capture(t, func() int { return run([]string{"usage", "reindex", "-from-scratch"}) })
	if code != 0 || !strings.Contains(out, "removed 2 row(s)") || !strings.Contains(out, "added 2 call(s)") {
		t.Fatalf("from scratch gave %d:\n%s", code, out)
	}
}

func TestDoctorAndDataStatusReportTheIndex(t *testing.T) {
	data := withUsageData(t)
	result := usageCheck()
	if result.Status != "pass" || !strings.Contains(result.Said, "does not exist yet") {
		t.Errorf("before the server made it: %+v", result)
	}
	seedUsage(t, data, usageRow("t1", 0, "s1", "ada@acme.com", "m", time.Now(), 1))
	result = usageCheck()
	if result.Status != "pass" || !strings.Contains(result.Said, "schema version") || result.Name != "usage index" {
		t.Errorf("after: %+v", result)
	}
	if line := usageStatusLine(); !strings.Contains(line, "schema version") || !strings.Contains(line, "1 rows") {
		t.Errorf("data status line: %q", line)
	}
	t.Setenv("GUSTO_DATABASE_URL", "ftp://u:hunter2@h/db")
	result = usageCheck()
	if result.Status != "fail" || strings.Contains(result.Said, "hunter2") || result.Remedy == "" {
		t.Errorf("a database that cannot be used: %+v", result)
	}
}

func TestAnIndexPlaceNeverHoldsAPassword(t *testing.T) {
	t.Setenv("GUSTO_DATABASE_URL", "postgres://gusto:hunter2@db.internal:5432/gusto?search_path=org_acme")
	place := usagePlace("jobs")
	if strings.Contains(place, "hunter2") || !strings.Contains(place, "db.internal") || !strings.Contains(place, "org_acme") {
		t.Errorf("%q", place)
	}
}

func TestAServerFillsTheIndexAtStartAndStopsCleanly(t *testing.T) {
	data := withUsageData(t)
	writeTask(t, data, "20261001-0900-a", "ada@acme.com", modelLine+modelLine)
	index, err := jobs.Open(filepath.Join(data, "jobs"))
	if err != nil {
		t.Fatal(err)
	}
	instances, err := instance.Open(filepath.Join(data, "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	identity, _ := web.NewIdentity(context.Background(), "", "", "", "", web.Access{})
	pages, err := web.New(index, instances, nil, "unused", identity, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	stop, err := startUsage(nil, pages, index.Root())
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	deadline := time.Now().Add(10 * time.Second)
	for {
		store, err := usage.Open(context.Background(), "", data)
		if err != nil {
			t.Fatal(err)
		}
		totals, _ := store.Totals(context.Background(), usage.Filter{Person: "ada@acme.com"})
		store.Close()
		if totals.Calls == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the index holds %d calls", totals.Calls)
		}
		time.Sleep(50 * time.Millisecond)
	}
	stop()
}
