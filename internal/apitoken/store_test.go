package apitoken_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/apitoken"
)

func opened(t *testing.T) (*apitoken.Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tokens", "tokens.json")
	store, err := apitoken.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	return store, path
}

func TestATokenIsMadeOnceAndItsSecretIsNeverKept(t *testing.T) {
	store, path := opened(t)
	token, secret, err := store.Create("ci", []string{apitoken.Submit}, "admin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(secret, apitoken.Prefix+token.ID+"_") || len(secret) < 40 {
		t.Fatalf("the secret is %q", secret)
	}
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	random := strings.TrimPrefix(secret, apitoken.Prefix+token.ID+"_")
	if strings.Contains(string(text), random) || strings.Contains(string(text), secret) {
		t.Error("the file holds the secret, and anybody who reads it can be the token")
	}
	if !strings.Contains(string(text), `"hash"`) || !strings.Contains(string(text), "admin@example.com") {
		t.Errorf("the file should hold the hash and who made it:\n%s", text)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the file is %v: %v", info.Mode(), err)
	}
}

func TestASecretChecksOutAndACloseGuessDoesNot(t *testing.T) {
	store, _ := opened(t)
	token, secret, err := store.Create("ci", []string{apitoken.Submit, apitoken.Spend}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Check(secret)
	if err != nil || got.ID != token.ID || got.Name != "ci" || !got.May(apitoken.Spend) || got.May(apitoken.Audit) {
		t.Fatalf("%+v %v", got, err)
	}
	for name, wrong := range map[string]string{
		"one character off":            secret[:len(secret)-1] + "x",
		"the right id, a wrong secret": apitoken.Prefix + token.ID + "_" + strings.Repeat("A", 43),
		"no such id":                   apitoken.Prefix + "ffffffff_" + strings.Repeat("A", 43),
		"the shared password":          "a-shared-password",
		"nothing":                      "",
		"only the prefix":              apitoken.Prefix,
	} {
		if _, err := store.Check(wrong); !errors.Is(err, apitoken.ErrUnknown) {
			t.Errorf("%s: got %v, want it unknown", name, err)
		}
	}
}

func TestARevokedTokenIsRefusedAndItsNameCanBeUsedAgain(t *testing.T) {
	store, _ := opened(t)
	token, secret, err := store.Create("ci", []string{apitoken.Submit}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Create("ci", []string{apitoken.Submit}, "admin"); err == nil {
		t.Fatal("two live tokens shared a name")
	}
	ended, err := store.Revoke(token.ID, "admin@example.com")
	if err != nil || ended.Live() || ended.RevokedBy != "admin@example.com" {
		t.Fatalf("%+v %v", ended, err)
	}
	if _, err := store.Check(secret); !errors.Is(err, apitoken.ErrRevoked) {
		t.Fatalf("a revoked token: %v", err)
	}
	if _, err := store.Revoke(token.ID, "admin"); err != nil {
		t.Errorf("revoking twice: %v", err)
	}
	if _, err := store.Revoke("00000000", "admin"); !errors.Is(err, apitoken.ErrUnknown) {
		t.Errorf("revoking one nobody made: %v", err)
	}
	if _, _, err := store.Create("ci", []string{apitoken.Submit}, "admin"); err != nil {
		t.Errorf("a name whose token was revoked is free again: %v", err)
	}
	all := store.List()
	if len(all) != 2 || all[0].Live() == false || all[1].Live() {
		t.Errorf("both tokens stay listed, the newest first, the revoked one marked: %+v", all)
	}
}

func TestATokenNeedsAPlainNameAndAtLeastOneKnownScope(t *testing.T) {
	store, _ := opened(t)
	for _, name := range []string{"", "Has Capitals", "has space", "-leading", "a/b", strings.Repeat("x", 61)} {
		if _, _, err := store.Create(name, []string{apitoken.Submit}, "admin"); !errors.Is(err, apitoken.ErrName) {
			t.Errorf("%q: %v", name, err)
		}
	}
	if _, _, err := store.Create("ci", nil, "admin"); err == nil {
		t.Error("a token that may do nothing was made")
	}
	if _, _, err := store.Create("ci", []string{"delete-everything"}, "admin"); err == nil {
		t.Error("an unknown scope was accepted")
	}
	made, _, err := store.Create("ci", []string{apitoken.Audit, apitoken.Submit, apitoken.Submit}, "admin")
	if err != nil || strings.Join(made.Scopes, ",") != "submit,audit" {
		t.Errorf("the scopes are kept once each, in order: %+v %v", made.Scopes, err)
	}
}

// Two processes share the file. A token made by one is known to the other on its
// next check, with no restart.
func TestATokenMadeByAnotherProcessIsKnownAtOnce(t *testing.T) {
	server, path := opened(t)
	other, err := apitoken.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, secret, err := other.Create("made-elsewhere", []string{apitoken.Spend}, "operator")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := server.Check(secret); err != nil || got.Name != "made-elsewhere" {
		t.Fatalf("%+v %v", got, err)
	}
	made, _, _ := other.Create("second", []string{apitoken.Submit}, "operator")
	if _, err := other.Revoke(made.ID, "operator"); err != nil {
		t.Fatal(err)
	}
	if len(server.List()) != 2 {
		t.Errorf("the server sees %d tokens, want 2", len(server.List()))
	}
}

func TestAStoreThatIsNotAListOfTokensIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tokens.json")
	if err := os.WriteFile(path, []byte(`{"not":"a list"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := apitoken.Open(path); err == nil {
		t.Fatal("a file that is not tokens was opened")
	}
}
