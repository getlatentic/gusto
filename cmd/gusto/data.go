package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/getlatentic/gusto/internal/datadir"
	"github.com/getlatentic/gusto/internal/settings"
)

// storeSettings is where the stores every command shares keep their data, and how the data may be changed when a release needs
// another format.
type storeSettings struct {
	jobs, sessions, setups, comparisons, integrations, presets, crew, changes string
	apiTokens, retention, databaseURL                                         string
	irreversible                                                              bool
}

func (s *storeSettings) declare(set *settings.Set) {
	set.String(&s.jobs, "GUSTO_JOBS", "jobs", "the directory tasks are kept in")
	set.String(&s.sessions, "GUSTO_SESSIONS", "sessions", "the directory conversations and rooms are kept in")
	set.String(&s.setups, "GUSTO_SETUPS", "setups", "the directory skill sets are kept in")
	set.String(&s.comparisons, "GUSTO_COMPARISONS", "comparisons", "the directory comparisons are kept in")
	set.String(&s.integrations, "GUSTO_INTEGRATIONS", "integrations", "the directory imported integrations are kept in")
	set.String(&s.presets, "GUSTO_PRESETS", "presets", "the directory presets are kept in")
	set.String(&s.crew, "GUSTO_CREW", "crew", "the directory the crew of agents is kept in")
	set.String(&s.changes, "GUSTO_CHANGES", filepath.Join("changes", "changes.jsonl"), "the file every change to the platform is recorded in")
	set.String(&s.apiTokens, "GUSTO_API_TOKENS", "", "the file API tokens are kept in")
	set.String(&s.retention, "GUSTO_RETENTION", "", "the directory of the retention policy and what it removed")
	set.Secret(&s.databaseURL, "GUSTO_DATABASE_URL", "the database the task and usage indexes are kept in, sqlite:// or postgres:// (default a file beside GUSTO_JOBS)")
	set.Bool(&s.irreversible, "GUSTO_MIGRATE_IRREVERSIBLE", "let a release change the data in a way an earlier release cannot read")
}

// loadStores reads the stores' settings, for a command that needs no others.
func loadStores() (storeSettings, error) {
	var stores storeSettings
	return stores, loadSettings(stores.declare)
}

// firstOf is given when it says something, and otherwise fallback: a command's flag before the setting it stands in for.
func firstOf(given, fallback string) string {
	if given != "" {
		return given
	}
	return fallback
}

// tasksDir is the tasks directory a command was given, or the one the settings name.
func tasksDir(given string) (string, error) {
	stores, err := loadStores()
	return firstOf(given, stores.jobs), err
}

// indexesFor is where the indexes are, for the tasks directory a command was given, or the one the settings name.
func indexesFor(tasks string) (indexes, error) {
	stores, err := loadStores()
	where := indexesOf(stores)
	where.tasks = firstOf(tasks, where.tasks)
	return where, err
}

// dataDirs is where each store keeps its data. A store that is not set up here is left out.
func dataDirs(stores storeSettings) map[datadir.Store]string {
	dirs := map[datadir.Store]string{
		datadir.Jobs:         stores.jobs,
		datadir.Sessions:     stores.sessions,
		datadir.Setups:       stores.setups,
		datadir.Comparisons:  stores.comparisons,
		datadir.Integrations: stores.integrations,
		datadir.Presets:      stores.presets,
		datadir.Crew:         stores.crew,
		datadir.Changes:      filepath.Dir(stores.changes),
	}
	if stores.apiTokens != "" {
		dirs[datadir.Tokens] = filepath.Dir(stores.apiTokens)
	}
	if stores.retention != "" {
		dirs[datadir.Retention] = stores.retention
	}
	return dirs
}

// ready makes every data directory ready for this release, or says why one is
// not. It runs before the server opens anything, so a refusal leaves the data
// exactly as it was.
func ready(stores storeSettings) error {
	options := datadir.Options{Release: releaseName(), Irreversible: stores.irreversible}
	dirs := dataDirs(stores)
	// Every directory is checked before any is changed, so an upgrade that one
	// store refuses does not leave the others upgraded.
	for _, store := range datadir.Stores {
		dir, kept := dirs[store]
		if !kept {
			continue
		}
		if err := datadir.Refusal(store, dir, options); err != nil {
			return err
		}
	}
	for _, store := range datadir.Stores {
		dir, kept := dirs[store]
		if !kept {
			continue
		}
		result, err := datadir.Ensure(store, dir, options)
		if err != nil {
			return err
		}
		for _, step := range result.Migrated {
			fmt.Printf("upgraded %s from format %d to %d: %s\n", dir, step.From, step.From+1, step.What)
		}
	}
	return nil
}

