package web

import (
	"net/http"
	"sort"
	"time"

	"github.com/getlatentic/gusto/internal/jobs"
)

// verifiedRuns is how many of the newest runs the verification page checks. Each check reads the run's files.
const verifiedRuns = 40

// checked is one run and what checking its files and signature found.
type checked struct {
	ID, Brief, Who, When, State string
	Href                        string
}

// verification says, for the newest runs, whether each still matches what was recorded when it finished.
func (s *Server) verificationPage(w http.ResponseWriter, r *http.Request) {
	all := s.index.List()
	sort.SliceStable(all, func(a, b int) bool { return all[a].Queued > all[b].Queued })
	counts := map[string]int{}
	var rows []checked
	for _, record := range all {
		if len(rows) == verifiedRuns {
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
		rows = append(rows, checkedRow(record, state))
	}
	s.drawConsole(w, "admin-verification.html", "verification", map[string]any{
		"Title": productName + " — verification", "Rows": rows, "Verified": counts["verified"], "Unsigned": counts["unsigned"],
		"Unchecked": counts["unchecked"], "Failing": counts["does not check out"], "Shown": verifiedRuns,
	})
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
