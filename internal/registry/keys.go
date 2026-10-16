package registry

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/getlatentic/gusto/internal/atomicfile"
)

// Keys is where the keys typed into the registry are kept, by provider. A shared server keeps them in files only its user can
// read; gusto on a person's own computer keeps them in the computer's keychain.
type Keys interface {
	Keep(provider, key string) error
	// Read is the key kept for a provider, or an error wrapping fs.ErrNotExist when none is.
	Read(provider string) (string, error)
	Forget(provider string) error
}

// FileKeys keeps each key in a file of its own in a folder, readable by this user alone.
type FileKeys string

// stored is what a provider's typed key is kept in.
type stored struct {
	Key string `json:"key"`
}

func (k FileKeys) file(provider string) string { return filepath.Join(string(k), provider+".json") }

func (k FileKeys) Keep(provider, key string) error {
	if err := os.MkdirAll(string(k), 0o700); err != nil {
		return err
	}
	return atomicfile.Write(k.file(provider), stored{Key: key}, 0o600)
}

func (k FileKeys) Read(provider string) (string, error) {
	text, err := os.ReadFile(k.file(provider))
	if err != nil {
		return "", err
	}
	var held stored
	if err := json.Unmarshal(text, &held); err != nil || held.Key == "" {
		return "", errors.New("the key that was kept cannot be read")
	}
	return held.Key, nil
}

func (k FileKeys) Forget(provider string) error {
	if err := os.Remove(k.file(provider)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// UseKeys keeps the keys typed from now on in keys, and reads them from there.
func (r *Registry) UseKeys(keys Keys) { r.keys = keys }
