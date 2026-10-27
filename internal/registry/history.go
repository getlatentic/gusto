package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/getlatentic/gusto/internal/atomicfile"
)

// mostRevisions is how many earlier states of the registry are kept to go back to.
const mostRevisions = 20

// Revision is an earlier state of the registry, kept from the moment it was replaced. It holds no key: a key kept here is in its own
// file, and is forgotten when its provider is retired.
type Revision struct {
	Version  int64    `json:"version"`
	At       int64    `json:"at"`
	Document Document `json:"document"`
}

func (r *Registry) historyPath() string { return filepath.Join(filepath.Dir(r.path), "history.json") }

func (r *Registry) readHistory() error {
	text, err := os.ReadFile(r.historyPath())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("the registry's history in %s cannot be read: %w", r.historyPath(), err)
	}
	if err := json.Unmarshal(text, &r.history); err != nil {
		return fmt.Errorf("the registry's history in %s is not valid: %w", r.historyPath(), err)
	}
	return nil
}

// remember keeps the state that is about to be replaced. A registry with nothing in it has nothing to go back to.
func (r *Registry) remember(was Document) error {
	if was.Version == 0 {
		return nil
	}
	kept := append(append([]Revision(nil), r.history...), Revision{Version: was.Version, At: time.Now().Unix(), Document: clone(was)})
	if len(kept) > mostRevisions {
		kept = kept[len(kept)-mostRevisions:]
	}
	if err := atomicfile.Write(r.historyPath(), kept, 0o600); err != nil {
		return err
	}
	r.history = kept
	return nil
}

// Step is one earlier state, and what was done to it to make the one after.
type Step struct {
	Version int64
	At      int64
	// Then is what was changed after this state: going back to it undoes these.
	Then []string
}

// Steps are the states that can be gone back to, the latest first, each with what was done after it.
func (r *Registry) Steps() []Step {
	r.mu.Lock()
	defer r.mu.Unlock()
	steps := make([]Step, 0, len(r.history))
	for i, revision := range r.history {
		next := r.document
		if i+1 < len(r.history) {
			next = r.history[i+1].Document
		}
		steps = append(steps, Step{Version: revision.Version, At: revision.At, Then: Describe(revision.Document, next)})
	}
	sort.SliceStable(steps, func(a, b int) bool { return steps[a].Version > steps[b].Version })
	return steps
}

// Restore puts the registry back as it was at an earlier version, as a change of its own, so that what it was before is kept and
// can be gone back to. A provider whose key was kept here and was retired since has lost its key, and is left out with the reason.
func (r *Registry) Restore(version int64) ([]string, error) {
	r.mu.Lock()
	var found *Revision
	for i := range r.history {
		if r.history[i].Version == version {
			found = &r.history[i]
		}
	}
	if found == nil {
		r.mu.Unlock()
		return nil, fmt.Errorf("version %d is not one the registry has kept", version)
	}
	snapshot, now := clone(found.Document), clone(r.document)
	r.mu.Unlock()
	return Describe(now, snapshot), r.change(func(d *Document) error {
		d.Providers, d.Models, d.Access, d.Policy = snapshot.Providers, snapshot.Models, snapshot.Access, snapshot.Policy
		return nil
	})
}

// Describe says what differs between two states, as short lines. It names providers and models and never a key.
func Describe(was, now Document) []string {
	var lines []string
	providers := map[string]Provider{}
	for _, provider := range was.Providers {
		providers[provider.Name] = provider
	}
	for _, provider := range now.Providers {
		before, had := providers[provider.Name]
		switch {
		case !had:
			lines = append(lines, "added provider "+provider.Name)
		case before.Retired != provider.Retired && provider.Retired:
			lines = append(lines, "retired provider "+provider.Name)
		case before.Retired != provider.Retired:
			lines = append(lines, "brought back provider "+provider.Name)
		case before.Pending != provider.Pending && provider.Pending:
			lines = append(lines, "turned off provider "+provider.Name)
		case before.Pending != provider.Pending:
			lines = append(lines, "turned on provider "+provider.Name)
		case before.Address != provider.Address || before.Key != provider.Key || before.Auth != provider.Auth || !sameJSON(before.Endpoints, provider.Endpoints):
			lines = append(lines, "changed provider "+provider.Name)
		}
		delete(providers, provider.Name)
	}
	for name := range providers {
		lines = append(lines, "removed provider "+name)
	}
	models := map[string]Model{}
	for _, model := range was.Models {
		models[model.Full()] = model
	}
	for _, model := range now.Models {
		before, had := models[model.Full()]
		switch {
		case !had:
			lines = append(lines, "added model "+model.Full())
		case before.Retired != model.Retired && model.Retired:
			lines = append(lines, "retired model "+model.Full())
		case before.Retired != model.Retired:
			lines = append(lines, "brought back model "+model.Full())
		case before.Pending != model.Pending && model.Pending:
			lines = append(lines, "turned off model "+model.Full())
		case before.Pending != model.Pending:
			lines = append(lines, "turned on model "+model.Full())
		}
		if had && !samePrice(before, model) {
			lines = append(lines, "changed the price of "+model.Full())
		}
		delete(models, model.Full())
	}
	for name := range models {
		lines = append(lines, "removed model "+name)
	}
	if !sameJSON(was.Access, now.Access) {
		lines = append(lines, "changed who may use which models")
	}
	if !sameJSON(was.Policy, now.Policy) {
		lines = append(lines, "changed what the registry allows")
	}
	sort.Strings(lines)
	return lines
}

func samePrice(a, b Model) bool { return sameJSON(a.Price, b.Price) }

func sameJSON(a, b any) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}
