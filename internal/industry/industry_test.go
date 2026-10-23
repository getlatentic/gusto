package industry

import (
	"strings"
	"testing"

	"github.com/getlatentic/gusto/pkg/job"
)

// Every template ships whole: named so a setup can take its name, with skills
// a harness can load, starters, and sample files marked as invented.
func TestEveryTemplateIsCompleteAndItsFilesAreInvented(t *testing.T) {
	all := All()
	if len(all) != 5 {
		t.Fatalf("%d templates", len(all))
	}
	for _, held := range all {
		if !job.Named(held.Name) || held.Title == "" || len(held.Skills) == 0 || len(held.Starters) == 0 || len(held.Harnesses) == 0 {
			t.Errorf("%s is incomplete", held.Name)
		}
		if !held.Sensitivity.Known() {
			t.Errorf("%s has sensitivity %q", held.Name, held.Sensitivity)
		}
		for _, skill := range held.Skills {
			if !job.Named(skill.Name) || skill.Title == "" || skill.Blurb == "" || skill.Description == "" || skill.Body == "" {
				t.Errorf("%s: skill %q is incomplete", held.Name, skill.Name)
			}
		}
		marked := false
		for name, text := range held.Files {
			if strings.Contains(name, "..") || strings.HasPrefix(name, "/") {
				t.Errorf("%s: a file is named %q", held.Name, name)
			}
			marked = marked || strings.Contains(text, "invented")
		}
		if !marked {
			t.Errorf("%s: no sample file says it is invented", held.Name)
		}
	}
	if defense, _ := Get("defence"); !defense.Restricted() {
		t.Error("the defense template is not restricted")
	}
}
