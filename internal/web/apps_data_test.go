package web

import (
	"testing"

	"github.com/getlatentic/gusto/internal/setup"
)

// appsJSON is the data of a tab of Apps & skills.
type appsJSON struct {
	Tabs              []tab
	Tab               string
	Refused, TurnedOn string
	Apps              []appRow
	Skills            []skillRow
	Writes, Imports   bool
	Servers           []string
	Imported          []importedChoice
}

func appsPageOf(t *testing.T, here dataReader, query string) appsJSON {
	t.Helper()
	var page appsJSON
	here.data(t, appsPath+query, &page)
	return page
}

// app is one app's row of the Apps tab, by its id.
func (a appsJSON) app(t *testing.T, id string) appRow {
	t.Helper()
	for _, each := range a.Apps {
		if each.ID == id {
			return each
		}
	}
	t.Fatalf("Apps has no row for %s: %+v", id, a.Apps)
	return appRow{}
}

// skill is one row of the Skills tab, by its name.
func (a appsJSON) skill(t *testing.T, name string) skillRow {
	t.Helper()
	for _, each := range a.Skills {
		if each.Name == name {
			return each
		}
	}
	t.Fatalf("Skills has no row for %s: %+v", name, a.Skills)
	return skillRow{}
}

// integrationJSON is the data of one imported plugin's page.
type integrationJSON struct {
	Integration      integrationView
	Shown            *shownVersion
	Changes          *versionChanges
	Against, Refused string
}

func integrationPageOf(t *testing.T, here dataReader, path string) integrationJSON {
	t.Helper()
	var page integrationJSON
	here.data(t, path, &page)
	return page
}

// skillSetJSON is the data of one skill set's console page.
type skillSetJSON struct {
	Set      skillSetView
	Uses     []setup.Use
	Servers  []serverChoice
	Imported []importedChoice
	Earlier  []earlierRow
	Refused  string
}

func skillSetPageOf(t *testing.T, here dataReader, name string) skillSetJSON {
	t.Helper()
	var page skillSetJSON
	here.data(t, setupsPath+"/"+name, &page)
	return page
}
