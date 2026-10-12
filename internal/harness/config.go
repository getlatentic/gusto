package harness

import (
	"bytes"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

// config is each harness's own configuration, in a folder named after it: what the runner image copies to /root/.config.
//
//go:embed config
var config embed.FS

// PlaceConfig writes each harness's own configuration to home's .config, where an adapter written for a container's /root
// finds it, for a harness that runs outside the image.
func PlaceConfig(home string) error {
	files, err := fs.Sub(config, "config")
	if err != nil {
		return err
	}
	return fs.WalkDir(files, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		to := filepath.Join(home, ".config", filepath.FromSlash(path))
		if entry.IsDir() {
			return os.MkdirAll(to, 0o700)
		}
		text, err := fs.ReadFile(files, path)
		if err != nil {
			return err
		}
		return os.WriteFile(to, bytes.ReplaceAll(text, []byte("/root"), []byte(home)), 0o600)
	})
}
