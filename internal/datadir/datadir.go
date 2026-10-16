// Package datadir gives every directory the server keeps data in a format
// version, so that a newer release can migrate it and an older one can refuse
// what it cannot read, by name, before it damages anything.
//
// A directory made before the stamp existed is format 1. A release that
// changes what a directory holds raises its format, adds a Step that migrates
// from the old format, and says in Releases what each release wrote. A Step
// that cannot be undone stops the upgrade until the operator says it may run.
package datadir

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// StampFile is the file that carries a directory's format. Its name begins with
// a dot so that a release that predates it, listing the directory, passes over it.
const StampFile = ".gusto-format"

// Store is one kind of data directory.
type Store string

const (
	Jobs        Store = "jobs"
	Sessions    Store = "sessions"
	Setups      Store = "setups"
	Comparisons Store = "comparisons"
	Changes     Store = "changes"
	Tokens      Store = "tokens"
	Retention   Store = "retention"
	// Integrations is the plugins an administrator imported. A release before it never reads the directory, so none can be hurt by it.
	Integrations Store = "integrations"
	// Presets is what each person saved to start a chat the same way again. A release before it never reads the directory.
	Presets Store = "presets"
)

// Stores is every kind, in the order they are listed.
var Stores = []Store{Jobs, Sessions, Setups, Comparisons, Changes, Tokens, Retention, Integrations, Presets}

// Current is the format each store has in this release.
var Current = map[Store]int{
	Jobs: 1, Sessions: 1, Setups: 1, Comparisons: 1, Changes: 1, Tokens: 1, Retention: 1, Integrations: 1, Presets: 1,
}

// Stamp is what a directory says about itself.
type Stamp struct {
	Format int `json:"format"`
	// Release is the release that last wrote the stamp.
	Release string `json:"release"`
	At      int64  `json:"at"`
}

// Read is a directory's stamp, and whether it has one.
func Read(dir string) (Stamp, bool, error) {
	text, err := os.ReadFile(filepath.Join(dir, StampFile))
	if errors.Is(err, os.ErrNotExist) {
		return Stamp{}, false, nil
	}
	if err != nil {
		return Stamp{}, false, err
	}
	var stamp Stamp
	if err := json.Unmarshal(text, &stamp); err != nil || stamp.Format < 1 {
		return Stamp{}, false, fmt.Errorf("%s does not say a format: %v", filepath.Join(dir, StampFile), err)
	}
	return stamp, true, nil
}

func write(dir string, format int, release string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	text, err := json.Marshal(Stamp{Format: format, Release: release, At: time.Now().Unix()})
	if err != nil {
		return err
	}
	scratch := filepath.Join(dir, StampFile+".tmp")
	if err := os.WriteFile(scratch, append(text, '\n'), 0o640); err != nil {
		return err
	}
	return os.Rename(scratch, filepath.Join(dir, StampFile))
}

// legacy is the format of a directory that has data and no stamp: everything
// written before stamps existed.
const legacy = 1

// present says whether a directory holds anything besides its stamp.
func present(dir string) bool {
	listing, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range listing {
		if !strings.HasPrefix(entry.Name(), StampFile) {
			return true
		}
	}
	return false
}

// NewerError is a directory written by a release newer than this one.
type NewerError struct {
	Store, Dir     string
	Found, Known   int
	WrittenBy      string
	ThisRelease    string
	RollbackTarget string
}

func (e NewerError) Error() string {
	return fmt.Sprintf("%s holds %s data in format %d, written by %s, and this release (%s) reads format %d and no newer. "+
		"Run this release's upgrade again, or roll the data back with the newer release: gusto data rollback -to <release>. "+
		"Nothing has been changed", e.Dir, e.Store, e.Found, or(e.WrittenBy, "a newer release"), or(e.ThisRelease, "unnamed"), e.Known)
}

