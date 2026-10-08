package redact

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Config is an organisation's choice: the kinds to mask, in a file an administrator changes.
type Config struct {
	Patterns []string `json:"patterns"`
}

// ReadConfig reads the file and makes the redactor it asks for. No file, or no patterns, masks nothing. A field the file names that
// this does not know is refused, since a misspelt one would otherwise be no protection.
func ReadConfig(path string) (*Redactor, error) {
	if path == "" {
		return nil, nil
	}
	text, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var config Config
	decoder := json.NewDecoder(bytes.NewReader(text))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("%s is not a redaction file: %w", path, err)
	}
	return New(config.Patterns...)
}
