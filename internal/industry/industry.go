// Package industry is the templates haas ships with: a harness set up for the
// work of one field, with skills written for it, starters, and a folder of
// sample files so the starters work on the first try.
//
// Every sample file is invented. None of it is anybody's record.
package industry

import (
	"embed"
	"encoding/json"
	"fmt"
	"path"
	"sort"

	"github.com/getlatentic/haas/internal/job"
)

//go:embed templates/*.json
var shipped embed.FS

// Template is one field's setup, as shipped.
type Template struct {
	Name     string `json:"name"`
	Title    string `json:"title"`
	Industry string `json:"industry"`
	Summary  string `json:"summary"`
	// Harnesses is the order to offer them in, the one that suits it first.
	Harnesses   []string          `json:"harnesses"`
	Sensitivity job.Sensitivity   `json:"sensitivity"`
	Servers     []string          `json:"servers,omitempty"`
	Skills      []Skill           `json:"skills"`
	Starters    []string          `json:"starters"`
	Files       map[string]string `json:"files"`
	// Check is the command that says whether work on the sample files is
	// done, run in the box after the agent. A template with one can put
	// several agents on the same task and let it decide between them.
	Check []string `json:"check,omitempty"`
}

// Skill is one of a template's skills.
type Skill struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	// Blurb is what the skill does, said to the person choosing it. The
	// description is said to the model, which chooses by it.
	Blurb       string `json:"blurb"`
	Description string `json:"description"`
	Body        string `json:"body"`
}

// All is every template, in the order they are offered.
func All() []Template {
	entries, _ := shipped.ReadDir("templates")
	var all []Template
	for _, entry := range entries {
		if held, err := read(entry.Name()); err == nil {
			all = append(all, held)
		}
	}
	// A restricted template runs only where there is a model in the building,
	// so it comes after the ones any deployment can start.
	sort.Slice(all, func(a, b int) bool {
		if all[a].Restricted() != all[b].Restricted() {
			return !all[a].Restricted()
		}
		return all[a].Industry < all[b].Industry
	})
	return all
}

// Get is one template by name.
func Get(name string) (Template, bool) {
	if !job.Named(name) {
		return Template{}, false
	}
	held, err := read(name + ".json")
	return held, err == nil
}

func read(file string) (Template, error) {
	text, err := shipped.ReadFile(path.Join("templates", file))
	if err != nil {
		return Template{}, err
	}
	var held Template
	if err := json.Unmarshal(text, &held); err != nil {
		return Template{}, fmt.Errorf("%s: %w", file, err)
	}
	return held, nil
}

// Restricted is a template whose work may not leave the building.
func (t Template) Restricted() bool { return t.Sensitivity == job.Restricted }
