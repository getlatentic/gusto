package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/cloud"
	"github.com/getlatentic/gusto/internal/entry"
)

// hostedOrg is the environment gusto cloud gives one organization's server, cut to what a server behind it needs, and the key
// gusto cloud signs that organization's entries with.
func hostedOrg(t *testing.T, slug string) (env []string, key []byte, addr string) {
	t.Helper()
	dir := t.TempDir()
	key = []byte(slug + "-entry-key-that-is-long-enough-to-be-one")
	keyFile := filepath.Join(dir, "entry.key")
	if err := os.WriteFile(keyFile, key, 0o600); err != nil {
		t.Fatal(err)
	}
	addr, proxy := "127.0.0.1:"+strconv.Itoa(freePort(0)), "127.0.0.1:"+strconv.Itoa(freePort(0))
	env = []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "GUSTO_PASSWORD=unused", "GUSTO_SANDBOX=process",
		"GUSTO_ADDR=" + addr, "GUSTO_PROXY_ADDR=" + proxy, "GUSTO_DATA=" + filepath.Join(dir, "data"),
		"GUSTO_ORG=" + slug, "GUSTO_ENTRY_KEY=" + keyFile, "GUSTO_CLOUD_URL=https://cloud.test",
	}
	return env, key, addr
}

// healthy says the server at addr answers its health check, waiting up to wait.
func healthy(addr string, wait time.Duration) bool { return answers("http://"+addr+"/healthz", wait) }

// entering is the status of signing in at addr with an entry token gusto cloud made with key for org.
func entering(t *testing.T, addr string, key []byte, org string) int {
	t.Helper()
	token, err := entry.Mint(key, org, "ada@acme.com", "admin", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Get("http://" + addr + "/auth/entry?t=" + token)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	return response.StatusCode
}

// Two organizations' real servers run in one host and each is still the organization it was started as: a sign-in gusto
// cloud made for one is taken by that one's server and refused by the other's.
func TestOrganizationsInOneHostEachTakeOnlyTheirOwnSignIn(t *testing.T) {
	host := &cloud.Host{Serve: serveHosted}
	t.Cleanup(func() { host.StopAll(10 * time.Second) })
	acmeEnv, acmeKey, acme := hostedOrg(t, "acme")
	globexEnv, _, globex := hostedOrg(t, "globex")
	host.Start(cloud.ServerSpec{Slug: "acme", Env: acmeEnv})
	host.Start(cloud.ServerSpec{Slug: "globex", Env: globexEnv})
	if !healthy(acme, 20*time.Second) || !healthy(globex, 20*time.Second) {
		t.Fatalf("the servers did not answer; running: %v", host.Running())
	}
	if got := entering(t, acme, acmeKey, "acme"); got != http.StatusSeeOther {
		t.Errorf("acme's own sign-in at acme: %d", got)
	}
	if got := entering(t, globex, acmeKey, "acme"); got != http.StatusForbidden {
		t.Errorf("acme's sign-in at globex: %d", got)
	}
	host.Stop("acme", 10*time.Second)
	if healthy(acme, 0) || !healthy(globex, 0) {
		t.Errorf("stopping acme stops only acme: acme %v, globex %v", healthy(acme, 0), healthy(globex, 0))
	}
}

func TestAHostRefusesAServerForOnePersonOnTheirOwnComputer(t *testing.T) {
	env, _, _ := hostedOrg(t, "acme")
	if err := serveHosted(t.Context(), append(env, "GUSTO_LOCAL=on")); err == nil {
		t.Fatal("a local server was run in a host")
	}
}

func TestAHostedServerReadsTheSettingsFileItsEnvironmentNames(t *testing.T) {
	file := filepath.Join(t.TempDir(), "server.toml")
	if err := os.WriteFile(file, []byte("org_name = \"Acme\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lookup, err := environmentOf([]string{"GUSTO_SETTINGS_FILE=" + file, "GUSTO_ORG=acme"})
	if err != nil {
		t.Fatal(err)
	}
	if lookup("GUSTO_ORG_NAME") != "Acme" || lookup("GUSTO_ORG") != "acme" {
		t.Errorf("%q %q", lookup("GUSTO_ORG_NAME"), lookup("GUSTO_ORG"))
	}
}
