package computer

import (
	"sort"
	"strings"
	"time"
)

// Drift is where the record and the gateway disagree about an organization.
type Drift struct {
	// Missing are computers the record says are there and the gateway has no
	// sandbox for.
	Missing []Computer
	// Orphans are sandboxes the gateway has under this organization's name and
	// the record has no computer for.
	Orphans []string
}

// Clean says the two agree.
func (d Drift) Clean() bool { return len(d.Missing) == 0 && len(d.Orphans) == 0 }

// MakingWithin is how long a computer may be being made. One that has been Creating longer was left so by a server that ended
// before it said how the making went, and is missing when the gateway has no sandbox for it.
const MakingWithin = 15 * time.Minute

// Compare sets the record against the names of the sandboxes on a gateway. A
// computer that is still being made, or was deleted, is not missing, and a
// sandbox that is not a computer's, a task's for one, is not an orphan.
func (r *Registry) Compare(gateway []string) (Drift, error) {
	all, err := r.All()
	if err != nil {
		return Drift{}, err
	}
	there := map[string]bool{}
	for _, name := range gateway {
		there[name] = true
	}
	known := map[string]bool{}
	var drift Drift
	for _, each := range all {
		known[each.Sandbox] = true
		if (each.State != Creating || r.now().Sub(each.Created) > MakingWithin) && !there[each.Sandbox] {
			drift.Missing = append(drift.Missing, each)
		}
	}
	prefix := Prefix(r.org)
	for _, name := range gateway {
		if strings.HasPrefix(name, prefix) && !known[name] {
			drift.Orphans = append(drift.Orphans, name)
		}
	}
	sort.Strings(drift.Orphans)
	return drift, nil
}
