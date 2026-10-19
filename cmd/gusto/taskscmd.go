package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/getlatentic/gusto/cmd/internal/cmdline"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/taskindex"
)

var envTasks = []cmdline.Param{
	{Name: "GUSTO_DATABASE_URL", Text: "where the index is kept: empty for tasks.db beside the tasks directory, or a postgres:// address (a search_path parameter names the schema)"},
	{Name: "GUSTO_JOBS", Text: "the directory of tasks, when -tasks is not given (default jobs)"},
}

func tasksCommands() []*cmdline.Command {
	return []*cmdline.Command{{
		Name: "tasks", Group: groupRecords, Summary: "List and search the tasks a server has, from its index",
		Description: "The index of tasks keeps one row for each task, so that a long list can be searched and paged without reading " +
			"every task directory. Each task's state.json stays the truth. The server keeps the index current as tasks change, and these " +
			"commands read it and rebuild it. A rebuilt index holds the same rows as the one it replaces.",
		Subs: []*cmdline.Command{
			{
				Name: "list", Summary: "List tasks, the most recently asked first",
				Usage: []string{"[-person <who>] [-state <state>] [-agent-type <type>] [-repo <repo>] [-team <team>] [-search <text>] [-month <2006-01>] [-sort queued|ended|seq] [-limit <n>] [-tasks <dir>] [-json]"},
				Description: "Lists the tasks that match every flag given, read from the index and not from the task directories. " +
					"-state is queued, running, passed, failed or interrupted, or several of them separated by commas. -search matches part of " +
					"a task's id, brief, asker or repository, without regard to case. -sort orders from the greatest: queued is the task asked " +
					"for last, ended the task that ended last, and seq the task that arrived last. An index that was never filled lists nothing: " +
					"run gusto tasks reindex.",
				Flags:    flagsOf(declareTasksList),
				Env:      envTasks,
				Examples: []string{"gusto tasks list -state failed,interrupted -month 2026-10", "gusto tasks list -person ada@acme.com -search login -json"},
				Exit: []cmdline.Param{
					{Name: "0", Text: "success, including when no task matches"},
					{Name: "1", Text: "the index could not be read"},
					{Name: "2", Text: "usage error"},
				},
				SeeAlso: []string{"tasks reindex", "data"},
				Run:     tasksList,
			},
			{
				Name: "reindex", Summary: "Fill the index from the task directories now",
				Usage: []string{"[-from-scratch] [-tasks <dir>]"},
				Description: "Reads every task's state.json and brings the index up to date: it adds the tasks that have no row, corrects the " +
					"rows that differ from the file in any field, and removes the rows of tasks whose directory is gone. It changes no task. " +
					"It can be run again at any time, and while a server runs. With -from-scratch it first removes every row, says how many, " +
					"and rebuilds them all. A task a server is running is indexed as the file says, running, until the server writes it again.",
				Flags:    flagsOf(declareTasksReindex),
				Env:      envTasks,
				Examples: []string{"gusto tasks reindex", "gusto tasks reindex -from-scratch"},
				Exit: []cmdline.Param{
					{Name: "0", Text: "the index says what the task directories say"},
					{Name: "1", Text: "the index could not be read or written, or the run was interrupted"},
					{Name: "2", Text: "usage error"},
				},
				SeeAlso: []string{"tasks list", "usage reindex"},
				Run:     tasksReindex,
			},
		},
	}}
}

type tasksListFlags struct {
	person, state, agentType, repo, team, search, month, sort, tasks string
	limit                                                            int
	json                                                             bool
}

func declareTasksList(set *flag.FlagSet) *tasksListFlags {
	f := &tasksListFlags{}
	set.StringVar(&f.person, "person", "", "only this `person`'s tasks")
	set.StringVar(&f.state, "state", "", "only tasks in this `state`, or several separated by commas")
	set.StringVar(&f.agentType, "agent-type", "", "only this `agent type`, the program that runs the task")
	set.StringVar(&f.repo, "repo", "", "only tasks on this `repository`")
	set.StringVar(&f.team, "team", "", "only this `team`'s tasks")
	set.StringVar(&f.search, "search", "", "only tasks whose id, brief, asker or repository holds this `text`")
	set.StringVar(&f.month, "month", "", "only tasks asked for in this month, as `2006-01`")
	set.StringVar(&f.sort, "sort", "", "the `order`: queued (the default), ended or seq, from the greatest")
	set.IntVar(&f.limit, "limit", 20, "the most `tasks` to list")
	set.StringVar(&f.tasks, "tasks", "", "the `directory` the server keeps tasks in (default $GUSTO_JOBS, else jobs)")
	set.BoolVar(&f.json, "json", false, "print the tasks as JSON")
	return f
}

