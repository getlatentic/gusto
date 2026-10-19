package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/dbkit"
)

// fixtureNow is the moment the fixture of the old layout was made at: its clock when the earlier release wrote it.
var fixtureNow = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

var fixtureSettings = Settings{AllowFreeMail: true, Demo: DemoSettings{
	Length: 30 * time.Minute, Extend: 10 * time.Minute, MaxOrgs: 5, Patience: 10 * time.Minute, PerDay: 40, PerPerson: 2, PerCompany: 6}}

// oldLayout copies the files the earlier release wrote, which testdata/oldlayout holds, to a root of its own.
// The files were made by that release's code with a fixed clock (testdata/oldlayout-generator.go.txt), and
// manifest.json holds the secrets of the links it made.
func oldLayout(t testing.TB) (root string, secrets map[string]string) {
	t.Helper()
	root = filepath.Join(t.TempDir(), "cloud")
	copyTree(t, filepath.Join("testdata", "oldlayout", "cloud"), root)
	text, err := os.ReadFile(filepath.Join("testdata", "oldlayout", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(text, &secrets); err != nil {
		t.Fatal(err)
	}
	return root, secrets
}

func copyTree(t testing.TB, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, path)
		if entry.IsDir() {
			return os.MkdirAll(filepath.Join(to, rel), 0o700)
		}
		text, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(to, rel), text, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// digests is every file under root and what it holds, to show that nothing was changed.
func digests(t testing.TB, root string) map[string][32]byte {
	t.Helper()
	out := map[string][32]byte{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		text, err := os.ReadFile(path)
		rel, _ := filepath.Rel(root, path)
		out[rel] = sha256.Sum256(text)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// importedAt makes a store on a database and imports the root into it as if the time were now.
func importedAt(t testing.TB, db *dbkit.DB, root string, now time.Time) (*Store, ImportRecord) {
	t.Helper()
	s, err := newStore(db, root, fixtureSettings)
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now }
	record, ran, err := s.importOnce(context.Background())
	if err != nil || !ran {
		t.Fatalf("ran %v: %v", ran, err)
	}
	return s, record
}
