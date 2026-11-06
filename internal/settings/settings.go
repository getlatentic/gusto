// Package settings declares the GUSTO_ settings a command reads, each once, with its type, its default and a line saying what
// it does, and reads them all when the command starts. A value that does not parse stops the command with the setting's name,
// rather than leaving it at its default. The declarations are also what the command's help and docs/CLI.md list.
package settings

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Set is the settings one command reads, in the order they were declared.
type Set struct {
	entries []*entry
	byName  map[string]*entry
}

type entry struct {
	name string
	help string
	// shown is the default as help says it, or nothing when there is none worth saying.
	shown  string
	secret bool
	// reset puts the default in the bound field, and parse the value that was set.
	reset func()
	parse func(value string) error
	raw   string
}

// Param is a setting as a command's help lists it.
type Param struct{ Name, Text string }

// New is a set with nothing declared.
func New() *Set { return &Set{byName: map[string]*entry{}} }

// declare adds a setting. A name declared twice, or with no help, is a mistake in gusto's own code, so it panics, as the flag
// package does for a flag defined twice.
func (s *Set) declare(e *entry) {
	if e.help == "" {
		panic("settings: " + e.name + " has no help")
	}
	if _, twice := s.byName[e.name]; twice {
		panic("settings: " + e.name + " is declared twice")
	}
	s.entries = append(s.entries, e)
	s.byName[e.name] = e
}

// String is a setting of any text, with fallback when it is not set.
func (s *Set) String(into *string, name, fallback, help string) {
	s.declare(&entry{name: name, help: help, shown: fallback,
		reset: func() { *into = fallback },
		parse: func(value string) error { *into = value; return nil },
	})
}

// Secret is a setting whose value is never printed: not in help, not in an error, not in a record of what changed.
func (s *Set) Secret(into *string, name, help string) {
	s.declare(&entry{name: name, help: help, secret: true,
		reset: func() { *into = "" },
		parse: func(value string) error { *into = value; return nil },
	})
}

// Int is a whole number no smaller than least, with fallback when it is not set.
func (s *Set) Int(into *int, name string, fallback, least int, help string) {
	s.declare(&entry{name: name, help: help, shown: strconv.Itoa(fallback),
		reset: func() { *into = fallback },
		parse: func(value string) error {
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return errors.New("it is a whole number")
			}
			if n < least {
				return fmt.Errorf("it is %d or more", least)
			}
			*into = n
			return nil
		},
	})
}

// Bool is a setting that is on or off, and off when it is not set. 1, true, on and yes are on; 0, false, off and no are off.
func (s *Set) Bool(into *bool, name, help string) {
	s.declare(&entry{name: name, help: help,
		reset: func() { *into = false },
		parse: func(value string) error {
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "1", "true", "on", "yes":
				*into = true
			case "0", "false", "off", "no":
				*into = false
			default:
				return errors.New("it is on or off: 1 or 0, true or false, on or off, yes or no")
			}
			return nil
		},
	})
}

// Choice is one of choices, with fallback when it is not set.
func (s *Set) Choice(into *string, name, fallback string, choices []string, help string) {
	s.declare(&entry{name: name, help: help + " (" + strings.Join(choices, ", ") + ")", shown: fallback,
		reset: func() { *into = fallback },
		parse: func(value string) error {
			if !slices.Contains(choices, value) {
				return errors.New("it is one of " + strings.Join(choices, ", "))
			}
			*into = value
			return nil
		},
	})
}

// List is a setting written as one comma-separated line, which is how a systemd unit carries a list. Blank members are left
// out, so a trailing comma does not become one.
func (s *Set) List(into *[]string, name, help string) {
	s.declare(&entry{name: name, help: help,
		reset: func() { *into = nil },
		parse: func(value string) error {
			*into = Split(value)
			return nil
		},
	})
}

// Split is a comma-separated line as its members, trimmed, with blank ones left out.
func Split(line string) []string {
	var members []string
	for _, each := range strings.Split(line, ",") {
		if trimmed := strings.TrimSpace(each); trimmed != "" {
			members = append(members, trimmed)
		}
	}
	return members
}

// Load fills every setting from lookup, which is os.Getenv for a command and a map in a test. A setting not set is given its
// default. Every value that does not parse is said, by name, in the one error.
func (s *Set) Load(lookup func(name string) string) error {
	var problems []error
	for _, e := range s.entries {
		e.reset()
		e.raw = lookup(e.name)
		if e.raw == "" {
			continue
		}
		if err := e.parse(e.raw); err != nil {
			problems = append(problems, e.problem(err))
		}
	}
	return errors.Join(problems...)
}

func (e *entry) problem(err error) error {
	if e.secret {
		return fmt.Errorf("%s is not right: %w", e.name, err)
	}
	return fmt.Errorf("%s is %q, and %w", e.name, e.raw, err)
}

// Raw is what a setting was set to when the set was loaded, or nothing for one not set or declared secret.
func (s *Set) Raw(name string) string {
	e, found := s.byName[name]
	if !found || e.secret {
		return ""
	}
	return e.raw
}

// Params is every setting as help lists it, with its default, unless the help already says what it is.
func (s *Set) Params() []Param {
	params := make([]Param, 0, len(s.entries))
	for _, e := range s.entries {
		text := e.help
		if e.shown != "" && !strings.Contains(text, "(default ") {
			text += " (default " + e.shown + ")"
		}
		params = append(params, Param{Name: e.name, Text: text})
	}
	return params
}

// Over is lookup with the values in over read first, for a command that decides some settings itself and leaves the rest to
// lookup.
func Over(over map[string]string, lookup func(name string) string) func(name string) string {
	return func(name string) string {
		if value, decided := over[name]; decided {
			return value
		}
		return lookup(name)
	}
}
