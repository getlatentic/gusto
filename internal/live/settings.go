package live

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

// Settings are what an organisation's administrator changes in GUSTO_LIVE: how many jobs a person and a team may run at once, and
// which models people are offered. A field the file leaves out keeps the value the server started with.
type Settings struct {
	LimitPerson *int     `json:"limitPerson,omitempty"`
	LimitTeam   *int     `json:"limitTeam,omitempty"`
	Models      []string `json:"models,omitempty"`
}

// ReadSettings reads the file. No file is no settings. A field the file names that this build does not know is refused, because a
// misspelt limit would otherwise be no limit.
func ReadSettings(path string) (Settings, error) {
	if path == "" {
		return Settings{}, nil
	}
	text, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	var settings Settings
	decoder := json.NewDecoder(bytes.NewReader(text))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&settings); err != nil {
		return Settings{}, fmt.Errorf("%s is not a settings file: %w", path, err)
	}
	for name, limit := range map[string]*int{"limitPerson": settings.LimitPerson, "limitTeam": settings.LimitTeam} {
		if limit != nil && *limit < 0 {
			return Settings{}, fmt.Errorf("%s: %s is below nothing", path, name)
		}
	}
	for _, model := range settings.Models {
		if strings.TrimSpace(model) == "" || strings.ContainsAny(model, " \t\n,") {
			return Settings{}, fmt.Errorf("%s: %q is not a model's name", path, model)
		}
	}
	return settings, nil
}
