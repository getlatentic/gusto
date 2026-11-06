package vmwall

import (
	"encoding/json"
	"os"

	"github.com/getlatentic/gusto/internal/atomicfile"
)

// Settings is what gusto needs to use what was installed: it is written last, so a half-finished install is not used.
type Settings struct {
	Version string `json:"version"`
	// Binary is the openshell command, Gateway the name it knows the gateway by, and Image the root file system tasks start from.
	Binary  string `json:"binary"`
	Gateway string `json:"gateway"`
	Image   string `json:"image"`
}

// Installed reads the settings of an install, or says there is none.
func Installed(l Layout) (Settings, bool) {
	text, err := os.ReadFile(l.Settings())
	if err != nil {
		return Settings{}, false
	}
	var settings Settings
	if json.Unmarshal(text, &settings) != nil || settings.Binary == "" || settings.Image == "" {
		return Settings{}, false
	}
	return settings, true
}

func (s Settings) save(path string) error {
	text, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.WriteFile(path, append(text, '\n'), 0o600)
}