func dataStatus() int {
	stores, err := loadStores()
	if err != nil {
		return fail(err)
	}
	table := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "store\tdirectory\tformat\twritten by\tthis release reads")
	dirs := dataDirs(stores)
	for _, store := range datadir.Stores {
		dir, kept := dirs[store]
		if !kept {
			continue
		}
		stamp, stamped, err := datadir.Read(dir)
		format, by := "none (format 1 if it has data)", ""
		if err != nil {
			return fail(err)
		}
		if stamped {
			format, by = fmt.Sprint(stamp.Format), stamp.Release
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%d\n", store, dir, format, by, datadir.Current[store])
	}
	table.Flush()
	fmt.Println()
	fmt.Println(usageStatusLine(indexesOf(stores)))
	fmt.Println(taskIndexStatusLine(indexesOf(stores)))
	return 0
}

// dataCheck says what starting this release would do to the data, and whether
// it could be undone. It changes nothing, and exits 1 when the server would
// refuse to start.
func dataCheck() int {
	stores, err := loadStores()
	if err != nil {
		return fail(err)
	}
	refused := false
	options := datadir.Options{Release: releaseName(), Irreversible: stores.irreversible}
	dirs := dataDirs(stores)
	for _, store := range datadir.Stores {
		dir, kept := dirs[store]
		if !kept {
			continue
		}
		from, to, pending, err := datadir.Plan(store, dir)
		if err != nil {
			return fail(err)
		}
		if err := datadir.Refusal(store, dir, options); err != nil {
			refused = true
			fmt.Printf("%-12s REFUSED: %v\n", store, err)
			continue
		}
		switch {
		case len(pending) == 0:
			fmt.Printf("%-12s format %d, nothing to do\n", store, from)
		default:
			fmt.Printf("%-12s format %d to %d: %d step(s)\n", store, from, to, len(pending))
			for _, step := range pending {
				fmt.Printf("  %s (%s)\n", step.What, reversibility(step))
			}
		}
	}
	if refused {
		return 1
	}
	return 0
}

func reversibility(step datadir.Step) string {
	if step.Down == nil {
		return "cannot be undone"
	}
	return "can be undone"
}

type rollbackFlags struct{ to string }

func declareRollback(set *flag.FlagSet) *rollbackFlags {
	f := &rollbackFlags{}
	set.StringVar(&f.to, "to", "", "the `release` to go back to")
	return f
}

// dataRollback takes the data back to what a named release wrote, so that
// release can run on it. It checks every store first and changes none if any
// cannot go back.
func dataRollback(args []string) int {
	set := newSet("data rollback")
	f := declareRollback(set)
	if code, ok := parseFlags("data rollback", set, args); !ok {
		return code
	}
	if f.to == "" || set.NArg() > 0 {
		return usageError("data rollback", "name the release to go back to with -to")
	}
	formats, err := datadir.FormatsOf(f.to)
	if err != nil {
		return fail(err)
	}
	stores, err := loadStores()
	if err != nil {
		return fail(err)
	}
	dirs := dataDirs(stores)
	if err := canAllGoBack(dirs, formats); err != nil {
		return fail(err)
	}
	for _, store := range datadir.Stores {
		dir, kept := dirs[store]
		if !kept {
			continue
		}
		if err := datadir.Rollback(store, dir, formats[store], releaseName()); err != nil {
			return fail(err)
		}
		fmt.Printf("%s is at format %d, as %s wrote it\n", dir, formats[store], f.to)
	}
	return 0
}

// canAllGoBack is whether every store's directory can be taken to the format it had in formats, each refusal said.
func canAllGoBack(dirs map[datadir.Store]string, formats map[datadir.Store]int) error {
	var refusals []error
	for _, store := range datadir.Stores {
		if dir, kept := dirs[store]; kept {
			if err := canGoBack(store, dir, formats[store]); err != nil {
				refusals = append(refusals, err)
			}
		}
	}
	return errors.Join(refusals...)
}

// canGoBack is whether a directory can be taken to format to. A directory
// already there, or older, is fine as it is.
func canGoBack(store datadir.Store, dir string, to int) error {
	stamp, stamped, err := datadir.Read(dir)
	if err != nil {
		return err
	}
	if !stamped || stamp.Format <= to {
		return nil
	}
	return datadir.CanRollback(store, dir, to)
}