func (f *tasksListFlags) filter() (taskindex.Filter, error) {
	filter := taskindex.Filter{Asked: f.person, AgentType: f.agentType, Repo: f.repo, Team: f.team, Search: f.search}
	for _, state := range strings.Split(f.state, ",") {
		if state = strings.TrimSpace(state); state != "" {
			filter.States = append(filter.States, state)
		}
	}
	if f.month == "" {
		return filter, nil
	}
	first, err := time.Parse("2006-01", f.month)
	if err != nil {
		return filter, fmt.Errorf("-month wants a month written 2006-01, not %q", f.month)
	}
	filter.QueuedFrom, filter.QueuedTo = first, first.AddDate(0, 1, 0)
	return filter, nil
}

func tasksList(args []string) int {
	set := newSet("tasks list")
	f := declareTasksList(set)
	if code, ok := parseFlags("tasks list", set, args); !ok {
		return code
	}
	if set.NArg() != 0 {
		return usageError("tasks list", "takes no arguments")
	}
	filter, err := f.filter()
	if err != nil {
		return usageError("tasks list", err.Error())
	}
	if f.limit < 1 {
		return usageError("tasks list", "-limit wants a number above 0")
	}
	if _, err := taskindex.SortFor(f.sort); err != nil {
		return usageError("tasks list", err.Error())
	}
	ctx := context.Background()
	store, err := openTasks(ctx, orSetting(f.tasks, "GUSTO_JOBS", "jobs"))
	if err != nil {
		return fail(fmt.Errorf("%s", taskindex.Scrub(err)))
	}
	defer store.Close()
	found, err := listTasks(ctx, store, filter, f.sort, f.limit)
	if err != nil {
		return fail(fmt.Errorf("%s", taskindex.Scrub(err)))
	}
	if f.json {
		return printJSON(found)
	}
	return printTasks(found)
}

func listTasks(ctx context.Context, store taskindex.Store, filter taskindex.Filter, sort string, limit int) ([]taskindex.Row, error) {
	found := []taskindex.Row{}
	var after taskindex.Cursor
	for len(found) < limit {
		page, next, err := store.List(ctx, filter, taskindex.Page{Limit: limit - len(found), After: after, Sort: sort})
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

func printTasks(found []taskindex.Row) int {
	if len(found) == 0 {
		fmt.Println("No task matches.")
		return 0
	}
	table := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "ASKED\tTASK\tSTATE\tPERSON\tAGENT TYPE\tREPOSITORY\tBRIEF")
	for _, r := range found {
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", time.Unix(r.Queued, 0).UTC().Format("2006-01-02 15:04"), r.ID, r.State,
			orDash(r.Asked), orDash(r.AgentType), orDash(r.Repo), shortened(r.Brief, 60))
	}
	table.Flush()
	return 0
}

// shortened is the first line of text, cut to n letters.
func shortened(text string, n int) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	if letters := []rune(line); len(letters) > n {
		return string(letters[:n-1]) + "…"
	}
	return line
}

type tasksReindexFlags struct {
	tasks       string
	fromScratch bool
}

func declareTasksReindex(set *flag.FlagSet) *tasksReindexFlags {
	f := &tasksReindexFlags{}
	set.BoolVar(&f.fromScratch, "from-scratch", false, "remove every row first, then rebuild them all")
	set.StringVar(&f.tasks, "tasks", "", "the `directory` the server keeps tasks in (default $GUSTO_JOBS, else jobs)")
	return f
}

func tasksReindex(args []string) int {
	set := newSet("tasks reindex")
	f := declareTasksReindex(set)
	if code, ok := parseFlags("tasks reindex", set, args); !ok {
		return code
	}
	if set.NArg() != 0 {
		return usageError("tasks reindex", "takes no arguments")
	}
	tasks := orSetting(f.tasks, "GUSTO_JOBS", "jobs")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	store, err := openTasks(ctx, tasks)
	if err != nil {
		return fail(fmt.Errorf("the task index at %s: %s", taskIndexPlace(tasks), taskindex.Scrub(err)))
	}
	defer store.Close()
	progress := &taskindex.Progress{Changed: printTaskProgress}
	result, err := jobs.Rebuild(ctx, store, tasks, f.fromScratch, progress)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return fail(fmt.Errorf("%s", taskindex.Scrub(err)))
	}
	fmt.Printf("read %d task(s): added %d, corrected %d, removed %d\n", result.Tasks, result.Missing, result.Stale, result.Gone)
	return 0
}

func printTaskProgress(s taskindex.Status) {
	fmt.Fprintf(os.Stderr, "\rindexing %d of %d task(s)", s.Done, s.Total)
}
