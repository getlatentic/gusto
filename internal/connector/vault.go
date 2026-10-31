package connector

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// vault keeps small documents sealed on disk with a key kept beside them: a person's tokens, and a client an app registered. The
// key is made the first time and readable by this server's user alone, so a copy of the folder without it reads nothing.
type vault struct {
	dir  string
	aead cipher.AEAD
}

func openVault(dir string) (*vault, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	keyPath := filepath.Join(dir, "key")
	key, err := os.ReadFile(keyPath)
	if errors.Is(err, fs.ErrNotExist) {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := writeWhole(keyPath, key); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("%s is not a key", keyPath)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &vault{dir: dir, aead: aead}, nil
}

// put seals a document at a name inside the vault. The name is bound to what is sealed, so a file moved to another name opens to nothing.
func (v *vault) put(name string, document any) error {
	plain, err := json.Marshal(document)
	if err != nil {
		return err
	}
	nonce := make([]byte, v.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	path := filepath.Join(v.dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return writeWhole(path, v.aead.Seal(nonce, nonce, plain, []byte(name)))
}

// get opens what was sealed at a name, and says false when nothing is there.
func (v *vault) get(name string, into any) (bool, error) {
	sealed, err := os.ReadFile(filepath.Join(v.dir, name))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	size := v.aead.NonceSize()
	if len(sealed) < size {
		return false, fmt.Errorf("%s is not sealed here", name)
	}
	plain, err := v.aead.Open(nil, sealed[:size], sealed[size:], []byte(name))
	if err != nil {
		return false, fmt.Errorf("%s does not open with this vault's key", name)
	}
	return true, json.Unmarshal(plain, into)
}

func (v *vault) remove(name string) error {
	err := os.Remove(filepath.Join(v.dir, name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// writeWhole puts bytes at path by renaming a finished file into place, readable by this user alone, so a reader never sees half.
func writeWhole(path string, data []byte) error {
	temp, err := os.CreateTemp(filepath.Dir(path), ".write-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}
