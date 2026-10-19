package helper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

func TestAServerIsKeyedTheSameWhateverWayItIsWritten(t *testing.T) {
	for written, want := range map[string]string{
		"https://Gusto.Example.com/":    "https://gusto.example.com",
		" https://gusto.example.com ":   "https://gusto.example.com",
		"http://127.0.0.1:8040":         "http://127.0.0.1:8040",
		"https://gusto.example.com/org": "https://gusto.example.com/org",
	} {
		if got := Server(written); got != want {
			t.Errorf("%q is keyed as %q, not %q", written, got, want)
		}
	}
}

func TestATokenIsKeptInTheKeychainAndFoundAgainByAnyWritingOfTheServer(t *testing.T) {
	keyring.MockInit()
	vault, err := OpenVault(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Get("https://gusto.example.com"); !errors.Is(err, ErrNotSignedIn) {
		t.Fatalf("a computer that never signed in said %v", err)
	}
	if err := vault.Set("https://gusto.example.com/", "gusto_abc_secret"); err != nil {
		t.Fatal(err)
	}
	if got, err := vault.Get("HTTPS://gusto.example.com"); err != nil || got != "gusto_abc_secret" {
		t.Errorf("%q %v", got, err)
	}
	if err := vault.Delete("https://gusto.example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Get("https://gusto.example.com"); !errors.Is(err, ErrNotSignedIn) {
		t.Errorf("a token that was deleted was found: %v", err)
	}
}

func TestWithNoKeychainTheTokenIsKeptInAFileOnlyItsOwnerReadsAndTheFirstTimeSaysSo(t *testing.T) {
	keyring.MockInitWithError(errors.New("no secret service on this machine"))
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("AppData", home)
	warned := &bytes.Buffer{}
	vault, err := OpenVault(warned)
	if err != nil {
		t.Fatal(err)
	}
	if err := vault.Set("https://gusto.example.com", "gusto_abc_secret"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warned.String(), "file only you can read") {
		t.Errorf("the person was not told where the token went: %q", warned)
	}
	if got, err := vault.Get("https://gusto.example.com"); err != nil || got != "gusto_abc_secret" {
		t.Fatalf("%q %v", got, err)
	}
	if runtime.GOOS != "windows" {
		config, _ := os.UserConfigDir()
		files, _ := os.ReadDir(config + "/gusto")
		for _, file := range files {
			if info, _ := file.Info(); info.Mode().Perm() != 0o600 {
				t.Errorf("%s is readable by more than its owner: %v", file.Name(), info.Mode().Perm())
			}
		}
		if len(files) != 1 {
			t.Errorf("%d files were kept", len(files))
		}
	}
	if err := vault.Delete("https://gusto.example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Get("https://gusto.example.com"); err == nil {
		t.Error("a token that was deleted was found")
	}
}

// device is a server's side of the device flow, answering what a test chose.
func device(t *testing.T, polls []map[string]any, started map[string]any) *httptest.Server {
	t.Helper()
	next := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/device":
			if started == nil {
				http.NotFound(w, r)
				return
			}
			json.NewEncoder(w).Encode(started)
		case "/api/device/token":
			reply := polls[min(next, len(polls)-1)]
			next++
			if reply["error"] != nil {
				w.WriteHeader(http.StatusBadRequest)
			}
			json.NewEncoder(w).Encode(reply)
		default:
			http.NotFound(w, r)
		}
	}))
}

func noWait(context.Context, time.Duration) error { return nil }

func TestALoginShowsTheCodeOnceWaitsForTheApprovalAndGivesTheToken(t *testing.T) {
	server := device(t, []map[string]any{{"error": "authorization_pending"}, {"error": "slow_down"}, {"access_token": "gusto_abc_secret", "person": "ada@acme.example"}},
		map[string]any{"device_code": "d", "user_code": "BCDF-GHJK", "verification_uri": "https://gusto.example.com/device", "expires_in": 600, "interval": 3})
	defer server.Close()
	var shown []string
	token, person, err := Login{Server: server.URL, Label: "Laptop", Wait: noWait, Show: func(address, code string) { shown = append(shown, address, code) }}.Run(context.Background())
	if err != nil || token != "gusto_abc_secret" || person != "ada@acme.example" {
		t.Fatalf("%q %q %v", token, person, err)
	}
	if len(shown) != 2 || shown[0] != "https://gusto.example.com/device?code=BCDF-GHJK" || shown[1] != "BCDF-GHJK" {
		t.Errorf("the person was shown %q", shown)
	}
}

func TestALoginSaysWhyItFailed(t *testing.T) {
	for name, test := range map[string]struct {
		polls   []map[string]any
		started map[string]any
		say     string
	}{
		"a server that takes no tokens": {nil, nil, "GUSTO_API_TOKENS"},
		"a code that expired":           {[]map[string]any{{"error": "expired_token"}}, map[string]any{"device_code": "d", "user_code": "X", "verification_uri": "http://x/device", "expires_in": 600, "interval": 1}, "expired"},
		"a refusal":                     {[]map[string]any{{"error": "server_error"}}, map[string]any{"device_code": "d", "user_code": "X", "verification_uri": "http://x/device", "expires_in": 600, "interval": 1}, "refused"},
	} {
		server := device(t, test.polls, test.started)
		_, _, err := Login{Server: server.URL, Wait: noWait}.Run(context.Background())
		server.Close()
		if err == nil || !strings.Contains(err.Error(), test.say) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestALoginStopsWhenThePersonGivesUp(t *testing.T) {
	server := device(t, []map[string]any{{"error": "authorization_pending"}},
		map[string]any{"device_code": "d", "user_code": "X", "verification_uri": "http://x/device", "expires_in": 600, "interval": 1})
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := (Login{Server: server.URL}).Run(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("a canceled login said %v", err)
	}
}

func TestAClientSaysWhoItIsAndANoLongerAcceptedTokenSaysToSignInAgain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		json.NewEncoder(w).Encode(Who{Person: "ada@acme.example", Role: "runner"})
	}))
	defer server.Close()
	who, err := Client{Server: server.URL, Token: "good"}.Me(context.Background())
	if err != nil || who.Person != "ada@acme.example" {
		t.Fatalf("%+v %v", who, err)
	}
	if err := (Client{Server: server.URL, Token: "good"}).End(context.Background()); err != nil {
		t.Errorf("a token could not be ended: %v", err)
	}
	if _, err := (Client{Server: server.URL, Token: "stale"}).Me(context.Background()); err == nil || !strings.Contains(err.Error(), "gusto login") {
		t.Errorf("a stale token said %v", err)
	}
}

func TestTheStatusCarriesWhetherTheIconIsShown(t *testing.T) {
	daemon := &Daemon{Server: "https://s"}
	if daemon.Status().Tray != "" {
		t.Error("a helper that has not said invents a state for its icon")
	}
	daemon.SetTray("none: no D-Bus session")
	if got := daemon.Status().Tray; got != "none: no D-Bus session" {
		t.Errorf("tray = %q", got)
	}
}
