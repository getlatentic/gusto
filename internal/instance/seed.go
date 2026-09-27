package instance

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Seed writes files into an instance's folder and commits them, so what a
// template brings is where the harness starts from and not a change it made.
func (s *Store) Seed(id string, files map[string]string) error {
	if len(files) == 0 {
		return nil
	}
	workspace := s.Workspace(id)
	for name, text := range files {
		clean := filepath.Clean(name)
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			return fmt.Errorf("%q is not a file in the folder", name)
		}
		path := filepath.Join(workspace, clean)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return err
		}
	}
	if err := run(workspace, "add", "-A"); err != nil {
		return err
	}
	return run(workspace, "-c", "user.name=haas", "-c", "user.email=haas@localhost",
		"commit", "--quiet", "-m", "the template's sample files")
}
