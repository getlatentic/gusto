package web

import (
	"sort"
	"time"

	"github.com/getlatentic/gusto/internal/jobs"
)

// verifiedRuns is how many of the newest runs the task checks look at. Each check reads the run's files.
const verifiedRuns = 40

// checked is one run and what checking its files and signature found.
type checked struct {
	ID, Brief, Who, When, State string
	Href                        string
}

// runChecks is what checking the newest finished runs found: how many came out each way, and the ones that need a look.
type runChecks struct {
	Rows                                   []checked
	Verified, Unsigned, Unchecked, Failing int
}

// Checked is how many runs were checked.
func (c runChecks) Checked() int { return c.Verified + c.Unsigned + c.Unchecked + c.Failing }

// checkRuns says, for the newest runs, whether each still matches what was recorded when it finished.
func (s *Server) checkRuns() runChecks {
	all := s.index.List()
	sort.SliceStable(all, func(a, b int) bool { return all[a].Queued > all[b].Queued })
	counts := map[string]int{}
	var rows []checked
	for _, record := range all {
		if checkedCount(counts) == verifiedRuns {
			break
		}
		if !record.State.Done() {
			continue
		}
		dir := record.Dir(s.index.Root())
		manifest := readManifest(dir)
		if manifest == nil {
			continue
		}
		state := verdict(s.proofOf(record.ID, dir, manifest))
		counts[state]++
		if state != "verified" {
			rows = append(rows, checkedRow(record, state))
		}
	}
	return runChecks{Rows: rows, Verified: counts["verified"], Unsigned: counts["unsigned"], Unchecked: counts["unchecked"], Failing: counts["does not check out"]}
}

func checkedRow(record jobs.Record, state string) checked {
	return checked{ID: record.ID, Brief: record.Brief, Who: record.Asked, State: state,
		When: time.Unix(record.Queued, 0).UTC().Format("2 Jan 15:04"), Href: "/jobs/" + record.ID}
}

// verdict is the one word for a run: verified, unsigned, unchecked (signed by a key this server does not hold) or failing.
func verdict(p *proof) string {
	switch {
	case !p.Holds():
		return "does not check out"
	case p.Signed:
		return "verified"
	case p.Unchecked != "":
		return "unchecked"
	}
	return "unsigned"
}

func checkedCount(counts map[string]int) int {
	total := 0
	for _, each := range counts {
		total += each
	}
	return total
}
