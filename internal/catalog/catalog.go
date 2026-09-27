// Package catalog says what a person can actually pick.
//
// A list of names is not a catalog. Somebody choosing a harness needs to know
// what it is, what it needs before it can answer, and — the part a bare list
// hides — whether this machine can run it at all. Offering one it cannot is how
// a first run becomes a container that starts, fails to find its program, and
// reports it as the harness failing.
package catalog

import (
	"sort"

	"github.com/getlatentic/haas/internal/harness"
)

// Entry is one harness, as a person choosing has to see it.
type Entry struct {
	Name    string
	What    string
	Program string
	Needs   harness.Needs
	// Declared is whether the image names this harness among those it
	// carries. It is the image's word; Ready rests on the program itself.
	Declared bool
	// Ready is whether this machine can run it now. Why says what is missing
	// when it cannot, in words somebody can act on.
	Ready bool
	Why   string
}

// What a harness needs, said plainly rather than as a keyword.
func (e Entry) Wants() string {
	switch e.Needs {
	case harness.NeedsNothing:
		return "no model at all"
	case harness.NeedsLocal:
		return "a model in the building"
	case harness.NeedsAccount:
		return "your own signed-in account"
	case harness.NeedsKey:
		return "a key the proxy holds"
	}
	return string(e.Needs)
}

// Survey asks the image which harnesses it carries. It is done once, when the
// server starts, because the answer only changes when the image does and asking
// per page would start a container for every row.
func Survey(runtime, image string, names []string) []Entry {
	return Of(Container(runtime, image), names)
}

// Of builds the catalog from what one image answers. A name nobody can load is
// left out: there is nothing to say about it that would help somebody choose.
func Of(image Image, names []string) []Entry {
	adapters := map[string]harness.Adapter{}
	programs := make([]string, 0, len(names))
	for _, name := range names {
		if adapter, err := harness.Load("", name); err == nil {
			adapters[name] = adapter
			programs = append(programs, adapter.Program)
		}
	}
	carried := image.Carried(programs)
	entries := make([]Entry, 0, len(adapters))
	for name, adapter := range adapters {
		entries = append(entries, entryOf(name, adapter, carried))
	}
	sort.Slice(entries, func(a, b int) bool {
		if entries[a].Ready != entries[b].Ready {
			return entries[a].Ready
		}
		return entries[a].Name < entries[b].Name
	})
	return entries
}

func entryOf(name string, adapter harness.Adapter, carried Carried) Entry {
	entry := Entry{
		Name:     name,
		What:     adapter.What,
		Program:  adapter.Program,
		Needs:    adapter.Needs,
		Declared: carried.Declares(name),
	}
	entry.Ready, entry.Why = carried.Runs(name, adapter.Program)
	if entry.Ready && adapter.Needs == harness.NeedsAccount {
		// The program is there, but it answers to somebody's own
		// subscription, which a shared server does not have.
		entry.Ready = false
		entry.Why = "it signs in as a person, so it runs where that person is"
	}
	return entry
}

// Ready is the names a job may name, which is what a form should offer.
func Ready(entries []Entry) []string {
	var names []string
	for _, entry := range entries {
		if entry.Ready {
			names = append(names, entry.Name)
		}
	}
	return names
}
