package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/getlatentic/gusto/internal/datadir"
)

// dataDirs is where each store keeps its data, from the same settings the
// server reads. A store that is not set up here is left out.
func dataDirs() map[datadir.Store]string {
	dirs := map[datadir.Store]string{
		datadir.Jobs:         setting("GUSTO_JOBS", "jobs"),
		datadir.Sessions:     setting("GUSTO_SESSIONS", "sessions"),
		datadir.Setups:       setting("GUSTO_SETUPS", "setups"),
		datadir.Comparisons:  setting("GUSTO_COMPARISONS", "comparisons"),
		datadir.Integrations: setting("GUSTO_INTEGRATIONS", "integrations"),
		datadir.Presets:      setting("GUSTO_PRESETS", "presets"),
		datadir.Changes:      filepath.Dir(setting("GUSTO_CHANGES", filepath.Join("changes", "changes.jsonl"))),
	}
	if path := os.Getenv("GUSTO_API_TOKENS"); path != "" {
		dirs[datadir.Tokens] = filepath.Dir(path)
	}
	if dir := os.Getenv("GUSTO_RETENTION"); dir != "" {
		dirs[datadir.Retention] = dir
	}
	return dirs
}

// ready makes every data directory ready for this release, or says why one is
// not. It runs before the server opens anything, so a refusal leaves the data
// exactly as it was.
func ready() error {
	options := datadir.Options{Release: releaseName(), Irreversible: os.Getenv("GUSTO_MIGRATE_IRREVERSIBLE") == "1"}
	dirs := dataDirs()
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
	table := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "store\tdirectory\tformat\twritten by\tthis release reads")
	dirs := dataDirs()
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
	fmt.Println(usageStatusLine())
	fmt.Println(taskIndexStatusLine())
	return 0
}

// dataCheck says what starting this release would do to the data, and whether
// it could be undone. It changes nothing, and exits 1 when the server would
// refuse to start.
func dataCheck() int {
	refused := false
	options := datadir.Options{Release: releaseName(), Irreversible: os.Getenv("GUSTO_MIGRATE_IRREVERSIBLE") == "1"}
	dirs := dataDirs()
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
	dirs := dataDirs()
	var refusals []error
	for _, store := range datadir.Stores {
		if dir, kept := dirs[store]; kept {
			if err := canGoBack(store, dir, formats[store]); err != nil {
				refusals = append(refusals, err)
			}
		}
	}
	if err := errors.Join(refusals...); err != nil {
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
