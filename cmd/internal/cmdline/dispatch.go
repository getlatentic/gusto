package cmdline

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// Run is the program: it finds the command the arguments name and runs it, and answers help, a wrong name and a wrong
// use itself. Help asked for goes to standard output and succeeds; help given because of a mistake goes to standard error
// and is a usage error, exit code 2.
func (p *Program) Run(args []string) int {
	if len(args) == 0 {
		p.writeGlobal(os.Stderr)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "-help":
		p.writeGlobal(os.Stdout)
		return 0
	case "--version":
		args = []string{"version"}
	}
	command := lookup(p.Commands, args[0])
	if command == nil {
		return p.unknown("command", "", args[0], names(p.Commands), "")
	}
	return p.dispatch([]*Command{command}, args[1:])
}

func (p *Program) dispatch(chain []*Command, rest []string) int {
	command := chain[len(chain)-1]
	if len(command.Subs) > 0 && len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		if sub := lookup(command.Subs, rest[0]); sub != nil {
			return p.dispatch(append(chain, sub), rest[1:])
		}
		return p.unknown("subcommand", strings.TrimPrefix(p.pathOf(chain), p.Name+" "), rest[0], names(command.Subs), p.pathOf(chain))
	}
	if asksForHelp(rest) {
		p.writeText(os.Stdout, chain)
		return 0
	}
	if command.Run == nil {
		return p.usageError(chain, "needs one of: "+strings.Join(names(command.Subs), ", "))
	}
	return command.Run(rest)
}

// asksForHelp is whether -h or --help comes before a bare --, which ends the flags and begins what is not one.
func asksForHelp(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "--":
			return false
		case "-h", "--help", "-help":
			return true
		}
	}
	return false
}

func (p *Program) writeText(w io.Writer, chain []*Command) { writeText(w, p.describe(chain)) }

// unknown says a name is not a command, or a subcommand of one, and the nearest that is.
func (p *Program) unknown(kind, of, typed string, candidates []string, helpPath string) int {
	message := fmt.Sprintf("%s: unknown %s %q", p.Name, kind, typed)
	if of != "" {
		message += " of " + p.Name + " " + of
	}
	if near, found := closest(typed, candidates); found {
		message += fmt.Sprintf(". Did you mean %q?", near)
	} else {
		message += "."
	}
	more := p.Name + " help"
	if helpPath != "" {
		more += " " + strings.TrimPrefix(helpPath, p.Name+" ")
	}
	fmt.Fprintf(os.Stderr, "%s\nRun '%s' for the %ss.\n", message, more, kind)
	return 2
}

// UsageError tells a person a command was used wrongly: what was wrong, if it is known, the command's usage line, and where
// to read more. The path names the command, as "pr open".
func (p *Program) UsageError(path, problem string) int {
	return p.usageError(p.chainOf(path), problem)
}

func (p *Program) usageError(chain []*Command, problem string) int {
	d := p.describe(chain)
	if problem != "" {
		fmt.Fprintf(os.Stderr, "%s: %s\n", d.Path, problem)
	}
	fmt.Fprintf(os.Stderr, "usage: %s\nRun '%s help %s' for details.\n", d.Usage[0], p.Name, strings.TrimPrefix(d.Path, p.Name+" "))
	return 2
}

// ParseFlags parses a command's flags the same way for every command: -h and --help print the command's help and succeed,
// and a flag that is not the command's prints its usage and is a usage error. When ok is false the code is the command's
// answer.
func (p *Program) ParseFlags(path string, set *flag.FlagSet, args []string) (code int, ok bool) {
	set.SetOutput(io.Discard)
	if err := set.Parse(args); err != nil {
		return p.FlagError(path, err), false
	}
	return 0, true
}

// FlagError is the answer to a flag set that did not parse: the help that was asked for, or the usage of a flag that is not
// the command's.
func (p *Program) FlagError(path string, err error) int {
	if errors.Is(err, flag.ErrHelp) {
		p.writeText(os.Stdout, p.chainOf(path))
		return 0
	}
	return p.UsageError(path, err.Error())
}

// HelpCommand is the help command: no arguments for the whole program, --markdown for the reference, or a command and
// perhaps its subcommand.
func (p *Program) HelpCommand(args []string) int {
	switch {
	case len(args) == 0:
		p.writeGlobal(os.Stdout)
		return 0
	case len(args) == 1 && (args[0] == "--markdown" || args[0] == "-markdown"):
		p.WriteMarkdown(os.Stdout)
		return 0
	}
	chain := p.chainOf(strings.Join(args, " "))
	if chain == nil {
		return p.unknown("command", "", strings.Join(args, " "), names(p.Commands), "")
	}
	p.writeText(os.Stdout, chain)
	return 0
}

// WriteGlobal prints the help of the program as a whole.
func (p *Program) WriteGlobal(w io.Writer) { p.writeGlobal(w) }

// Help prints the help of the command a path names.
func (p *Program) Help(w io.Writer, path string) bool {
	chain := p.chainOf(path)
	if chain != nil {
		p.writeText(w, chain)
	}
	return chain != nil
}

// Paths is every command and subcommand, as "pr open", hidden ones included.
func (p *Program) Paths() []string {
	var paths []string
	var walk func(prefix string, commands []*Command)
	walk = func(prefix string, commands []*Command) {
		for _, command := range commands {
			paths = append(paths, strings.TrimSpace(prefix+" "+command.Name))
			walk(prefix+" "+command.Name, command.Subs)
		}
	}
	walk("", p.Commands)
	return paths
}
