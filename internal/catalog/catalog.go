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
	Title   string
	What    string
	Program string
	Needs   harness.Needs
	// Skills and MCP are whether the harness can be given skills and MCP
	// servers, so a setup it cannot take is never offered for it.
	Skills bool
	MCP    bool
	// Declared is whether the image names this harness among those it
	// carries. It is the image's word; Ready rests on the program itself.
	Declared bool
	// Ready is whether this machine can run it now. Why says what is missing
	// when it cannot, in words somebody can act on.
	Ready bool
	Why   string
	// Measured is what a matrix saw this harness do on its simplest job, one
	// run a model. Empty is a harness no matrix measured.
	Measured []Measured
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
// per page would start a container for every row. The matrix named by
// HAAS_MATRIX is read then too, so a pair that cannot finish the simplest job
// says so.
func Survey(runtime, image string, names []string) []Entry {
	return SurveyOf(Container(runtime, image), names)
}

// SurveyOf is Survey of an image that is asked some other way, such as a
// sandbox on a gateway.
func SurveyOf(image Image, names []string) []Entry {
	return measuredFromSetting(Of(image, names))
}

// Of builds the catalog from what one image answers. A name nobody can load is
// left out: there is nothing to say about it that would help somebody choose.
func Of(image Image, names []string) []Entry {
	adapters := map[string]harness.Adapter{}
	programs := make([]string, 0, len(names))
	for _, name := range names {
		if adapter, err := harness.Load("", name); err == nil {
			adapters[name] = adapter
			programs = append(programs, adapter.Installed())
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
		Title:    titleOf(name, adapter),
		What:     adapter.What,
		Program:  adapter.Program,
		Needs:    adapter.Needs,
		Skills:   adapter.Skills != "",
		MCP:      adapter.TakesMCP(),
		Declared: carried.Declares(name),
	}
	entry.Ready, entry.Why = carried.Runs(name, adapter.Installed())
	if carried.Machine && !entry.Ready {
		entry.Why = notInstalled(adapter)
	}
	if entry.Ready && adapter.Needs == harness.NeedsAccount {
		// The program is there, but it answers to somebody's own
		// subscription, which a shared server does not have.
		entry.Ready = false
		entry.Why = "it signs in as a person, so it runs where that person is"
	}
	return entry
}

// notInstalled is how a person installs a harness that is not on their computer.
func notInstalled(adapter harness.Adapter) string {
	if adapter.Package == "" {
		return adapter.Installed() + " is not installed on this computer"
	}
	return "npm install -g " + adapter.Package
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

// titleOf is a harness's name for a person, its program's name when the
// adapter gives none.
func titleOf(name string, adapter harness.Adapter) string {
	if adapter.Title != "" {
		return adapter.Title
	}
	return name
}
