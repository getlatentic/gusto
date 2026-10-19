package boot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (h *host) as(role string) {
	h.t.Helper()
	os.WriteFile(h.conf, []byte(mustRead(h.t, h.conf)+"ROLE="+role+"\n"), 0o644)
}

func (h *host) has(path string) bool {
	_, err := os.Stat(filepath.Join(h.dir, path))
	return err == nil
}

func TestAnOrchestratorHasTheNameAndTheLeaseAndNoImageOrCredentials(t *testing.T) {
	h := newHost(t)
	h.as("orchestrator")
	done, err := h.boot()
	if err != nil {
		t.Fatal(err)
	}
	if count(done, "certbot ") != 1 || count(done, "haas-dns ") != 1 {
		t.Errorf("the front door has a certificate and the name: %v", done)
	}
	if count(done, "credentials ") != 0 || h.has("lib/runner-all-rootfs.tar.gz") {
		t.Errorf("it runs no computer, so it has no image and no model credentials: %v", done)
	}
	if !h.has("lease/lease.json") {
		t.Error("the lease is the front door's")
	}
	secret := filepath.Join(h.dir, "run", "cell-secret")
	info, err := os.Stat(secret)
	if err != nil || info.Mode().Perm() != 0o600 || mustRead(t, secret) != "value-of-/haas/poc/cell-secret\n" {
		t.Errorf("the secret cells are told with is kept for its owner alone: %v %v", info, err)
	}
	if h.has("run/cell.env") {
		t.Error("an orchestrator is not a cell")
	}
}

func TestACellHasTheImageTheCredentialsAndAnIdentityAndNeverTheNameOrTheTokens(t *testing.T) {
	h := newHost(t)
	h.put("image/key1.tar.gz", "the image")
	h.as("cell")
	done, err := h.boot()
	if err != nil {
		t.Fatal(err)
	}
	if count(done, "certbot ") != 0 || count(done, "haas-dns ") != 0 {
		t.Errorf("a cell does not take the name: %v", done)
	}
	if got, _ := os.ReadFile(filepath.Join(h.dir, "lib", "runner-all-rootfs.tar.gz")); string(got) != "the image" {
		t.Errorf("the image: %q", got)
	}
	if count(done, "credentials ") != 1 {
		t.Errorf("computers on a cell reach the models: %v", done)
	}
	identity := mustRead(t, filepath.Join(h.dir, "run", "cell.env"))
	if want := "GUSTO_CELL_NAME=cell-0abc123def4567890\nGUSTO_CELL_PRIVATE=10.0.7.21\n"; identity != want {
		t.Errorf("who it is: %q", identity)
	}
	if h.has("run/secrets/cf-dns-token") || h.has("run/secrets/mail.env") || h.has("lease/lease.json") {
		t.Error("a cell holds neither the Cloudflare tokens nor a lease")
	}
	if name := strings.Split(strings.Split(identity, "\n")[0], "=")[1]; len(name) > 31 {
		t.Errorf("%q is too long to be a cell's name", name)
	}
}

func TestACellWithNoSecretDoesNotStart(t *testing.T) {
	h := newHost(t)
	h.put("image/key1.tar.gz", "x")
	h.as("cell")
	if _, err := h.boot("FAKE_MISSING=/haas/poc/cell-secret"); err == nil {
		t.Fatal("a cell that cannot be told what to do by anyone must not start")
	}
	if h.has("run/cell.env") {
		t.Error("the host said who it was before it had a secret")
	}
}

func TestAnUnknownRoleIsRefused(t *testing.T) {
	h := newHost(t)
	h.as("both")
	if _, err := h.boot(); err == nil {
		t.Fatal("a role that is not one is refused")
	}
}

func TestTheBotChecksSecretIsKeptForTheFrontDoorToReadAsItsEnvironmentAndForNoOneElse(t *testing.T) {
	for _, role := range []string{"all", "orchestrator"} {
		h := newHost(t)
		h.put("image/key1.tar.gz", "x")
		h.as(role)
		if _, err := h.boot(); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(h.dir, "run", "secrets", "turnstile.env")
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o600 || mustRead(t, path) != "GUSTO_CLOUD_TURNSTILE_SECRET=value-of-/haas/poc/turnstile-secret\n" {
			t.Errorf("%s: %v %v", role, info, err)
		}
	}
	cell := newHost(t)
	cell.put("image/key1.tar.gz", "x")
	cell.as("cell")
	if _, err := cell.boot(); err != nil {
		t.Fatal(err)
	}
	if cell.has("run/secrets/turnstile.env") {
		t.Error("a cell, which runs other people's code, holds the bot check's secret")
	}
}

func TestAHostWhoseBotCheckSecretIsNotStoredStartsWithoutOneAndLeavesNoStaleSecretBehind(t *testing.T) {
	h := newHost(t)
	h.put("image/key1.tar.gz", "x")
	h.as("orchestrator")
	if _, err := h.boot(); err != nil {
		t.Fatal(err)
	}
	if !h.has("run/secrets/turnstile.env") {
		t.Fatal("the secret was not kept")
	}
	if _, err := h.boot("FAKE_MISSING=/haas/poc/turnstile-secret"); err != nil {
		t.Fatalf("a host with no bot check is still a host: %v", err)
	}
	if h.has("run/secrets/turnstile.env") {
		t.Error("a secret that is no longer stored is still on the host")
	}
}

func TestTheLeadHooksAddressAndSecretAreKeptForTheFrontDoorAloneAndBothAreNeeded(t *testing.T) {
	for _, role := range []string{"all", "orchestrator"} {
		h := newHost(t)
		h.put("image/key1.tar.gz", "x")
		h.as(role)
		if _, err := h.boot(); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(h.dir, "run", "secrets", "leadhook.env")
		info, err := os.Stat(path)
		want := "GUSTO_CLOUD_LEAD_HOOK_URL=value-of-/haas/poc/lead-hook-url\nGUSTO_CLOUD_LEAD_HOOK_SECRET=value-of-/haas/poc/lead-hook-secret\n"
		if err != nil || info.Mode().Perm() != 0o600 || mustRead(t, path) != want {
			t.Errorf("%s: %v %v", role, info, err)
		}
		if _, err := h.boot("FAKE_MISSING=/haas/poc/lead-hook-secret"); err != nil {
			t.Fatalf("a host with no lead hook is still a host: %v", err)
		}
		if h.has("run/secrets/leadhook.env") {
			t.Errorf("%s: half a hook, or a hook no longer stored, is still on the host", role)
		}
	}
	cell := newHost(t)
	cell.put("image/key1.tar.gz", "x")
	cell.as("cell")
	if _, err := cell.boot(); err != nil {
		t.Fatal(err)
	}
	if cell.has("run/secrets/leadhook.env") {
		t.Error("a cell, which runs other people's code, holds the lead hook")
	}
}
