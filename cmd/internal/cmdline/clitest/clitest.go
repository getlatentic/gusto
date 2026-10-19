// Package clitest is the checks every command line built on cmdline must pass, so that each program asserts the same
// contract in one call.
package clitest

import (
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/cmd/internal/cmdline"
)

// Capture runs fn with standard output and standard error captured.
func Capture(t *testing.T, fn func() int) (stdout, stderr string, code int) {
	t.Helper()
	outWas, errWas := os.Stdout, os.Stderr
	outRead, outWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errRead, errWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = outWrite, errWrite
	done := make(chan [2]string)
	go func() {
		out, _ := io.ReadAll(outRead)
		errs, _ := io.ReadAll(errRead)
		done <- [2]string{string(out), string(errs)}
	}()
	code = fn()
	outWrite.Close()
	errWrite.Close()
	os.Stdout, os.Stderr = outWas, errWas
	read := <-done
	return read[0], read[1], code
}

// Verify checks the program against the contract: the global help lists each command that is not hidden once, under its
// group; every command and subcommand answers --help with exit code 0, a Usage line and no line over 80 columns; a command
// that is not one is answered with the nearest that is, and exit code 2.
func Verify(t *testing.T, p *cmdline.Program) {
	t.Helper()
	t.Run("global help lists every command once under its group", func(t *testing.T) { globalHelp(t, p) })
	t.Run("every command answers --help", func(t *testing.T) { everyCommandHelps(t, p) })
	t.Run("an unknown command suggests the closest", func(t *testing.T) { suggests(t, p) })
	t.Run("the program with no arguments is a usage error", func(t *testing.T) {
		out, errs, code := Capture(t, func() int { return p.Run(nil) })
		if code != 2 || out != "" || !strings.Contains(errs, "Usage:") {
			t.Errorf("no arguments gave code %d, stdout %q, stderr %q", code, out, errs)
		}
	})
}

func globalHelp(t *testing.T, p *cmdline.Program) {
	for _, args := range [][]string{{"help"}, {"-h"}, {"--help"}} {
		out, _, code := Capture(t, func() int { return p.Run(args) })
		if code != 0 {
			t.Fatalf("%v: exit code %d", args, code)
		}
		listed := listedUnderGroups(out, p)
		for _, command := range p.Commands {
			groups := listed[command.Name]
			switch {
			case command.Hidden && len(groups) != 0:
				t.Errorf("%v: hidden command %s is listed", args, command.Name)
			case !command.Hidden && (len(groups) != 1 || groups[0] != command.Group):
				t.Errorf("%v: %s is listed under %v, want once under %q", args, command.Name, groups, command.Group)
			}
		}
		tooWide(t, strings.Join(args, " "), out)
	}
}

// listedUnderGroups reads the group each command name is printed under.
func listedUnderGroups(help string, p *cmdline.Program) map[string][]string {
	listed := map[string][]string{}
	group := ""
	known := map[string]bool{}
	for _, name := range p.Groups {
		known[name] = true
	}
	for _, line := range strings.Split(help, "\n") {
		if name := strings.TrimSuffix(line, ":"); known[name] && !strings.HasPrefix(line, " ") {
			group = name
			continue
		}
		if line == "" || !strings.HasPrefix(line, "  ") {
			if line != "" && !strings.HasPrefix(line, " ") {
				group = ""
			}
			continue
		}
		if fields := strings.Fields(line); group != "" && len(fields) > 0 {
			listed[fields[0]] = append(listed[fields[0]], group)
		}
	}
	return listed
}

var usageLine = regexp.MustCompile(`(?m)^Usage:\n  \S`)

func everyCommandHelps(t *testing.T, p *cmdline.Program) {
	for _, path := range p.Paths() {
		for _, form := range [][]string{append(strings.Fields(path), "--help"), append(strings.Fields(path), "-h"), append([]string{"help"}, strings.Fields(path)...)} {
			out, errs, code := Capture(t, func() int { return p.Run(form) })
			if code != 0 || errs != "" {
				t.Errorf("%v: exit code %d, stderr %q", form, code, errs)
			}
			if !usageLine.MatchString(out) {
				t.Errorf("%v: no Usage line in\n%s", form, out)
			}
			if !strings.HasPrefix(out, p.Name+" "+path+":") {
				t.Errorf("%v: help does not begin with the command's name:\n%s", form, out)
			}
			tooWide(t, strings.Join(form, " "), out)
		}
	}
}

func suggests(t *testing.T, p *cmdline.Program) {
	var target *cmdline.Command
	for _, command := range p.Commands {
		if !command.Hidden && len(command.Name) >= 5 {
			target = command
			break
		}
	}
	typo := target.Name[:len(target.Name)-1] + "x"
	_, errs, code := Capture(t, func() int { return p.Run([]string{typo}) })
	if code != 2 || !strings.Contains(errs, `Did you mean "`+target.Name+`"?`) || !strings.Contains(errs, "Run '"+p.Name+" help' for the commands.") {
		t.Errorf("%s gave code %d and %q", typo, code, errs)
	}
	_, errs, code = Capture(t, func() int { return p.Run([]string{"zzzzzzzz"}) })
	if code != 2 || strings.Contains(errs, "Did you mean") {
		t.Errorf("a name near nothing gave code %d and %q", code, errs)
	}
}

func tooWide(t *testing.T, what, text string) {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		if len([]rune(line)) > cmdline.Width {
			t.Errorf("%s: a line is %d columns, over %d: %q", what, len([]rune(line)), cmdline.Width, line)
		}
	}
}
