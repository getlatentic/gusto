package main

import (
	"strings"
	"testing"
)

// The most a kept home may take is a setting declared with the others: five GiB until the operator says otherwise, and a value
// that does not read is said by name before the server starts.
func TestTheKeptHomeLimitIsASettingCheckedAtStart(t *testing.T) {
	t.Parallel()
	load := func(value string) (serveSettings, error) {
		cfg, _, err := loadServe(func(name string) string {
			if name == "GUSTO_HOME_LIMIT_MB" {
				return value
			}
			return ""
		})
		return cfg, err
	}
	if cfg, err := load(""); err != nil || cfg.boxes.homeLimitMB != 5120 {
		t.Errorf("unset, the limit is %d MiB (%v), not 5120", cfg.boxes.homeLimitMB, err)
	}
	if cfg, err := load("2048"); err != nil || cfg.boxes.homeLimitMB != 2048 {
		t.Errorf("GUSTO_HOME_LIMIT_MB=2048 read as %d (%v)", cfg.boxes.homeLimitMB, err)
	}
	for _, wrong := range []string{"big", "0", "-5"} {
		if _, err := load(wrong); err == nil || !strings.Contains(err.Error(), "GUSTO_HOME_LIMIT_MB") {
			t.Errorf("GUSTO_HOME_LIMIT_MB=%q is not said: %v", wrong, err)
		}
	}
}
