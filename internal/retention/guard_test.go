package retention

import (
	"testing"

	"github.com/getlatentic/gusto/internal/tasks"
)

// listing is the index, and something that happens as a sweep lists it: a hold placed while the sweep runs.
type listing struct {
	*tasks.Index
	meanwhile func()
}

func (l listing) List() []tasks.Record {
	all := l.Index.List()
	l.meanwhile()
	return all
}

func TestAHoldPlacedWhileASweepRunsKeepsTheTask(t *testing.T) {
	index := opened(t)
	finished(t, index, "old", tasks.Passed, 100, 1000)
	holds := OpenHolds(t.TempDir())
	during := listing{Index: index, meanwhile: func() {
		if err := holds.HoldTask("old", "ada@acme.test", "an audit"); err != nil {
			t.Fatal(err)
		}
	}}
	report := Sweep(during, index.Root(), policy, holds, nil, noon, false)
	if _, kept := index.Get("old"); !kept || len(report.Held) != 1 {
		t.Errorf("a task held while the sweep ran was removed: %+v", report)
	}
}

func TestARecordCannotGoWhileItsWorkingFolderIsKeptForEver(t *testing.T) {
	forever := Policy{RecordDays: map[string]int{"passed": 30}}
	if err := forever.Check(); err == nil {
		t.Error("a policy that removes a workspace meant to be kept for ever with its record was taken")
	}
	both := Policy{WorkingFolderDays: map[string]int{"passed": 7}, RecordDays: map[string]int{"passed": 30}}
	if err := both.Check(); err != nil {
		t.Errorf("a workspace before its record was refused: %v", err)
	}
}
