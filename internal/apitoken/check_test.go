package apitoken

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTokensThatShareAnIdAreEachChecked(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	shared := []Token{
		{ID: "0a0b0c0d", Name: "first", Scopes: []string{Submit}, Hash: digest("one"), Made: 1, By: "the test"},
		{ID: "0a0b0c0d", Name: "second", Scopes: []string{Spend}, Hash: digest("two"), Made: 2, By: "the test"},
	}
	text, _ := json.Marshal(shared)
	if err := os.WriteFile(path, text, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if token, err := store.Check(Prefix + "0a0b0c0d_two"); err != nil || token.Name != "second" {
		t.Errorf("the second token with a shared id was refused: %+v %v", token, err)
	}
	made, _, err := store.Create("third", []string{Audit}, "the test")
	if err != nil || len(made.ID) != 2*idBytes {
		t.Errorf("a new token's id is %q: %v", made.ID, err)
	}
}

func TestATokenFileThatCannotBeReadIsNotBelievedOrWrittenOver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, secret, err := store.Create("ci", []string{Submit}, "the test")
	if err != nil {
		t.Fatal(err)
	}
	broken := []byte("[{\"id\": torn")
	if err := os.WriteFile(path, broken, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Check(secret); !errors.Is(err, ErrUnread) {
		t.Errorf("a token was believed from a file that cannot be read: %v", err)
	}
	if text, _ := os.ReadFile(path); string(text) != string(broken) {
		t.Errorf("the file was written over: %s", text)
	}
}

func TestAComputersTokenUnusedForLongStopsWorking(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "tokens.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	store.now = func() time.Time { return now }
	_, secret, err := store.CreateFor("ada@acme.com", "laptop")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(WorkIdle - time.Hour)
	if _, err := store.Check(secret); err != nil {
		t.Fatalf("a token used within its time was refused: %v", err)
	}
	now = now.Add(WorkIdle - time.Hour)
	if _, err := store.Check(secret); err != nil {
		t.Errorf("a token in use was refused: %v", err)
	}
	now = now.Add(WorkIdle + time.Hour)
	if _, err := store.Check(secret); !errors.Is(err, ErrIdle) {
		t.Errorf("a token unused for longer than %v works: %v", WorkIdle, err)
	}
}