func or(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// IrreversibleError is an upgrade whose steps cannot all be undone, refused
// until the operator says it may go ahead.
type IrreversibleError struct {
	Store, Dir string
	Steps      []Step
}

func (e IrreversibleError) Error() string {
	var names []string
	for _, step := range e.Steps {
		names = append(names, fmt.Sprintf("%s format %d to %d (%s)", step.Store, step.From, step.From+1, step.What))
	}
	return fmt.Sprintf("upgrading %s cannot be undone: %s. Once it runs, an older release cannot read this data, and going back means restoring a copy made before. "+
		"Back the directory up, then set GUSTO_MIGRATE_IRREVERSIBLE=1 to go ahead. Nothing has been changed", e.Dir, strings.Join(names, "; "))
}

// Options are what an upgrade is allowed to do.
type Options struct {
	// Release names the running release, for the stamp.
	Release string
	// Irreversible allows steps that have no Down.
	Irreversible bool
}

// Result is what Ensure found and did.
type Result struct {
	Store        Store
	Dir          string
	From, To     int
	Stamped      bool
	Migrated     []Step
	WasUnstamped bool
}

// Ensure makes a directory ready for this release: it stamps one that has no
// stamp, migrates one that is older, and refuses one that is newer or whose
// migration cannot be undone and has not been allowed.
func Ensure(store Store, dir string, opts Options) (Result, error) {
	known := Current[store]
	result := Result{Store: store, Dir: dir, To: known}
	stamp, stamped, err := Read(dir)
	if err != nil {
		return result, err
	}
	switch {
	case stamped:
		result.From = stamp.Format
	case present(dir):
		result.From, result.WasUnstamped = legacy, true
	default:
		result.From = known
	}
	if result.From > known {
		return result, NewerError{Store: string(store), Dir: dir, Found: result.From, Known: known, WrittenBy: stamp.Release, ThisRelease: opts.Release}
	}
	pending := stepsFor(store, result.From, known)
	if blocked := irreversible(pending); len(blocked) > 0 && !opts.Irreversible {
		return result, IrreversibleError{Store: string(store), Dir: dir, Steps: blocked}
	}
	for _, step := range pending {
		if err := step.Up(dir); err != nil {
			return result, fmt.Errorf("upgrading %s from format %d to %d (%s) failed, and the data is still at format %d: %w",
				dir, step.From, step.From+1, step.What, step.From, err)
		}
		if err := write(dir, step.From+1, opts.Release); err != nil {
			return result, err
		}
		result.Migrated = append(result.Migrated, step)
	}
	if stamped && result.From == known && stamp.Release == opts.Release {
		return result, nil
	}
	result.Stamped = true
	return result, write(dir, known, opts.Release)
}

// Refusal is the error Ensure would give for a directory, found without
// changing anything.
func Refusal(store Store, dir string, opts Options) error {
	from, to, pending, err := Plan(store, dir)
	if err != nil {
		return err
	}
	stamp, _, _ := Read(dir)
	if from > to {
		return NewerError{Store: string(store), Dir: dir, Found: from, Known: to, WrittenBy: stamp.Release, ThisRelease: opts.Release}
	}
	if blocked := irreversible(pending); len(blocked) > 0 && !opts.Irreversible {
		return IrreversibleError{Store: string(store), Dir: dir, Steps: blocked}
	}
	return nil
}

// Plan says what Ensure would do to a directory, and changes nothing.
func Plan(store Store, dir string) (from, to int, pending []Step, err error) {
	stamp, stamped, err := Read(dir)
	if err != nil {
		return 0, 0, nil, err
	}
	from = Current[store]
	switch {
	case stamped:
		from = stamp.Format
	case present(dir):
		from = legacy
	}
	return from, Current[store], stepsFor(store, from, Current[store]), nil
}

func irreversible(steps []Step) []Step {
	var blocked []Step
	for _, step := range steps {
		if step.Down == nil {
			blocked = append(blocked, step)
		}
	}
	return blocked
}

// undoing is the steps that take a directory back to format to, and refuses
// when one of them cannot be undone.
func undoing(store Store, dir string, to int) (steps []Step, from int, err error) {
	stamp, stamped, err := Read(dir)
	if err != nil {
		return nil, 0, err
	}
	from = Current[store]
	if stamped {
		from = stamp.Format
	}
	if to > from || to < 1 {
		return nil, from, fmt.Errorf("%s is at format %d and cannot be rolled back to %d", dir, from, to)
	}
	for format := from - 1; format >= to; format-- {
		if step, found := stepFrom(store, format); found {
			steps = append(steps, step)
		}
	}
	if blocked := irreversible(steps); len(blocked) > 0 {
		var names []string
		for _, step := range blocked {
			names = append(names, fmt.Sprintf("format %d to %d (%s)", step.From, step.From+1, step.What))
		}
		return nil, from, fmt.Errorf("%s cannot be rolled back to format %d: %s cannot be undone, so the data would be unreadable. "+
			"Restore a copy made before the upgrade. Nothing has been changed", dir, to, strings.Join(names, "; "))
	}
	return steps, from, nil
}

// CanRollback says whether a directory can be taken back to format to, and
// changes nothing. It is the check made before an upgrade, and before a rollback.
func CanRollback(store Store, dir string, to int) error {
	_, _, err := undoing(store, dir, to)
	return err
}

// Rollback undoes migrations until the directory is in format to, so that the
// release that wrote that format can run on it. It runs with the newer release,
// because only that release knows how, and it refuses first when a step in the
// way cannot be undone.
func Rollback(store Store, dir string, to int, release string) error {
	steps, _, err := undoing(store, dir, to)
	if err != nil {
		return err
	}
	for _, step := range steps {
		if err := step.Down(dir); err != nil {
			return fmt.Errorf("rolling %s back from format %d failed, and the data is still at format %d: %w", dir, step.From+1, step.From+1, err)
		}
		if err := write(dir, step.From, release); err != nil {
			return err
		}
	}
	return write(dir, to, release)
}
