package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/cloud"
)

const goodSecret = "0123456789abcdef0123456789abcdef"

func withCellEnv(t *testing.T, tune func(map[string]string)) {
	t.Helper()
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret")
	os.WriteFile(secret, []byte(goodSecret+"\n"), 0o600)
	set := map[string]string{
		"HAAS_CELL_NAME": "cell-a", "HAAS_CELL_PRIVATE": "10.0.1.5", "HAAS_CELL_SECRET_FILE": secret,
		"HAAS_CELLS_DIR": filepath.Join(dir, "cells"), "HAAS_CELL_PORT": "",
	}
	if tune != nil {
		tune(set)
	}
	for name, value := range set {
		t.Setenv(name, value)
	}
}

func TestACellIsMadeFromItsSettingsAndListensOnThePrivateAddressOnly(t *testing.T) {
	withCellEnv(t, nil)
	agent, listen, dir, err := cellFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if agent.Name != "cell-a" || agent.Private != "10.0.1.5" || string(agent.Secret) != goodSecret || listen != "10.0.1.5:8060" || !strings.HasSuffix(dir, "cells") {
		t.Errorf("%+v %q %q", agent, listen, dir)
	}
	withCellEnv(t, func(s map[string]string) { s["HAAS_CELL_PORT"] = "9060" })
	if _, listen, _, _ := cellFromEnv(); listen != "10.0.1.5:9060" {
		t.Errorf("%q", listen)
	}
}

func TestACellWithASettingWrongOrMissingIsRefusedByNameAndNeverStarts(t *testing.T) {
	for name, tune := range map[string]func(map[string]string){
		"a name that is not one":        func(s map[string]string) { s["HAAS_CELL_NAME"] = "Cell A" },
		"a name that is the front door": func(s map[string]string) { s["HAAS_CELL_NAME"] = "orchestrator" },
		"no name":                       func(s map[string]string) { s["HAAS_CELL_NAME"] = "" },
		"an address that is a name":     func(s map[string]string) { s["HAAS_CELL_PRIVATE"] = "cell.internal" },
		"an IPv6 address":               func(s map[string]string) { s["HAAS_CELL_PRIVATE"] = "fd00::1" },
		"no directory":                  func(s map[string]string) { s["HAAS_CELLS_DIR"] = "" },
		"no secret file":                func(s map[string]string) { s["HAAS_CELL_SECRET_FILE"] = "" },
		"a secret file that is not":     func(s map[string]string) { s["HAAS_CELL_SECRET_FILE"] = "/nonexistent/secret" },
	} {
		withCellEnv(t, tune)
		if _, _, _, err := cellFromEnv(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	dir := t.TempDir()
	short := filepath.Join(dir, "short")
	os.WriteFile(short, []byte("too short"), 0o600)
	withCellEnv(t, func(s map[string]string) { s["HAAS_CELL_SECRET_FILE"] = short })
	if _, _, _, err := cellFromEnv(); err == nil || !strings.Contains(err.Error(), "too short") {
		t.Errorf("a secret that anybody could guess was accepted: %v", err)
	}
}

func TestTheFrontDoorHasNoPoolUntilItIsToldWhereCellsRegisterAndIsThisMachineACellOnlyIfSaidSo(t *testing.T) {
	local := &cloud.LocalCell{}
	t.Setenv("HAAS_CLOUD_CELLS_DIR", "")
	if pool, err := cellPool(local); pool != nil || err != nil {
		t.Fatalf("one machine has no pool: %v %v", pool, err)
	}
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret")
	os.WriteFile(secret, []byte(goodSecret), 0o600)
	t.Setenv("HAAS_CLOUD_CELLS_DIR", filepath.Join(dir, "cells"))
	t.Setenv("HAAS_CLOUD_CELL_SECRET_FILE", "")
	if _, err := cellPool(local); err == nil {
		t.Error("cells with no secret were accepted")
	}
	t.Setenv("HAAS_CLOUD_CELL_SECRET_FILE", secret)
	pool, err := cellPool(local)
	if err != nil || pool.Local != nil || pool.MinFreeMiB != 4096 {
		t.Fatalf("%+v %v", pool, err)
	}
	t.Setenv("HAAS_CLOUD_LOCAL_CELL", "on")
	t.Setenv("HAAS_CLOUD_CELL_MIN_FREE_MIB", "6000")
	if pool, _ := cellPool(local); pool.Local != local || pool.MinFreeMiB != 6000 {
		t.Errorf("%+v", pool)
	}
}

func TestTheFrontDoorPlacesOnCellsOnlyWhenTheyAreConfigured(t *testing.T) {
	t.Setenv("HAAS_CLOUD_SERVERS", "")
	t.Setenv("HAAS_CLOUD_ROOT", filepath.Join(t.TempDir(), "cloud"))
	t.Setenv("HAAS_CLOUD_CELLS_DIR", "")
	_, plain, err := openCloud()
	if err != nil || plain == nil || plain.Cells != nil {
		t.Fatalf("one machine: %v %+v", err, plain)
	}
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret")
	os.WriteFile(secret, []byte(goodSecret), 0o600)
	t.Setenv("HAAS_CLOUD_CELLS_DIR", filepath.Join(dir, "cells"))
	t.Setenv("HAAS_CLOUD_CELL_SECRET_FILE", secret)
	_, placed, err := openCloud()
	if err != nil || placed.Cells == nil {
		t.Errorf("cells: %v %+v", err, placed)
	}
}
