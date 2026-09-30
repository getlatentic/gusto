package roster

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWhatIsWrittenIsWhatIsRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "members.json")
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	want := Roster{
		Written: now,
		Members: []Member{{Email: "ada@acme.com", Role: "admin", Joined: now, LastSignIn: now}, {Email: "bob@acme.com", Role: "runner", Joined: now}},
		Invited: []Invited{{Email: "cy@acme.com", By: "ada@acme.com", Created: now, Expires: now.Add(time.Hour)}},
	}
	if err := Write(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil || len(got.Members) != 2 || got.Members[0] != want.Members[0] || got.Members[1] != want.Members[1] || got.Invited[0] != want.Invited[0] || !got.Written.Equal(now) {
		t.Fatalf("%+v %v", got, err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("the list of people is private: %v", info.Mode())
	}
}

func TestAListThatCannotBeReadIsAnErrorAndNotAnEmptyOrganisation(t *testing.T) {
	dir := t.TempDir()
	if _, err := Read(filepath.Join(dir, "none.json")); err == nil || !strings.Contains(err.Error(), "none.json") {
		t.Errorf("a missing list: %v", err)
	}
	broken := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(broken, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(broken); err == nil || !strings.Contains(err.Error(), "not valid") {
		t.Errorf("a broken list: %v", err)
	}
}
