package main

import (
	"errors"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/cloud"
	"github.com/getlatentic/gusto/internal/jobs"
	"github.com/getlatentic/gusto/internal/quota"
)

func TestTheFrontDoorIsMadeFromItsSettings(t *testing.T) {
	// A test binary is not a gusto that can serve an organization.
	t.Setenv("GUSTO_CLOUD_SERVERS", "off")
	root := filepath.Join(t.TempDir(), "cloud")
	t.Setenv("GUSTO_CLOUD_ROOT", root)
	t.Setenv("GUSTO_CLOUD_URL", "https://cloud.example.com")
	t.Setenv("GUSTO_CLOUD_DOMAIN", "example.com")
	t.Setenv("GUSTO_CLOUD_FREE_MAIL", "allow")
	t.Setenv("GUSTO_CLOUD_TRIAL_JOBS", "7")
	front, _, err := openCloud()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(front.Handler())
	defer server.Close()

	form := url.Values{"email": {"ada@gmail.com"}, "org": {"Acme"}}
	request := httptest.NewRequest("POST", "/signup", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", "http://example.com")
	request.Host = "example.com"
	recorder := httptest.NewRecorder()
	front.Handler().ServeHTTP(recorder, request)
	if recorder.Code != 303 {
		t.Fatalf("free mail is allowed when the operator says so: %d %s", recorder.Code, recorder.Body)
	}
	cookie := recorder.Result().Cookies()[0]
	if cookie.Name != "__Host-gusto_cloud" || !cookie.Secure {
		t.Errorf("an https address means a secure cookie: %+v", cookie)
	}
	page := httptest.NewRecorder()
	get := httptest.NewRequest("GET", "/org", nil)
	get.AddCookie(cookie)
	front.Handler().ServeHTTP(page, get)
	if !strings.Contains(page.Body.String(), "acme.example.com") || !strings.Contains(page.Body.String(), "7 in all") {
		t.Errorf("the page should show the organization's address and the trial from the settings:\n%s", page.Body)
	}
}

func TestBadFrontDoorSettingsAreRefusedByName(t *testing.T) {
	t.Setenv("GUSTO_CLOUD_SERVERS", "off")
	for name, set := range map[string]map[string]string{
		"a link address that is not one": {"GUSTO_CLOUD_URL": "cloud.example.com"},
		"a mail setting that is not one": {"GUSTO_CLOUD_MAIL": "smtp://mail.example.com"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("GUSTO_CLOUD_ROOT", filepath.Join(t.TempDir(), "cloud"))
			for key, value := range set {
				t.Setenv(key, value)
			}
			if _, _, err := openCloud(); err == nil || !strings.Contains(err.Error(), "GUSTO_CLOUD_") {
				t.Errorf("%v", err)
			}
		})
	}
}

// The criterion the story ends on: a sign-up cannot make more than a small amount
// of work until it is verified. The front door writes the limit and the
// organization's own server enforces it, and confirming the address lifts it
// without the server being restarted.
func TestASignUpRunsASmallTrialUntilItsAddressIsVerified(t *testing.T) {
	t.Setenv("GUSTO_CLOUD_SERVERS", "off")
	t.Setenv("GUSTO_CLOUD_ROOT", filepath.Join(t.TempDir(), "cloud"))
	front, _, err := openCloud()
	if err != nil {
		t.Fatal(err)
	}
	_, org, err := front.Store.SignUp(cloud.Signup{Email: "ada@acme.com", OrgName: "Acme"})
	if err != nil {
		t.Fatal(err)
	}
	// The organization's server, as it would be started: a jobs directory of its
	// own under the organization's data directory, and the quota the front door wrote.
	index, err := jobs.Open(filepath.Join(front.Store.DataDir(org.Slug), "jobs"))
	if err != nil {
		t.Fatal(err)
	}
	index.UseQuota(quota.Open(front.Store.QuotaFile(org.Slug)))

	start := func(brief string) error {
		id, err := index.Reserve(brief, time.Now())
		if err != nil {
			return err
		}
		record := jobs.Record{ID: id, State: jobs.Queued, Brief: brief, Harness: "fixture", Asked: "ada@acme.com", Queued: jobs.Now()}
		if err := os.MkdirAll(record.Dir(index.Root()).Path(), 0o755); err != nil {
			return err
		}
		return index.Add(record)
	}
	for _, brief := range []string{"one", "two", "three"} {
		if err := start(brief); err != nil {
			t.Fatalf("%s is within the trial: %v", brief, err)
		}
	}
	err = start("four")
	if !errors.Is(err, quota.ErrExceeded) || !strings.Contains(err.Error(), "3 tasks in all") || !strings.Contains(err.Error(), "Confirm the admin's email address") {
		t.Fatalf("the fourth task is over the trial, and the refusal says how to lift it: %v", err)
	}

	if _, err := front.Store.Verify("ada@acme.com"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	if err := start("four"); err != nil {
		t.Errorf("verifying lifts the limit for the running server: %v", err)
	}
}

func TestTheOperatorsSettingsForEveryOrganizationAreReadFromAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "org.env")
	os.WriteFile(path, []byte("# what every organization's server has\nGUSTO_CREDENTIALS=/etc/gusto/credentials\n\n  GUSTO_SANDBOX=openshell  \n"), 0o600)
	env, err := operatorEnv(path)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(env, "\n")
	for _, want := range []string{"PATH=", "GUSTO_CREDENTIALS=/etc/gusto/credentials", "GUSTO_SANDBOX=openshell"} {
		if !strings.Contains(joined, want) {
			t.Errorf("%q is not in %v", want, env)
		}
	}
	if strings.Contains(joined, "#") {
		t.Errorf("a comment is not a setting: %v", env)
	}
	bad := filepath.Join(t.TempDir(), "bad.env")
	os.WriteFile(bad, []byte("GUSTO_SANDBOX=openshell\nthis is not a setting\n"), 0o600)
	if _, err := operatorEnv(bad); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("a line that is not a setting is named by its number: %v", err)
	}
	if _, err := operatorEnv(filepath.Join(t.TempDir(), "missing.env")); err == nil {
		t.Error("a file that is named and missing is an error")
	}
	if env, err := operatorEnv(""); err != nil || len(env) != 1 {
		t.Errorf("no file is only the path: %v %v", env, err)
	}
}

func TestTheOperatorSeesEveryComputerWithItsOrganizationAndPerson(t *testing.T) {
	store, err := cloud.Open(filepath.Join(t.TempDir(), "cloud"), cloud.Settings{PerDomain: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, sign := range []cloud.Signup{{Email: "ada@acme.com", OrgName: "Acme"}, {Email: "eve@globex.com", OrgName: "Globex"}, {Email: "ned@initech.com", OrgName: "Initech"}} {
		if _, org, err := store.SignUp(sign); err != nil {
			t.Fatal(err)
		} else if org.Slug != "initech-com" {
			registry, _ := store.Computers(org.Slug)
			registry.Ensure(sign.Email, "cell-1")
		}
	}
	var out strings.Builder
	if err := listComputers(&out, store); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range []string{"ORGANIZATION", "acme-com", "ada@acme.com", "globex-com", "eve@globex.com", "cell-1", "creating", "never", "2 workspaces in 3 organizations"} {
		if !strings.Contains(text, want) {
			t.Errorf("%q is not in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "ned@initech.com") {
		t.Errorf("an organization with no computers lists none:\n%s", text)
	}
}
