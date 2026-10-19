package vmwall

import (
	"encoding/json"
	"os"
)

// Settings is what gusto needs to use what was installed: it is written last, so a half-finished install is not used.
type Settings struct {
	Version string `json:"version"`
	// Binary is the openshell command, Gateway the name it knows the gateway by, and Image the root file system jobs start from.
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

// Adopt has gusto use an install: GUSTO_OPENSHELL, GUSTO_OPENSHELL_GATEWAY and GUSTO_OPENSHELL_IMAGE, each only where the person or an
// operator has not set it. It is the installed settings that decide nothing a person said.
func Adopt(l Layout, getenv func(string) string, setenv func(name, value string)) {
	settings, ok := Installed(l)
	if !ok {
		return
	}
	for name, value := range map[string]string{"GUSTO_OPENSHELL": settings.Binary, "GUSTO_OPENSHELL_GATEWAY": settings.Gateway, "GUSTO_OPENSHELL_IMAGE": settings.Image} {
		if getenv(name) == "" {
			setenv(name, value)
		}
	}
}

func (s Settings) save(path string) error {
	text, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(text, '\n'), 0o600)
}
