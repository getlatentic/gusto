package cloud

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/internal/computer"
)

// marked is the ids of the homes asked to be removed in an organization's computers directory, which is the mark the container
// backend looks for, whether or not a record of the computer exists.
func marked(t *testing.T, s *Store, slug string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(s.DataDir(slug), "computers"))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, entry := range entries {
		if id, ok := strings.CutSuffix(entry.Name(), ".retire"); ok {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

func sorted(ids ...string) []string {
	slices.Sort(ids)
	return ids
}

// Where agents run in containers a person has a home and no record of a computer, so leaving marks the home by its id
// (getlatentic/planning#830).
func TestLeavingAWorkspaceMarksTheHomeThereAndNoRecordIsNeeded(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f, ada := acme(t)
		f.invitedAndJoined(ada, "bob@acme.com")
		ada.post("/org/workspaces", url.Values{"name": {"Engineering"}})
		ada.post("/org/workspaces/add", url.Values{"workspace": {"engineering"}, "email": {"bob@acme.com"}})
		if status, _, _ := ada.post("/org/workspaces/remove", url.Values{"workspace": {"engineering"}, "email": {"bob@acme.com"}}); status != http.StatusSeeOther {
			t.Fatalf("taking bob out of engineering: %d", status)
		}
		want := computer.ID("acme-com", "engineering", "bob@acme.com")
		if got := marked(t, f.store, "acme-com"); !slices.Equal(got, []string{want}) {
			t.Errorf("marked %v, want only the home in engineering %s", got, want)
		}
	})
}

func TestRemovingAPersonMarksTheirHomeInEveryWorkspaceTheyWereInWithNoRecord(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f, ada := acme(t)
		f.invitedAndJoined(ada, "bob@acme.com")
		f.invitedAndJoined(ada, "cy@acme.com")
		ada.post("/org/workspaces", url.Values{"name": {"Engineering"}})
		ada.post("/org/workspaces/add", url.Values{"workspace": {"engineering"}, "email": {"bob@acme.com"}})
		if status, _, _ := ada.post("/org/people/remove", url.Values{"email": {"bob@acme.com"}}); status != http.StatusSeeOther {
			t.Fatalf("removing bob: %d", status)
		}
		want := sorted(computer.ID("acme-com", "", "bob@acme.com"), computer.ID("acme-com", "engineering", "bob@acme.com"))
		if got := marked(t, f.store, "acme-com"); !slices.Equal(got, want) {
			t.Errorf("marked %v, want bob's two homes %v and not cy's", got, want)
		}
	})
}

// The record of a computer in a workspace the person has since left is still marked when they leave the organization.
func TestRemovingAPersonMarksARecordedComputerInAWorkspaceTheyHadLeft(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		f, ada := acme(t)
		f.invitedAndJoined(ada, "bob@acme.com")
		registry, err := f.store.Computers("acme-com")
		if err != nil {
			t.Fatal(err)
		}
		kept, _, _ := registry.Ensure("bob@acme.com", "old-team", "local")
		if status, _, _ := ada.post("/org/people/remove", url.Values{"email": {"bob@acme.com"}}); status != http.StatusSeeOther {
			t.Fatalf("removing bob: %d", status)
		}
		retiring, _ := registry.Retiring()
		if len(retiring) != 1 || retiring[0].ID != kept.ID {
			t.Errorf("the computer in a workspace bob had left is marked: %+v", retiring)
		}
	})
}
