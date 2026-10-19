package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/getlatentic/gusto/cmd/internal/cmdline"
	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/replay"
)

const (
	groupTask    = "Run a task"
	groupSend    = "Send and prove"
	groupRun     = "Run gusto"
	groupAccount = "Account"
	groupRecords = "Records, keys and data"
	groupOther   = "Other"
)

// program is the command table. It is built when asked for, since the commands in it call back into it to report a
// wrong use.
func program() *cmdline.Program {
	var commands []*cmdline.Command
	for _, part := range [][]*cmdline.Command{taskCommands(), sendCommands(), runCommands(), accountCommands(), recordCommands(), usageCommands(), tasksCommands(), otherCommands()} {
		commands = append(commands, part...)
	}
	return &cmdline.Program{
		Name:    "gusto",
		Tagline: "gusto runs the AI agents you already use, each in a sandbox, on a task.",
		Groups:  []string{groupTask, groupSend, groupRun, groupAccount, groupRecords, groupOther},
		Exit: []cmdline.Param{
			{Name: "0", Text: "success"},
			{Name: "1", Text: "failure, including a check that failed"},
			{Name: "2", Text: "usage error"},
		},
		Commands: commands,
	}
}

func usageError(path, problem string) int { return program().UsageError(path, problem) }

func newSet(path string) *flag.FlagSet { return flag.NewFlagSet(path, flag.ContinueOnError) }

// parseFlags is how every command reads its flags, so that -h, --help and a wrong flag are answered the same way.
func parseFlags(path string, set *flag.FlagSet, args []string) (code int, ok bool) {
	return program().ParseFlags(path, set, args)
}

// interspersed reads flags wherever they sit among the arguments, and returns the arguments.
func interspersed(path string, set *flag.FlagSet, args []string) (named []string, code int, ok bool) {
	set.SetOutput(io.Discard)
	named, err := replay.Interspersed(set, args)
	if err != nil {
		return nil, program().FlagError(path, err), false
	}
	return named, 0, true
}

// flagsOf is the table's view of a command's flags: the declaration the command parses with, run on a set of its own.
func flagsOf[T any](declare func(*flag.FlagSet) *T) func(*flag.FlagSet) {
	return func(set *flag.FlagSet) { declare(set) }
}

// none runs a command that takes no arguments.
func none(path string, run func() int) func([]string) int {
	return func(args []string) int {
		if len(args) != 0 {
			return usageError(path, "takes no arguments")
		}
		return run()
	}
}

// oneDir is the directory a command that takes only that was given.
func oneDir(path string, args []string) (job.Dir, int) {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return "", usageError(path, "needs one task directory")
	}
	return job.Dir(args[0]), 0
}

// orSetting is what a flag was given, or else what the environment says, or else the fallback. A default read from the
// environment is not declared on the flag, so that help reads the same wherever it is run.
func orSetting(given, name, fallback string) string {
	if given != "" {
		return given
	}
	return setting(name, fallback)
}

func fail(err error) int {
	fmt.Fprintf(os.Stderr, "gusto: %v\n", err)
	return 1
}

// atHome runs a subcommand of gusto local with the folder gusto keeps for this person.
func atHome(do func(home string, args []string) int) func([]string) int {
	return func(args []string) int {
		home, err := localHome()
		if err != nil {
			return fail(err)
		}
		return do(home, args)
	}
}

func homeOnly(path string, do func(home string) int) func([]string) int {
	return atHome(func(home string, args []string) int {
		if len(args) != 0 {
			return usageError(path, "takes no arguments")
		}
		return do(home)
	})
}
