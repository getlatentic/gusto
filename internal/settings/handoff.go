package settings

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// FileHolds says a settings file may hold a setting: one a command declares that is not a secret, and not under an old name.
// What it refuses stays in the environment of a process another starts.
func FileHolds(name string) bool { return fileMay(name) == nil }

// ReadFile is the settings a file WriteFile wrote holds, by name, as text. The file was checked when it was written, by the
// process that wrote it, which knew every setting; the process that reads it may not.
func ReadFile(path string) (map[string]string, error) {
	var keyed map[string]string
	if _, err := toml.DecodeFile(path, &keyed); err != nil {
		return nil, err
	}
	values := make(map[string]string, len(keyed))
	for key, value := range keyed {
		values[nameOf(key)] = value
	}
	return values, nil
}

// WriteFile writes settings, by name, to a settings file only its owner can read, all at once: a process that reads it while
// it is written reads the whole of the old file or of the new.
func WriteFile(path string, values map[string]string) error {
	keyed := make(map[string]string, len(values))
	for name, value := range values {
		keyed[Key(name)] = value
	}
	var text bytes.Buffer
	if err := toml.NewEncoder(&text).Encode(keyed); err != nil {
		return err
	}
	written, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(written.Name())
	if _, err := written.Write(text.Bytes()); err != nil {
		written.Close()
		return err
	}
	if err := written.Close(); err != nil {
		return err
	}
	return os.Rename(written.Name(), path)
}
