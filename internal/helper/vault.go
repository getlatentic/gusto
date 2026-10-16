// Package helper is what runs on a person's own computer to reach their organisation's server: it signs in once, keeps the
// token where the operating system keeps secrets, and speaks to the server's API for the person.
package helper

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
)

// ErrNotSignedIn is the answer when no token is kept for a server.
var ErrNotSignedIn = errors.New("not signed in: run `gusto login`")

// Vault keeps the token a person's computer was given, one for each server.
type Vault interface {
	Get(server string) (string, error)
	Set(server, token string) error
	Delete(server string) error
}

// Server is an address as the vault keys it: no trailing slash, and the scheme and host in lower case.
func Server(address string) string {
	address = strings.TrimRight(strings.TrimSpace(address), "/")
	scheme, rest, found := strings.Cut(address, "://")
	if !found {
		return strings.ToLower(address)
	}
	host, path, _ := strings.Cut(rest, "/")
	if path != "" {
		path = "/" + path
	}
	return strings.ToLower(scheme) + "://" + strings.ToLower(host) + path
}

// service is the name the keychain files the tokens under.
const service = "gusto"

// keychain is the operating system's own: the Keychain on macOS, the Credential Manager on Windows, the Secret Service on
// Linux.
type keychain struct{}

func (keychain) Get(server string) (string, error) {
	token, err := keyring.Get(service, Server(server))
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotSignedIn
	}
	return token, err
}

func (keychain) Set(server, token string) error { return keyring.Set(service, Server(server), token) }

func (keychain) Delete(server string) error {
	if err := keyring.Delete(service, Server(server)); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

// fileVault keeps a token in a file only its owner reads. It is for a machine with no keychain, such as a server with no
// desktop session, and for a person who asks for it with GUSTO_VAULT=file.
type fileVault struct{ dir string }

func (f fileVault) path(server string) string {
	sum := sha256.Sum256([]byte(Server(server)))
	return filepath.Join(f.dir, "token-"+hex.EncodeToString(sum[:6]))
}

func (f fileVault) Get(server string) (string, error) {
	text, err := os.ReadFile(f.path(server))
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNotSignedIn
	}
	return strings.TrimSpace(string(text)), err
}

func (f fileVault) Set(server, token string) error {
	if err := os.MkdirAll(f.dir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(f.path(server), []byte(token+"\n"), 0o600)
}

func (f fileVault) Delete(server string) error {
	if err := os.Remove(f.path(server)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// fallback keeps tokens in the keychain, and in a file when the keychain cannot be reached, saying so once.
type fallback struct {
	keys, files Vault
	warn        io.Writer
}

func (f fallback) Get(server string) (string, error) {
	token, err := f.keys.Get(server)
	if err == nil {
		return token, nil
	}
	if token, fileErr := f.files.Get(server); fileErr == nil {
		return token, nil
	}
	return "", err
}

func (f fallback) Set(server, token string) error {
	if err := f.keys.Set(server, token); err == nil {
		return nil
	} else if f.warn != nil {
		fmt.Fprintf(f.warn, "the operating system's keychain cannot be reached (%v), so the token is kept in a file only you can read\n", err)
	}
	return f.files.Set(server, token)
}

// Delete forgets the token wherever it is kept. A keychain that cannot be reached holds nothing this program put there, so
// its failure counts only when the token can still be read back from it.
func (f fallback) Delete(server string) error {
	keyErr := f.keys.Delete(server)
	if fileErr := f.files.Delete(server); fileErr != nil {
		return fileErr
	}
	if keyErr != nil {
		if _, stillThere := f.keys.Get(server); stillThere == nil {
			return keyErr
		}
	}
	return nil
}

// OpenVault is where this computer keeps tokens: the keychain, with a file beside the helper's settings when there is no
// keychain. GUSTO_VAULT=file asks for the file alone. Warnings go to warn.
func OpenVault(warn io.Writer) (Vault, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	files := fileVault{dir: filepath.Join(dir, "gusto")}
	if os.Getenv("GUSTO_VAULT") == "file" {
		return files, nil
	}
	return fallback{keys: keychain{}, files: files, warn: warn}, nil
}
