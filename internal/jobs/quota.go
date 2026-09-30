package jobs

import (
	"fmt"
	"time"

	"github.com/getlatentic/haas/internal/quota"
)

// UseQuota puts the index under a quota: a job past the number the quota allows
// is refused when it is asked for, and the queue starts no more at once than it
// allows. The quota is read as the index goes, so a limit that is lifted applies
// to the next job without a restart.
func (i *Index) UseQuota(q *quota.File) {
	i.guard.Lock()
	defer i.guard.Unlock()
	i.quota = q
}

// overQuota is a refusal when one more job would pass the quota, and is called
// with the lock held. A quota that cannot be read refuses every job: an
// organisation that was given limits and lost them runs nothing.
func (i *Index) overQuota(now time.Time) error {
	if i.quota == nil {
		return nil
	}
	q, err := i.quota.Current()
	if err != nil {
		return fmt.Errorf("no job is started, because %w", err)
	}
	return q.Refuse(i.startedWithin(q.Monthly, now))
}

// Allowance is what the quota lets the organisation start, and how many jobs it has
// started within that: in all, or in this calendar month when the quota is monthly.
type Allowance struct {
	Quota   quota.Quota
	Started int
}

// Allowance reads the quota and how much of it is used. It says false when the index
// is under no quota, and an error when the quota cannot be read, since a page that
// showed the organisation with no limit would be wrong and look right.
func (i *Index) Allowance(now time.Time) (Allowance, bool, error) {
	i.guard.Lock()
	defer i.guard.Unlock()
	if i.quota == nil {
		return Allowance{}, false, nil
	}
	q, err := i.quota.Current()
	if err != nil {
		return Allowance{}, true, err
	}
	return Allowance{Quota: q, Started: i.startedWithin(q.Monthly, now)}, true, nil
}

// startedWithin is how many jobs were started in all, or in the calendar month
// (in UTC) that now falls in.
func (i *Index) startedWithin(monthly bool, now time.Time) int {
	if !monthly {
		return len(i.records)
	}
	now = now.UTC()
	started := 0
	for _, record := range i.records {
		at := time.Unix(record.Queued, 0).UTC()
		if at.Year() == now.Year() && at.Month() == now.Month() {
			started++
		}
	}
	return started
}

// orgLimit is how many jobs may run at once under the quota, and whether one
// applies. A quota that cannot be read allows one, so what was running goes on
// and nothing more crowds in.
func (i *Index) orgLimit() (int, bool) {
	if i.quota == nil {
		return 0, false
	}
	q, err := i.quota.Current()
	if err != nil {
		return 1, true
	}
	return q.Concurrent, true
}

// orgFull says the organisation runs as many as its quota allows.
func (i *Index) orgFull(running int) bool {
	limit, applies := i.orgLimit()
	return applies && running >= limit
}
