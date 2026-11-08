package main

import (
	"os"
	"testing"
)

// fromEnv is serve's settings as the test's environment sets them.
func fromEnv(t *testing.T) serveSettings {
	t.Helper()
	cfg, _, err := loadServe(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
