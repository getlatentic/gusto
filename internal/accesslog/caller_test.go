package accesslog

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const password = "correct-horse-battery-staple"

func TestTheCallerIsNamedByKind(t *testing.T) {
	cases := map[string]func(*http.Request){
		"bearer": func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+password) },
		"cookie": func(r *http.Request) { r.AddCookie(&http.Cookie{Name: "haas", Value: "1790000000.signature"}) },
		"none":   func(r *http.Request) {},
	}
	for kind, identify := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		identify(r)
		if got := caller(r); got != kind {
			t.Errorf("want %s, got %s", kind, got)
		}
	}
}

func TestBearerWinsOverACookie(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/jobs", nil)
	r.Header.Set("Authorization", "Bearer "+password)
	r.AddCookie(&http.Cookie{Name: "haas", Value: "1790000000.signature"})
	if got := caller(r); got != "bearer" {
		t.Errorf("want bearer, got %s", got)
	}
}

// The log is read by whoever reads journald, so a credential sent with a
// request never reaches it, in whichever place it was sent.
func TestNoCredentialReachesTheLog(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/jobs?password="+password, strings.NewReader(password))
	r.Header.Set("Authorization", "Bearer "+password)
	r.AddCookie(&http.Cookie{Name: "haas", Value: "cookie-" + password})
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) })

	wrapped, destination := kept(handler)
	wrapped.ServeHTTP(httptest.NewRecorder(), r)

	logged := destination.String()
	if logged == "" {
		t.Fatal("nothing was logged")
	}
	if strings.Contains(logged, password) {
		t.Errorf("the password is in the log: %s", logged)
	}
	if !strings.Contains(logged, `"caller":"bearer"`) || !strings.Contains(logged, `"path":"/api/jobs"`) {
		t.Errorf("the line lost what it should say: %s", logged)
	}
}
