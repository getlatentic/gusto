package main

import "testing"

// fromEnv is serve's settings as the test's environment sets them.
func fromEnv(t *testing.T) serveSettings {
	t.Helper()
	cfg, _, err := loadServe()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}
