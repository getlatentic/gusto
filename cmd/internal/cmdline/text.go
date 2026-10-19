package cmdline

import (
	"fmt"
	"io"
	"strings"
)

// Width is the most columns any help line takes.
const Width = 80

const labelLimit = 26

func wrap(text string, width int) []string {
	var lines []string
	for _, paragraph := range strings.Split(text, "\n\n") {
		line := ""
		for _, word := range strings.Fields(paragraph) {
			switch {
			case line == "":
				line = word
			case len(line)+1+len(word) <= width:
				line += " " + word
			default:
				lines = append(lines, line)
				line = word
			}
		}
		lines = append(lines, line)
		lines = append(lines, "")
	}
	return lines[:len(lines)-1]
}

// writeUsage breaks a usage line that is too long at a space outside any bracket, and indents the rest under it.
func writeUsage(w io.Writer, line string) {
	prefix := "  "
	for len(prefix)+len(line) > Width {
		cut := breakAt(line, Width-len(prefix))
		if cut <= 0 {
			break
		}
		fmt.Fprintf(w, "%s%s\n", prefix, line[:cut])
		line, prefix = line[cut+1:], "      "
	}
	fmt.Fprintf(w, "%s%s\n", prefix, line)
}

// breakAt is the last space within limit that is not inside [] or <>, or -1.
func breakAt(line string, limit int) int {
	depth, cut := 0, -1
	for i := 0; i < len(line) && i <= limit; i++ {
		switch line[i] {
		case '[', '<':
			depth++
		case ']', '>':
			depth--
		case ' ':
			if depth == 0 {
				cut = i
			}
		}
	}
	return cut
}

func writeBlock(w io.Writer, indent string, text string) {
	for _, line := range wrap(text, Width-len(indent)) {
		if line == "" {
			fmt.Fprintln(w)
			continue
		}
		fmt.Fprintln(w, indent+line)
	}
}

// writeParams lays out labels and their sentences in two columns, with the sentence under the label when the label is long.
func writeParams(w io.Writer, params []Param) {
	column := 0
	for _, param := range params {
		if n := len(param.Name); n <= labelLimit && n > column {
			column = n
		}
	}
	for _, param := range params {
		if len(param.Name) > labelLimit {
			fmt.Fprintf(w, "  %s\n", param.Name)
			writeBlock(w, strings.Repeat(" ", 2+column+2), param.Text)
			continue
		}
		lines := wrap(param.Text, Width-2-column-2)
		fmt.Fprintf(w, "  %-*s  %s\n", column, param.Name, lines[0])
		for _, line := range lines[1:] {
			fmt.Fprintf(w, "  %*s  %s\n", column, "", line)
		}
	}
}

func writeSection(w io.Writer, title string, params []Param) {
	if len(params) == 0 {
		return
	}
	fmt.Fprintf(w, "\n%s:\n", title)
	writeParams(w, params)
}

func writeText(w io.Writer, d doc) {
	writeBlock(w, "", d.Path+": "+d.Summary)
	fmt.Fprint(w, "\nUsage:\n")
	for _, line := range d.Usage {
		writeUsage(w, line)
	}
	if d.Description != "" {
		fmt.Fprintln(w)
		writeBlock(w, "", d.Description)
	}
	writeSection(w, "Arguments", d.Args)
	writeSection(w, "Commands", d.Subs)
	writeSection(w, "Flags", d.Flags)
	writeSection(w, "Environment", d.Env)
	if len(d.Examples) > 0 {
		fmt.Fprintln(w, "\nExamples:")
		for _, example := range d.Examples {
			fmt.Fprintf(w, "  %s\n", example)
		}
	}
	writeSection(w, "Exit codes", d.Exit)
	if len(d.SeeAlso) > 0 {
		fmt.Fprintf(w, "\nSee also: %s\n", strings.Join(d.SeeAlso, ", "))
	}
}

// writeGlobal is the help of the program as a whole: its commands, grouped by purpose.
func (p *Program) writeGlobal(w io.Writer) {
	fmt.Fprintf(w, "%s\n\nUsage:\n  %s <command> [arguments]\n  %s help <command> [<subcommand>]\n", p.Tagline, p.Name, p.Name)
	column := 0
	for _, command := range visible(p.Commands) {
		column = max(column, len(command.Name))
	}
	for _, group := range p.Groups {
		fmt.Fprintf(w, "\n%s:\n", group)
		for _, command := range visible(p.Commands) {
			if command.Group == group {
				fmt.Fprintf(w, "  %-*s  %s\n", column, command.Name, command.Summary)
			}
		}
	}
	fmt.Fprintf(w, "\nFlags:\n")
	writeParams(w, []Param{helpFlag, {"--version", "print the release"}})
	writeSection(w, "Exit codes", p.Exit)
	fmt.Fprintln(w)
	writeBlock(w, "", fmt.Sprintf("Run '%s help <command>' for a command's flags, environment and examples.", p.Name))
	writeBlock(w, "", fmt.Sprintf("Run '%s help --markdown' for the whole reference.", p.Name))
}
