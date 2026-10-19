package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/getlatentic/gusto/cmd/internal/cmdline"
	"github.com/getlatentic/gusto/internal/usage"
)

var envUsage = []cmdline.Param{
	{Name: "GUSTO_DATABASE_URL", Text: "where the index is kept: empty for usage.db beside the tasks directory, or a postgres:// address (a search_path parameter names the schema)"},
	{Name: "GUSTO_JOBS", Text: "the directory of tasks, when -tasks is not given (default jobs)"},
}

func usageCommands() []*cmdline.Command {
	return []*cmdline.Command{{
		Name: "usage", Group: groupRecords, Summary: "Search the model calls the agents made, by session",
		Description: "The index of model calls is built from each task's proxy record (out/proxy.jsonl), which stays the record. " +
			"These commands read the index and rebuild it. A rebuilt index holds the same rows as the one it replaces.",
		Subs: []*cmdline.Command{
			{
				Name: "sessions", Summary: "List sessions with what they spent, the most recently used first",
				Usage: []string{"[-person <who>] [-agent <name>] [-engine <name>] [-provider <name>] [-model <name>] [-month <2006-01>] [-limit <n>] [-tasks <dir>] [-json]"},
				Description: "Lists the sessions that have a call matching the flags. A session is the calls of one conversation, or of one " +
					"task that is in none. The cost counts only calls the models' prices were known for: a cost ending in + leaves out " +
					"some calls, and n/a is a session none of whose calls have a price. Cache is the share of input tokens the provider's cache answered.",
				Flags:    flagsOf(declareSessions),
				Env:      envUsage,
				Examples: []string{"gusto usage sessions -month 2026-10", "gusto usage sessions -person ada@acme.com -provider anthropic -json"},
				Exit: []cmdline.Param{
					{Name: "0", Text: "success, including when no session matches"},
					{Name: "1", Text: "the index could not be read"},
					{Name: "2", Text: "usage error"},
				},
				SeeAlso: []string{"usage reindex", "spend"},
				Run:     usageSessions,
			},
			{
				Name: "reindex", Summary: "Fill the index from the tasks' records now",
				Usage: []string{"[-from-scratch] [-tasks <dir>]"},
				Description: "Reads the model calls the index does not hold from every task's proxy record and adds them, printing how far it " +
					"has got. It can be run again at any time and picks up where an interrupted run stopped. With -from-scratch it first " +
					"removes every row, says how many, and rebuilds them all. A task that retention removed has no record to read, so " +
					"a rebuild from scratch leaves its calls out.",
				Flags:    flagsOf(declareReindex),
				Env:      envUsage,
				Examples: []string{"gusto usage reindex", "gusto usage reindex -from-scratch"},
				Exit: []cmdline.Param{
					{Name: "0", Text: "the index holds every call in the records"},
					{Name: "1", Text: "the index could not be read or written, or the run was interrupted"},
					{Name: "2", Text: "usage error"},
				},
				SeeAlso: []string{"usage sessions", "data"},
				Run:     usageReindex,
			},
		},
	}}
}

type sessionsFlags struct {
	person, agent, engine, provider, model, month, tasks string
	limit                                                int
	json                                                 bool
}

func declareSessions(set *flag.FlagSet) *sessionsFlags {
	f := &sessionsFlags{}
	set.StringVar(&f.person, "person", "", "only this `person`'s sessions")
	set.StringVar(&f.agent, "agent", "", "only this `agent`, by the name its person gave it")
	set.StringVar(&f.engine, "engine", "", "only this `engine`, the program that made the calls")
	set.StringVar(&f.provider, "provider", "", "only calls this `provider` answered")
	set.StringVar(&f.model, "model", "", "only calls to this `model`")
	set.StringVar(&f.month, "month", "", "only calls in this month, as `2006-01`")
	set.IntVar(&f.limit, "limit", 20, "the most `sessions` to list")
	set.StringVar(&f.tasks, "tasks", "", "the `directory` the server keeps tasks in (default $GUSTO_JOBS, else jobs)")
	set.BoolVar(&f.json, "json", false, "print the sessions as JSON")
	return f
}

func (f *sessionsFlags) filter() (usage.Filter, error) {
	filter := usage.Filter{Person: f.person, Agent: f.agent, Engine: f.engine, Provider: f.provider, Model: f.model}
	if f.month == "" {
		return filter, nil
	}
	first, err := time.Parse("2006-01", f.month)
	if err != nil {
		return filter, fmt.Errorf("-month wants a month written 2006-01, not %q", f.month)
	}
	filter.From, filter.To = first, first.AddDate(0, 1, 0)
	return filter, nil
}

