package main

import (
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestTheFrontDoorIsMadeFromItsSettings(t *testing.T) {
	root := filepath.Join(t.TempDir(), "cloud")
	t.Setenv("HAAS_CLOUD_ROOT", root)
	t.Setenv("HAAS_CLOUD_URL", "https://cloud.example.com")
	t.Setenv("HAAS_CLOUD_DOMAIN", "example.com")
	t.Setenv("HAAS_CLOUD_FREE_MAIL", "allow")
	t.Setenv("HAAS_CLOUD_TRIAL_JOBS", "7")
	front, err := openCloud()
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
	if cookie.Name != "__Host-haas_cloud" || !cookie.Secure {
		t.Errorf("an https address means a secure cookie: %+v", cookie)
	}
	page := httptest.NewRecorder()
	get := httptest.NewRequest("GET", "/org", nil)
	get.AddCookie(cookie)
	front.Handler().ServeHTTP(page, get)
	if !strings.Contains(page.Body.String(), "acme.example.com") || !strings.Contains(page.Body.String(), "7 in all") {
		t.Errorf("the page should show the organisation's address and the trial from the settings:\n%s", page.Body)
	}
}

func TestBadFrontDoorSettingsAreRefusedByName(t *testing.T) {
	for name, set := range map[string]map[string]string{
		"a link address that is not one": {"HAAS_CLOUD_URL": "cloud.example.com"},
		"a mail setting that is not one": {"HAAS_CLOUD_MAIL": "smtp://mail.example.com"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("HAAS_CLOUD_ROOT", filepath.Join(t.TempDir(), "cloud"))
			for key, value := range set {
				t.Setenv(key, value)
			}
			if _, err := openCloud(); err == nil || !strings.Contains(err.Error(), "HAAS_CLOUD_") {
				t.Errorf("%v", err)
			}
		})
	}
}
