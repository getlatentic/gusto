package main

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/gusto/cmd/internal/cmdline/clitest"
	"github.com/getlatentic/gusto/internal/cloud"
)

// oldCloudRoot is a root with the files the earlier release kept, copied from the fixture the cloud package tests.
func oldCloudRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "cloud")
	from := filepath.Join("..", "..", "internal", "cloud", "testdata", "oldlayout", "cloud")
	if err := os.CopyFS(root, os.DirFS(from)); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GUSTO_CLOUD_ROOT", root)
	t.Setenv("GUSTO_DATABASE_URL", "")
	return root
}

func TestCloudMigrateReportsWithoutChangingAnythingAndImportsOnRequest(t *testing.T) {
	root := oldCloudRoot(t)
	out, _, code := clitest.Capture(t, func() int { return run([]string{"cloud", "migrate"}) })
	if code != 0 || !strings.Contains(out, "not made yet") || !strings.Contains(out, "import             not run yet") ||
		!strings.Contains(out, "people         6") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(root, "cloud.db")); err == nil {
		t.Error("asking made the database")
	}

	out, _, code = clitest.Capture(t, func() int { return run([]string{"cloud", "migrate", "-import"}) })
	if code != 0 || !strings.Contains(out, "imported into the database") || !strings.Contains(out, "people         6") || !strings.Contains(out, "schema version     2 (this release reads 2)") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	out, _, code = clitest.Capture(t, func() int { return run([]string{"cloud", "migrate", "-import"}) })
	if code != 0 || !strings.Contains(out, "has had its import already") {
		t.Errorf("a second import loads nothing: exit %d:\n%s", code, out)
	}
	if _, _, code := clitest.Capture(t, func() int { return run([]string{"cloud", "migrate", "extra"}) }); code != 2 {
		t.Errorf("a stray argument is a usage error: %d", code)
	}
}

func TestTheFrontDoorKeepsItsAccountsWhereTheDatabaseAddressSaysAndStartsOnTheOldFiles(t *testing.T) {
	t.Setenv("GUSTO_CLOUD_SERVERS", "off")
	root := oldCloudRoot(t)
	elsewhere := filepath.Join(t.TempDir(), "elsewhere", "accounts.db")
	t.Setenv("GUSTO_DATABASE_URL", "sqlite://"+filepath.ToSlash(elsewhere))
	front, _, err := openCloud()
	if err != nil {
		t.Fatal(err)
	}
	defer front.Store.Close()
	if _, err := os.Stat(elsewhere); err != nil {
		t.Errorf("the accounts are not where the address says: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "cloud.db")); err == nil {
		t.Error("the default database was made as well")
	}
	if found, _ := front.Store.HasAccount("ada@acme.com"); !found {
		t.Error("the front door started on the old files without taking them in")
	}
}

func TestEachOrganizationsIndexDirectoryIsTheOneTheOperatorNamesOrBelowTheRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cloud")
	t.Setenv("GUSTO_CLOUD_ROOT", root)
	t.Setenv("GUSTO_DATABASE_URL", "")
	t.Setenv("GUSTO_CLOUD_INDEX_DIR", "")
	store, err := cloud.Open(root, cloud.Settings{})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	public, _ := url.Parse("https://cloud.example.com")
	controller, err := organizationServers(store, public, cloud.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	if controller.IndexDir != filepath.Join(root, "index") {
		t.Errorf("the default: %q", controller.IndexDir)
	}
	t.Setenv("GUSTO_CLOUD_INDEX_DIR", "/var/lib/gusto/index")
	if controller, _ = organizationServers(store, public, cloud.DefaultLimits()); controller.IndexDir != "/var/lib/gusto/index" {
		t.Errorf("the operator's: %q", controller.IndexDir)
	}
}
