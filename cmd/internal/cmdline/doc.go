package cmdline

import (
	"flag"
	"fmt"
	"strings"
)

// doc is everything help says of one command, before it is laid out as text or Markdown.
type doc struct {
	Path        string
	Summary     string
	Usage       []string
	Description string
	Args        []Param
	Subs        []Param
	Flags       []Param
	Env         []Param
	Exit        []Param
	Examples    []string
	SeeAlso     []string
}

var helpFlag = Param{"-h, --help", "show this help"}

func (p *Program) describe(chain []*Command) doc {
	command := chain[len(chain)-1]
	path := p.pathOf(chain)
	d := doc{
		Path: path, Summary: command.Summary, Description: command.Description,
		Args: command.Args, Env: command.Env, Examples: command.Examples,
		Exit: command.Exit, Usage: usageLines(path, command),
	}
	if d.Exit == nil {
		d.Exit = p.Exit
	}
	for _, sub := range visible(command.Subs) {
		d.Subs = append(d.Subs, Param{sub.Name, sub.Summary})
	}
	d.Flags = append(flagParams(path, command), helpFlag)
	for _, other := range command.SeeAlso {
		d.SeeAlso = append(d.SeeAlso, p.Name+" "+other)
	}
	return d
}

func usageLines(path string, command *Command) []string {
	usage := command.Usage
	if len(usage) == 0 && len(command.Subs) > 0 {
		usage = []string{"<command> [arguments]"}
	}
	if len(usage) == 0 {
		usage = []string{""}
	}
	lines := make([]string, len(usage))
	for i, rest := range usage {
		lines[i] = strings.TrimSpace(path + " " + rest)
	}
	return lines
}

func flagParams(path string, command *Command) []Param {
	if command.Flags == nil {
		return nil
	}
	set := flag.NewFlagSet(path, flag.ContinueOnError)
	command.Flags(set)
	var params []Param
	set.VisitAll(func(f *flag.Flag) {
		kind, text := flag.UnquoteUsage(f)
		label := "-" + f.Name
		if kind != "" {
			label += " <" + kind + ">"
		}
		if f.DefValue != "" && f.DefValue != "false" {
			text += fmt.Sprintf(" (default %s)", f.DefValue)
		}
		params = append(params, Param{label, text})
	})
	return params
}
