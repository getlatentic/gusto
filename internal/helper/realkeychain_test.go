package helper

import (
	"os"
	"testing"
)

// TestTheRealKeychainKeepsAToken uses the operating system's own keychain, which a test run on a developer's machine should not
// touch without being asked, so it runs when GUSTO_REAL_KEYCHAIN=1: on a Windows runner, and by hand.
func TestTheRealKeychainKeepsAToken(t *testing.T) {
	if os.Getenv("GUSTO_REAL_KEYCHAIN") != "1" {
		t.Skip("set GUSTO_REAL_KEYCHAIN=1 to use this computer's keychain")
	}
	vault, err := OpenVault(nil)
	if err != nil {
		t.Fatal(err)
	}
	const server = "https://keychain-test.invalid"
	if err := vault.Set(server, "gusto_x_realkeychain"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { vault.Delete(server) })
	if got, err := vault.Get(server); err != nil || got != "gusto_x_realkeychain" {
		t.Fatalf("read back %q, %v", got, err)
	}
	if err := vault.Delete(server); err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Get(server); err == nil {
		t.Error("a deleted token was still there")
	}
}
