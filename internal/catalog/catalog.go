// Package catalog says what a person can actually pick.
//
// A list of names is not a catalog. Somebody choosing a harness needs to know
// what it is, what it needs before it can answer, and — the part a bare list
// hides — whether this machine can run it at all. Offering one it cannot is how
// a first run becomes a container that starts, fails to find its program, and
// reports it as the harness failing.
package catalog

import (
	"context"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/getlatentic/haas/internal/harness"
)

// Entry is one harness, as a person choosing has to see it.
type Entry struct {
	Name    string
	Program string
	Needs   harness.Needs
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
	entries := make([]Entry, 0, len(names))
	for _, name := range names {
		adapter, err := harness.Load("", name)
		if err != nil {
			continue
		}
		entry := Entry{Name: name, Program: adapter.Program, Needs: adapter.Needs}
		entry.Ready, entry.Why = carried(runtime, image, adapter.Program)
		if entry.Ready && adapter.Needs == harness.NeedsAccount {
			// The program is there, but it answers to somebody's own
			// subscription, which a shared server does not have.
			entry.Ready = false
			entry.Why = "it signs in as a person, so it runs where that person is"
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(a, b int) bool {
		if entries[a].Ready != entries[b].Ready {
			return entries[a].Ready
		}
		return entries[a].Name < entries[b].Name
	})
	return entries
}

// carried asks the image whether it has the program, and says so plainly when
// it does not.
func carried(runtime, image, program string) (bool, string) {
	if runtime == "" || image == "" {
		return true, ""
	}
	context, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ask := exec.CommandContext(context, runtime, "run", "--rm", "--entrypoint", "sh", image,
		"-c", "command -v "+program+" >/dev/null")
	if err := ask.Run(); err != nil {
		return false, "the runner image does not carry " + program
	}
	return true, ""
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

// Models is what this server offers to think with, named by the deployment
// because only it knows which the credential reaches.
func Models(given string) []string {
	var models []string
	for _, each := range strings.Split(given, ",") {
		if trimmed := strings.TrimSpace(each); trimmed != "" {
			models = append(models, trimmed)
		}
	}
	return models
}
