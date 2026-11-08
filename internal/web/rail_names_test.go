package web

import (
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/instance"
)

func project(id, name string, started int64) yours {
	return yours{agent: agent{Instance: instance.Instance{ID: id, Started: started}, Name: name}}
}

func TestProjectsThatShareANameAreToldApartByWhenTheyWereStarted(t *testing.T) {
	first := time.Date(2026, 10, 26, 9, 5, 0, 0, time.Local).Unix()
	second := time.Date(2026, 10, 27, 14, 30, 0, 0, time.Local).Unix()
	used := []yours{
		project("a", "OpenCode", first),
		project("b", "OpenCode", second),
		project("c", "Claude Code", first),
	}
	started := startedWhereNamedAlike(used)
	if started["a"] != "26 Oct 09:05" || started["b"] != "27 Oct 14:30" {
		t.Errorf("two projects of one name carry when each was started: %v", started)
	}
	if _, told := started["c"]; told {
		t.Errorf("a project whose name is its own carries no date: %v", started)
	}
	names := projectNames(used)
	if names["a"] != "OpenCode · 26 Oct 09:05" || names["c"] != "Claude Code" {
		t.Errorf("in one line, a shared name is followed by its date and a unique name is left alone: %v", names)
	}
}
