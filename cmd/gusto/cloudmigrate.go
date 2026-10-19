package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/getlatentic/gusto/cmd/internal/cmdline"
	"github.com/getlatentic/gusto/internal/cloud"
)

var envCloudDatabase = []cmdline.Param{
	{Name: "GUSTO_DATABASE_URL", Text: "where the accounts are kept: empty for cloud.db under GUSTO_CLOUD_ROOT, or a postgres:// address (a search_path parameter names the schema, which is made when missing)"},
	{Name: "GUSTO_CLOUD_ROOT", Text: "the directory gusto cloud keeps organizations' files in, and cloud.db when there is no database address (default cloud)"},
}

func cloudMigrateCommand() *cmdline.Command {
	return &cmdline.Command{
		Name: "migrate", Summary: "Show the accounts database, or load the old account files into it",
		Usage: []string{"[-import]"},
		Description: "Says where gusto cloud keeps its accounts, the schema version the database is at and the newest this release " +
			"reads, and how many organizations, people, invitations, links, demo leads and runtime records it holds. It also says " +
			"how many records the files of an earlier release (people/, orgs/, tokens/ and demos/ under the root) hold that an " +
			"import would load, and when the import ran. Asking changes nothing: it makes no file and no schema. " +
			"With -import it brings the schema up to date and loads those files into the database now, if the database has not had " +
			"them, and prints how many records of each kind went in and how many were left out. gusto cloud does the same when " +
			"it starts on a database that has no import record, so this is for doing it first: stop the earlier release, run " +
			"this, then start the new one. The files are never changed or removed. The import is one transaction, and two runs " +
			"at once import once. A database that has been imported is not imported again, so a link that was used does not come back.",
		Flags:    flagsOf(declareCloudMigrate),
		Env:      envCloudDatabase,
		Examples: []string{"gusto cloud migrate", "gusto cloud migrate -import", "GUSTO_DATABASE_URL=postgres://db/gusto gusto cloud migrate -import"},
		Exit: []cmdline.Param{
			{Name: "0", Text: "the report was printed, or the import ran or had run"},
			{Name: "1", Text: "the database cannot be reached, is at a newer schema than this release reads, or the import failed"},
			{Name: "2", Text: "usage error"},
		},
		SeeAlso: []string{"cloud", "operator"},
		Run:     cloudMigrate,
	}
}

type cloudMigrateFlags struct{ run bool }

func declareCloudMigrate(set *flag.FlagSet) *cloudMigrateFlags {
	f := &cloudMigrateFlags{}
	set.BoolVar(&f.run, "import", false, "load the files of an earlier release into the database now, if it has not had them")
	return f
}

func cloudMigrate(args []string) int {
	set := newSet("cloud migrate")
	f := declareCloudMigrate(set)
	if code, ok := parseFlags("cloud migrate", set, args); !ok {
		return code
	}
	if set.NArg() != 0 {
		return usageError("cloud migrate", "takes no arguments")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	root, settings := setting("GUSTO_CLOUD_ROOT", "cloud"), cloud.Settings{DatabaseURL: cloudDatabaseURL()}
	if f.run {
		if code := runCloudImport(ctx, os.Stdout, root, settings); code != 0 {
			return code
		}
	}
	return printCloudReport(ctx, os.Stdout, root, settings)
}

func runCloudImport(ctx context.Context, out io.Writer, root string, settings cloud.Settings) int {
	record, ran, err := cloud.ImportNow(ctx, root, settings)
	if err != nil {
		return fail(err)
	}
	if !ran {
		fmt.Fprintf(out, "the database has had its import already, on %s; nothing was loaded\n\n", record.At.Format(time.RFC3339))
		return 0
	}
	fmt.Fprintf(out, "imported into the database, and the files under %s are as they were:\n", root)
	printCounts(out, "  ", record.Imported)
	if len(record.Skipped) > 0 {
		fmt.Fprintln(out, "left out:")
		printCounts(out, "  ", record.Skipped)
	}
	fmt.Fprintln(out)
	return 0
}

func printCloudReport(ctx context.Context, out io.Writer, root string, settings cloud.Settings) int {
	report, err := cloud.Inspect(ctx, root, settings)
	if err != nil {
		return fail(err)
	}
	where := report.Database
	if where == "" {
		where = "not made yet"
	}
	fmt.Fprintf(out, "accounts database  %s\n", where)
	fmt.Fprintf(out, "schema version     %d (this release reads %d)\n", report.Version, report.Latest)
	if report.Version > report.Latest {
		return fail(fmt.Errorf("the database is at a newer schema than this release reads: run the release that wrote it"))
	}
	if report.Version != 0 && report.Version == report.Latest {
		fmt.Fprintln(out, "holds:")
		printCounts(out, "  ", report.Counts)
	}
	fmt.Fprintln(out, "old files under the root, that an import would load:")
	printCounts(out, "  ", report.Files)
	if report.Import != nil {
		fmt.Fprintf(out, "import             ran %s: %s\n", report.Import.At.Format(time.RFC3339), report.Import.Summary())
	} else {
		fmt.Fprintln(out, "import             not run yet")
	}
	return 0
}

func printCounts(out io.Writer, indent string, counts map[string]int) {
	kinds := make([]string, 0, len(counts))
	for kind := range counts {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	table := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, kind := range kinds {
		fmt.Fprintf(table, "%s%s\t%d\n", indent, kind, counts[kind])
	}
	table.Flush()
}
