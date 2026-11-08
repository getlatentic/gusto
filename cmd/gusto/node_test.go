package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/cloud"
	"github.com/getlatentic/gusto/internal/settings"
)

const goodSecret = "0123456789abcdef0123456789abcdef"

// secretFile is a file with a secret long enough to be one.
func secretFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, []byte(goodSecret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// nodeFrom is a node's settings: a good one, changed as tune says.
func nodeFrom(t *testing.T, tune func(map[string]string)) (nodeSettings, error) {
	t.Helper()
	values := map[string]string{
		"GUSTO_NODE_NAME": "cell-a", "GUSTO_NODE_PRIVATE": "10.0.1.5", "GUSTO_NODE_SECRET_FILE": secretFile(t),
		"GUSTO_NODES_DIR": filepath.Join(t.TempDir(), "cells"),
	}
	if tune != nil {
		tune(values)
	}
	var cfg nodeSettings
	set := settings.New()
	cfg.declare(set)
	return cfg, set.Load(func(name string) string { return values[name] })
}

func TestANodeIsMadeFromItsSettingsAndListensOnThePrivateAddressOnly(t *testing.T) {
	t.Parallel()
	cfg, err := nodeFrom(t, nil)
	if err != nil {
		t.Fatal(err)
	}
	agent, listen, err := nodeOf(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Name != "cell-a" || agent.Private != "10.0.1.5" || string(agent.Secret) != goodSecret || listen != "10.0.1.5:8060" {
		t.Errorf("%+v %q", agent, listen)
	}
	cfg, _ = nodeFrom(t, func(s map[string]string) { s["GUSTO_NODE_PORT"] = "9060" })
	if _, listen, _ := nodeOf(cfg); listen != "10.0.1.5:9060" {
		t.Errorf("%q", listen)
	}
}

func TestANodeWithASettingWrongOrMissingIsRefusedByNameAndNeverStarts(t *testing.T) {
	t.Parallel()
	for name, tune := range map[string]func(map[string]string){
		"a name that is not one":      func(s map[string]string) { s["GUSTO_NODE_NAME"] = "Cell A" },
		"a name that is gusto cloud":  func(s map[string]string) { s["GUSTO_NODE_NAME"] = "orchestrator" },
		"no name":                     func(s map[string]string) { s["GUSTO_NODE_NAME"] = "" },
		"an address that is a name":   func(s map[string]string) { s["GUSTO_NODE_PRIVATE"] = "cell.internal" },
		"an IPv6 address":             func(s map[string]string) { s["GUSTO_NODE_PRIVATE"] = "fd00::1" },
		"no directory":                func(s map[string]string) { s["GUSTO_NODES_DIR"] = "" },
		"no secret file":              func(s map[string]string) { s["GUSTO_NODE_SECRET_FILE"] = "" },
		"a secret file that is not":   func(s map[string]string) { s["GUSTO_NODE_SECRET_FILE"] = "/nonexistent/secret" },
		"a port that is not a number": func(s map[string]string) { s["GUSTO_NODE_PORT"] = "eighty" },
	} {
		cfg, err := nodeFrom(t, tune)
		if err == nil {
			_, _, err = nodeOf(cfg)
		}
		if err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	short := filepath.Join(t.TempDir(), "short")
	os.WriteFile(short, []byte("too short"), 0o600)
	cfg, _ := nodeFrom(t, func(s map[string]string) { s["GUSTO_NODE_SECRET_FILE"] = short })
	if _, _, err := nodeOf(cfg); err == nil || !strings.Contains(err.Error(), "too short") {
		t.Errorf("a secret that anybody could guess was accepted: %v", err)
	}
}

func TestGustoCloudHasNoPoolUntilItIsToldWhereNodesRegisterAndIsThisMachineANodeOnlyIfSaidSo(t *testing.T) {
	t.Parallel()
	local := &cloud.LocalNode{}
	if pool, err := nodePool(cloudFrom(t, nil).nodes, local); pool != nil || err != nil {
		t.Fatalf("one machine has no pool: %v %v", pool, err)
	}
	nodes := filepath.Join(t.TempDir(), "cells")
	if _, err := nodePool(cloudFrom(t, map[string]string{"GUSTO_CLOUD_NODES_DIR": nodes}).nodes, local); err == nil {
		t.Error("cells with no secret were accepted")
	}
	pooled := map[string]string{"GUSTO_CLOUD_NODES_DIR": nodes, "GUSTO_CLOUD_NODE_SECRET_FILE": secretFile(t)}
	pool, err := nodePool(cloudFrom(t, pooled).nodes, local)
	if err != nil || pool.Local != nil || pool.MinFreeMiB != 4096 {
		t.Fatalf("%+v %v", pool, err)
	}
	if pool.GoneAfter != 30*time.Minute {
		t.Errorf("a cell is gone after half an hour unless it is said otherwise: %s", pool.GoneAfter)
	}
	pooled["GUSTO_CLOUD_LOCAL_NODE"], pooled["GUSTO_CLOUD_NODE_MIN_FREE_MIB"], pooled["GUSTO_CLOUD_NODE_GONE_MINUTES"] = "on", "6000", "5"
	if pool, _ := nodePool(cloudFrom(t, pooled).nodes, local); pool.Local != local || pool.MinFreeMiB != 6000 || pool.GoneAfter != 5*time.Minute {
		t.Errorf("%+v", pool)
	}
}

func TestGustoCloudPlacesOnNodesOnlyWhenTheyAreConfigured(t *testing.T) {
	t.Parallel()
	_, plain, err := openCloud(t.Context(), cloudFrom(t, map[string]string{"GUSTO_CLOUD_SERVERS": ""}))
	if err != nil || plain == nil || plain.Nodes != nil {
		t.Fatalf("one machine: %v %+v", err, plain)
	}
	pooled := map[string]string{
		"GUSTO_CLOUD_SERVERS": "", "GUSTO_CLOUD_NODES_DIR": filepath.Join(t.TempDir(), "cells"), "GUSTO_CLOUD_NODE_SECRET_FILE": secretFile(t),
	}
	_, placed, err := openCloud(t.Context(), cloudFrom(t, pooled))
	if err != nil || placed.Nodes == nil {
		t.Errorf("cells: %v %+v", err, placed)
	}
}

func TestGustoCloudStartsHostsForNodesOnlyWhenItIsToldHowAndToldEnough(t *testing.T) {
	t.Parallel()
	plain := &cloud.Controller{}
	if scaler, err := nodeScaler(cloudFrom(t, nil).scaler, plain); scaler != nil || err != nil {
		t.Fatalf("off unless asked for: %v %v", scaler, err)
	}
	launch := map[string]string{"GUSTO_CLOUD_NODE_LAUNCH": "ec2"}
	if _, err := nodeScaler(cloudFrom(t, launch).scaler, plain); err == nil {
		t.Error("hosts for cells with nowhere for cells to register")
	}
	pooled := cloudFrom(t, map[string]string{"GUSTO_CLOUD_NODES_DIR": filepath.Join(t.TempDir(), "cells"), "GUSTO_CLOUD_NODE_SECRET_FILE": secretFile(t)})
	pool, err := nodePool(pooled.nodes, &cloud.LocalNode{})
	if err != nil {
		t.Fatal(err)
	}
	controller := &cloud.Controller{Nodes: pool}
	if _, err := nodeScaler(cloudFrom(t, launch).scaler, controller); err == nil {
		t.Error("no template or script")
	}
	launch["GUSTO_CLOUD_NODE_TEMPLATE"] = "gusto-poc-demo-host-cell"
	launch["GUSTO_CLOUD_NODE_USERDATA"] = "/opt/gusto-bundle/user-data.sh"
	launch["GUSTO_CLOUD_NODE_SETTINGS"] = "ROLE=cell,RUN=live"
	scaler, err := nodeScaler(cloudFrom(t, launch).scaler, controller)
	if err != nil || scaler == nil || scaler.Max != 3 || scaler.Idle != 10*time.Minute || scaler.Boot != 10*time.Minute {
		t.Fatalf("%+v %v", scaler, err)
	}
	launch["GUSTO_CLOUD_NODE_MAX"], launch["GUSTO_CLOUD_NODE_IDLE_MINUTES"] = "1", "2"
	if scaler, _ := nodeScaler(cloudFrom(t, launch).scaler, controller); scaler.Max != 1 || scaler.Idle != 2*time.Minute {
		t.Errorf("%+v", scaler)
	}
	launch["GUSTO_CLOUD_NODE_SETTINGS"] = "ROLE"
	if _, err := nodeScaler(cloudFrom(t, launch).scaler, controller); err == nil {
		t.Error("a setting with no value")
	}
}
