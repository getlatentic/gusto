package helper

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/filelock"
)

// A helper that is starting holds its lock before it says where it is, so a second started at the same moment refuses rather
// than both running, and the first's file is not taken away by the one that ends.
func TestAHelperStartedBesideOneThatIsStartingRefuses(t *testing.T) {
	o := newOrganization(t)
	vault := &memoryVault{}
	vault.Set(o.URL, "good")
	dir := t.TempDir()
	starting, err := filelock.Try(filepath.Join(dir, "helper.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer starting.Release()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	second := &Daemon{Server: o.URL, Vault: vault, Dir: dir}
	if err := second.Run(ctx); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Errorf("a helper started beside one that is starting said %v", err)
	}
}

// failing is a keychain that cannot be reached until it is back, and keeps what it held before.
type failing struct {
	memoryVault
	down bool
}

func (f *failing) Get(server string) (string, error) {
	if f.down {
		return "", errors.New("the keychain is locked")
	}
	return f.memoryVault.Get(server)
}

func (f *failing) Set(server, token string) error {
	if f.down {
		return errors.New("the keychain is locked")
	}
	return f.memoryVault.Set(server, token)
}

// A sign-in that fell back to the file is the one used when the keychain comes back with an older token.
func TestATokenKeptInTheFileWinsOverAnOlderOneInTheKeychain(t *testing.T) {
	keys := &failing{}
	keys.memoryVault.Set("https://acme.example", "old")
	vault := fallback{keys: keys, files: fileVault{dir: t.TempDir()}}
	keys.down = true
	if err := vault.Set("https://acme.example", "new"); err != nil {
		t.Fatal(err)
	}
	keys.down = false
	if token, err := vault.Get("https://acme.example"); err != nil || token != "new" {
		t.Errorf("the token read back is %q, %v", token, err)
	}
	if err := vault.Set("https://acme.example", "newer"); err != nil {
		t.Fatal(err)
	}
	if token, _ := vault.Get("https://acme.example"); token != "newer" {
		t.Errorf("a token the keychain took is read back as %q", token)
	}
}