func usageSessions(args []string) int {
	set := newSet("usage sessions")
	f := declareSessions(set)
	if code, ok := parseFlags("usage sessions", set, args); !ok {
		return code
	}
	if set.NArg() != 0 {
		return usageError("usage sessions", "takes no arguments")
	}
	filter, err := f.filter()
	if err != nil {
		return usageError("usage sessions", err.Error())
	}
	if f.limit < 1 {
		return usageError("usage sessions", "-limit wants a number above 0")
	}
	ctx := context.Background()
	store, err := openUsage(ctx, orSetting(f.tasks, "GUSTO_JOBS", "jobs"))
	if err != nil {
		return fail(err)
	}
	defer store.Close()
	found, err := listSessions(ctx, store, filter, f.limit)
	if err != nil {
		return fail(err)
	}
	if f.json {
		return printJSON(found)
	}
	return printSessions(found)
}

func listSessions(ctx context.Context, store usage.Store, filter usage.Filter, limit int) ([]usage.Session, error) {
	found := []usage.Session{}
	var after usage.Cursor
	for len(found) < limit {
		page, next, err := store.Sessions(ctx, filter, usage.Page{Limit: limit - len(found), After: after})
		if err != nil {
			return nil, err
		}
		found = append(found, page...)
		if next == "" {
			break
		}
		after = next
	}
	return found, nil
}

func printJSON(value any) int {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fail(err)
	}
	return 0
}

func printSessions(found []usage.Session) int {
	if len(found) == 0 {
		fmt.Println("No session matches.")
		return 0
	}
	table := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "LAST USED\tSESSION\tPERSON\tAGENT\tENGINE\tCOST\tPROVIDER\tMODEL\tTOKENS\tCACHE")
	for _, s := range found {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			s.Last.UTC().Format("2006-01-02 15:04"), s.ID, s.Person, orDash(s.Agent), s.Engine, costOf(s.Totals),
			orDash(strings.Join(s.Providers, ",")), orDash(strings.Join(s.Models, ",")),
			compactCount(s.InputTokens+s.OutputTokens), cachePercent(s.CachedTokens, s.InputTokens))
	}
	table.Flush()
	return 0
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func costOf(t usage.Totals) string {
	switch {
	case t.Calls > 0 && t.Unmeasured == t.Calls:
		return "n/a"
	case t.Unmeasured > 0:
		return fmt.Sprintf("$%.2f+", t.USD)
	}
	return fmt.Sprintf("$%.2f", t.USD)
}

func cachePercent(cached, input int64) string {
	if input <= 0 {
		return "-"
	}
	return strconv.FormatFloat(100*float64(cached)/float64(input), 'f', 0, 64) + "%"
}

func compactCount(n int64) string {
	switch {
	case n >= 1_000_000:
		return strconv.FormatFloat(float64(n)/1_000_000, 'f', 1, 64) + "M"
	case n >= 1_000:
		return strconv.FormatFloat(float64(n)/1_000, 'f', 1, 64) + "k"
	}
	return strconv.FormatInt(n, 10)
}

type reindexFlags struct {
	tasks       string
	fromScratch bool
}

func declareReindex(set *flag.FlagSet) *reindexFlags {
	f := &reindexFlags{}
	set.BoolVar(&f.fromScratch, "from-scratch", false, "remove every row first, then rebuild them all")
	set.StringVar(&f.tasks, "tasks", "", "the `directory` the server keeps tasks in (default $GUSTO_JOBS, else jobs)")
	return f
}

func usageReindex(args []string) int {
	set := newSet("usage reindex")
	f := declareReindex(set)
	if code, ok := parseFlags("usage reindex", set, args); !ok {
		return code
	}
	if set.NArg() != 0 {
		return usageError("usage reindex", "takes no arguments")
	}
	tasks := orSetting(f.tasks, "GUSTO_JOBS", "jobs")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store, err := openUsage(ctx, tasks)
	if err != nil {
		return fail(err)
	}
	defer store.Close()
	if f.fromScratch {
		if err := emptyIndex(ctx, store); err != nil {
			return fail(err)
		}
	}
	who, err := openAttribution(tasks)
	if err != nil {
		return fail(err)
	}
	progress := &usage.Progress{Changed: printProgress}
	result, err := usage.Backfill(ctx, store, tasks, who.who, who.provider, progress)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return fail(err)
	}
	fmt.Printf("read %d task(s), added %d call(s) to the index", result.Tasks, result.Rows)
	if result.Malformed > 0 {
		fmt.Printf(", passed over %d line(s) that are not calls", result.Malformed)
	}
	fmt.Println()
	return 0
}

// emptyIndex removes every row, and says how many were there.
func emptyIndex(ctx context.Context, store usage.Store) error {
	removed, err := store.Reset(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("removed %d row(s)\n", removed)
	return nil
}

func printProgress(s usage.Status) {
	fmt.Fprintf(os.Stderr, "\rindexing %d of %d task(s)", s.Done, s.Total)
}
