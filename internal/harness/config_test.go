package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A harness's configuration names /root, which a container has and a person's computer does not: placed in a home, it names that home.
func TestAHarnessRunOutsideTheImageIsPointedAtItsOwnHome(t *testing.T) {
	home := t.TempDir()
	if err := PlaceConfig(home); err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(filepath.Join(home, ".config", "hermes", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(text), "/root") || !strings.Contains(string(text), filepath.Join(home, ".agents", "skills")) {
		t.Errorf("the configuration does not name the home:\n%s", text)
	}
}
