package keychain

import (
	"errors"
	"io/fs"
	"os"
	"testing"
)

// The tests keep a key in this computer's real keychain, which a machine with no desktop session cannot open, so they run
// when asked: GUSTO_TEST_KEYCHAIN=1.
func TestAKeyIsKeptReadAndForgotten(t *testing.T) {
	if os.Getenv("GUSTO_TEST_KEYCHAIN") != "1" {
		t.Skip("set GUSTO_TEST_KEYCHAIN=1 to use this computer's keychain")
	}
	store, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	const provider = "gusto-test-provider"
	t.Cleanup(func() { store.Forget(provider) })
	if err := store.Keep(provider, "sk-first"); err != nil {
		t.Fatal(err)
	}
	if err := store.Keep(provider, "sk-second with a space"); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Read(provider); err != nil || got != "sk-second with a space" {
		t.Errorf("read %q, %v", got, err)
	}
	if err := store.Forget(provider); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(provider); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a forgotten key is still there: %v", err)
	}
	if err := store.Forget(provider); err != nil {
		t.Errorf("forgetting a key twice is an error: %v", err)
	}
}
