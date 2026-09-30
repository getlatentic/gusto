package jobs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/haas/internal/quota"
)

// limited is an index under a quota kept in a file beside it.
func limited(t *testing.T, q quota.Quota) (*Index, string) {
	t.Helper()
	index := at(t)
	path := filepath.Join(t.TempDir(), "quota.json")
	if err := quota.Write(path, q); err != nil {
		t.Fatal(err)
	}
	index.UseQuota(quota.Open(path))
	return index, path
}

func TestAJobPastTheTrialIsRefusedByNameAndLeavesNoDirectory(t *testing.T) {
	index, _ := limited(t, quota.Quota{Jobs: 3, Concurrent: 1, Note: "Confirm your email address."})
	for i := range 3 {
		by(t, index, "amy", "", "job "+string(rune('a'+i)))
	}
	before, _ := os.ReadDir(index.Root())
	_, err := index.Reserve("one too many", time.Now())
	if !errors.Is(err, quota.ErrExceeded) {
		t.Fatalf("the fourth job should be refused as over the limit: %v", err)
	}
	if !strings.Contains(err.Error(), "3 jobs in all") || !strings.Contains(err.Error(), "Confirm your email address.") {
		t.Errorf("the refusal should say what was allowed and how to lift it: %v", err)
	}
	if after, _ := os.ReadDir(index.Root()); len(after) != len(before) {
		t.Errorf("a refused job made a directory: %d before, %d after", len(before), len(after))
	}
}

func TestAMonthlyQuotaCountsThisMonthOnly(t *testing.T) {
	index, _ := limited(t, quota.Quota{Jobs: 2, Monthly: true, Concurrent: 1})
	lastMonth := time.Now().UTC().AddDate(0, -1, -1)
	for i := range 5 {
		id := reserved(t, index, "old "+string(rune('a'+i)))
		record := Record{ID: id, State: Passed, Brief: "old", Harness: "fixture", Asked: "amy", Queued: lastMonth.Unix()}
		os.MkdirAll(record.Dir(index.Root()).Path(), 0o755)
		if err := index.Add(record); err != nil {
			t.Fatalf("a job of last month does not count against this month: %v", err)
		}
	}
	by(t, index, "amy", "", "this month 1")
	by(t, index, "amy", "", "this month 2")
	if _, err := index.Reserve("this month 3", time.Now()); !errors.Is(err, quota.ErrExceeded) || !strings.Contains(err.Error(), "2 jobs a month") {
		t.Errorf("%v", err)
	}
	// The same job asked for next month is the first of that month.
	if _, err := index.Reserve("next month", time.Now().UTC().AddDate(0, 1, 0)); err != nil {
		t.Errorf("a new month starts again: %v", err)
	}
}

