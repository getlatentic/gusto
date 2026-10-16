package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"
)

// Policy is what an export keeps and what it hides. It is configuration: a
// JSON file handed to `gusto export -policy`, recorded whole in the export it
// shaped, so a reader knows the rules as well as what they removed.
type Policy struct {
	// Drop names files of the export, as slash paths inside it, that are not
	// retained. A name may be a pattern, as path.Match reads one.
	Drop []string `json:"drop"`
	// Redact hides values inside the files that are retained.
	Redact Redact `json:"redact"`
}

// Redact hides values inside the retained files.
type Redact struct {
	// Fields are JSON member names whose values are replaced, in every JSON
	// and JSON-lines file the export carries.
	Fields []string `json:"fields"`
	// Patterns are replaced wherever they match text.
	Patterns []Pattern `json:"patterns"`
}

// Pattern is a regular expression with a name. The name is what the export
// writes where the match was, so the pattern itself need not be read to know
// what kind of thing was there.
type Pattern struct {
	Name  string `json:"name"`
	Match string `json:"match"`
}

// kept are the files an export cannot do without: the manifest is what the
// export verifies against, and job.json is the first thing the manifest names.
// Neither is redacted by pattern or field either, except job.json's values: a
// manifest is only hashes and names, with nothing in it to hide.
var kept = []string{SpecFile, ManifestFile}

// ReadPolicy reads a policy file. No file is the empty policy, which keeps
// everything and hides nothing.
func ReadPolicy(file string) (Policy, error) {
	if file == "" {
		return Policy{}, nil
	}
	text, err := os.ReadFile(file)
	if err != nil {
		return Policy{}, err
	}
	var policy Policy
	if err := json.Unmarshal(text, &policy); err != nil {
		return Policy{}, fmt.Errorf("%s: %w", file, err)
	}
	return policy, policy.check()
}

// check refuses a policy that would make the export unverifiable or that
// could not be applied, before anything is copied.
func (p Policy) check() error {
	for _, pattern := range p.Drop {
		if _, err := path.Match(pattern, ""); err != nil {
			return fmt.Errorf("drop %q: %w", pattern, err)
		}
		for _, name := range kept {
			if matches(pattern, name) {
				return fmt.Errorf("drop %q would remove %s, which the export verifies against", pattern, name)
			}
		}
	}
	for _, pattern := range p.Redact.Patterns {
		if pattern.Name == "" {
			return fmt.Errorf("the pattern %q needs a name to write in its place", pattern.Match)
		}
		if _, err := regexp.Compile(pattern.Match); err != nil {
			return fmt.Errorf("pattern %s: %w", pattern.Name, err)
		}
	}
	return nil
}

// written is the policy as audit.json records it, with an empty list where
// the file left one out, so a reader sees "nothing" rather than a gap.
func (p Policy) written() Policy {
	return Policy{
		Drop:   nonNil(p.Drop),
		Redact: Redact{Fields: nonNil(p.Redact.Fields), Patterns: nonNil(p.Redact.Patterns)},
	}
}

// drops says which drop rule removes a file, if one does.
func (p Policy) drops(name string) (string, bool) {
	for _, pattern := range p.Drop {
		if matches(pattern, name) {
			return "drop " + pattern, true
		}
	}
	return "", false
}

func matches(pattern, name string) bool {
	found, _ := path.Match(pattern, name)
	return found
}
