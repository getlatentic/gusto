package cloud

import (
	"os"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/getlatentic/gusto/internal/settings"
)

// An organization's server is started with its settings in a file only gusto cloud and the server read, and its secrets and
// what is not a setting in its environment, which names the file. A setting the operator gave is in the file too.
func TestAnOrganizationsServerHasItsSettingsInAFileAndItsSecretsInItsEnvironment(t *testing.T) {
	store := open(t, Settings{})
	secret := map[string]bool{"GUSTO_PASSWORD": true, databaseURLEnv: true}
	c := &Controller{Store: store, Node: &LocalNode{}, Env: []string{"GUSTO_MODELS=m", "PATH=/bin"}, CloudURL: "https://cloud.test",
		InFile: func(name string) bool { return strings.HasPrefix(name, "GUSTO_") && !secret[name] }}
	env, err := c.environment(t.Context(), "acme", Runtime{Port: 1, ProxyPort: 2}, c.Node)
	if err != nil {
		t.Fatal(err)
	}
	path, _ := settingOf(env, settings.FileEnv)
	for _, each := range env {
		name, _, _ := strings.Cut(each, "=")
		if strings.HasPrefix(name, "GUSTO_") && name != settings.FileEnv && !secret[name] {
			t.Errorf("%s is in the environment", name)
		}
	}
	if password, _ := settingOf(env, "GUSTO_PASSWORD"); password == "" {
		t.Error("the password is not in the environment")
	}
	if home, _ := settingOf(env, "HOME"); home == "" {
		t.Error("HOME is not in the environment")
	}
	var filed map[string]string
	if _, err := toml.DecodeFile(path, &filed); err != nil {
		t.Fatal(err)
	}
	if filed["org"] != "acme" || filed["models"] != "m" || filed["data"] != store.DataDir("acme") || filed["password"] != "" {
		t.Errorf("the file holds %v", filed)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the file is %v %v", info, err)
	}
}

// A node reads what a server was started with from the server's settings file and its environment, the environment winning,
// so it knows the server's sandbox when it cleans up after one.
func TestANodeReadsAServersSettingsFromItsFileAndItsEnvironment(t *testing.T) {
	path := t.TempDir() + "/server.toml"
	if err := settings.WriteFile(path, map[string]string{"GUSTO_SANDBOX": "container", "GUSTO_CONTAINER": "podman"}); err != nil {
		t.Fatal(err)
	}
	started := startedWith([]string{settings.FileEnv + "=" + path, "GUSTO_CONTAINER=docker"})
	if started["GUSTO_SANDBOX"] != "container" || started["GUSTO_CONTAINER"] != "docker" {
		t.Errorf("%v", started)
	}
}