// A limit that is lifted applies to the next job, and the server is not restarted.
func TestALiftedQuotaAppliesToTheNextJob(t *testing.T) {
	index, path := limited(t, quota.Quota{Jobs: 1, Concurrent: 1})
	by(t, index, "amy", "", "first")
	if _, err := index.Reserve("second", time.Now()); !errors.Is(err, quota.ErrExceeded) {
		t.Fatal(err)
	}
	if err := quota.Write(path, quota.Quota{Jobs: 200, Monthly: true, Concurrent: 4}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := index.Reserve("second", time.Now()); err != nil {
		t.Errorf("the lifted limit should apply: %v", err)
	}
}

// Two jobs asked for together both pass the check in Reserve; Add is where one of
// them is stopped, so the quota is never passed by a race.
func TestJobsAskedForTogetherNeverPassTheQuota(t *testing.T) {
	index, _ := limited(t, quota.Quota{Jobs: 5, Concurrent: 1})
	var wg sync.WaitGroup
	var mu sync.Mutex
	added := 0
	for i := range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := index.Reserve("race "+string(rune('a'+i%26))+string(rune('a'+i/26)), time.Now())
			if err != nil {
				return
			}
			record := Record{ID: id, State: Queued, Brief: "race", Harness: "fixture", Asked: "amy", Queued: Now()}
			os.MkdirAll(record.Dir(index.Root()).Path(), 0o755)
			if index.Add(record) == nil {
				mu.Lock()
				added++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if added != 5 || len(index.List()) != 5 {
		t.Fatalf("%d were added and %d are in the index, with a quota of 5", added, len(index.List()))
	}
}

func TestTheQueueStartsNoMoreAtOnceThanTheQuotaAllows(t *testing.T) {
	index, path := limited(t, quota.Quota{Jobs: 10, Concurrent: 1})
	first := by(t, index, "amy", "", "first")
	second := by(t, index, "bo", "", "second")
	if got := claimed(t, index); got != "first" {
		t.Fatalf("%q", got)
	}
	if got := claimed(t, index); got != "" {
		t.Fatalf("the organisation runs one at a time: %q", got)
	}
	wait, queued := index.WaitOf(second.ID)
	if !queued || wait.Reason != OrgAtLimit || wait.Limit != 1 || wait.Running != 1 {
		t.Errorf("the second waits on the organisation's limit: %+v", wait)
	}
	if err := index.Finish(first.ID, Passed, 0); err != nil {
		t.Fatal(err)
	}
	if got := claimed(t, index); got != "second" {
		t.Errorf("when one ends the next starts: %q", got)
	}
	// Raising the limit lets two run together.
	quota.Write(path, quota.Quota{Jobs: 10, Concurrent: 2})
	time.Sleep(1100 * time.Millisecond)
	by(t, index, "cy", "", "third")
	if got := claimed(t, index); got != "third" {
		t.Errorf("a higher limit starts a second at once: %q", got)
	}
}

// An organisation that was given limits and lost them runs nothing new, and
// what was running goes on.
func TestAQuotaThatCannotBeReadRefusesNewJobsAndLetsOneRun(t *testing.T) {
	index, path := limited(t, quota.Quota{Jobs: 10, Concurrent: 4})
	by(t, index, "amy", "", "queued before")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	_, err := index.Reserve("after", time.Now())
	if err == nil || errors.Is(err, quota.ErrExceeded) || !strings.Contains(err.Error(), "cannot be read") {
		t.Errorf("a quota that cannot be read is a fault, not a limit reached: %v", err)
	}
	if got := claimed(t, index); got != "queued before" {
		t.Errorf("one job may still run: %q", got)
	}
}

func TestAnIndexWithNoQuotaIsNotLimited(t *testing.T) {
	index := at(t)
	for i := range 30 {
		by(t, index, "amy", "", "job "+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
}

func TestTheAllowanceSaysHowMuchOfTheQuotaIsUsedAndFailsWhenItCannotBeRead(t *testing.T) {
	plain, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, under, err := plain.Allowance(time.Now()); under || err != nil {
		t.Errorf("an index under no quota: %v %v", under, err)
	}

	index, path := limited(t, quota.Quota{Jobs: 10, Monthly: true, Concurrent: 2, Computers: 4})
	lastMonth := time.Now().UTC().AddDate(0, -1, -1)
	record := Record{ID: "old", State: Passed, Brief: "old", Harness: "fixture", Asked: "amy", Queued: lastMonth.Unix()}
	os.MkdirAll(record.Dir(index.Root()).Path(), 0o755)
	if err := index.Add(record); err != nil {
		t.Fatal(err)
	}
	by(t, index, "amy", "", "this month")
	allowed, under, err := index.Allowance(time.Now())
	if err != nil || !under || allowed.Started != 1 || allowed.Quota.Jobs != 10 || allowed.Quota.Computers != 4 {
		t.Errorf("a monthly quota counts this month's jobs: %+v %v %v", allowed, under, err)
	}

	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, under, err := index.Allowance(time.Now()); !under || err == nil {
		t.Errorf("a quota that cannot be read is an error: %v %v", under, err)
	}
}
