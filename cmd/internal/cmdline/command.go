// Package cmdline is the command table a program's help, its reference and its dispatch are all made from, so that what
// the program does and what it says it does are written once.
package cmdline

import (
	"flag"
	"strings"
)

// Param is a named thing with a sentence about it: an argument, a flag, an environment variable, an exit code.
type Param struct{ Name, Text string }

// Command is one entry in the table. Usage lines are what follows the command's name, and an empty one stands for
// the bare name. Flags declares the command's flags on a set, which help reads the names, types and defaults from.
type Command struct {
	Name        string
	Group       string
	Summary     string
	Usage       []string
	Description string
	Args        []Param
	Flags       func(*flag.FlagSet)
	Env         []Param
	Examples    []string
	Exit        []Param
	SeeAlso     []string
	Subs        []*Command
	Run         func(args []string) int
	Hidden      bool
}

// Program is a command line: its name, how it introduces itself, the groups its commands are listed in, and the commands.
type Program struct {
	Name     string
	Tagline  string
	Groups   []string
	Commands []*Command
	Exit     []Param
}

func lookup(commands []*Command, name string) *Command {
	for _, command := range commands {
		if command.Name == name {
			return command
		}
	}
	return nil
}

// chainOf is the commands a path such as "pr open" names, outermost first, or nil when any part is not a command.
func (p *Program) chainOf(path string) []*Command {
	var chain []*Command
	level := p.Commands
	for _, name := range strings.Fields(path) {
		command := lookup(level, name)
		if command == nil {
			return nil
		}
		chain = append(chain, command)
		level = command.Subs
	}
	return chain
}

func (p *Program) pathOf(chain []*Command) string {
	names := []string{p.Name}
	for _, command := range chain {
		names = append(names, command.Name)
	}
	return strings.Join(names, " ")
}

func visible(commands []*Command) []*Command {
	var shown []*Command
	for _, command := range commands {
		if !command.Hidden {
			shown = append(shown, command)
		}
	}
	return shown
}

func names(commands []*Command) []string {
	var list []string
	for _, command := range visible(commands) {
		list = append(list, command.Name)
	}
	return list
}
