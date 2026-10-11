package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestTheLocalSetupIsMadeOnceAndOnlyItsPersonCanReadIt(t *testing.T) {
	home := filepath.Join(t.TempDir(), "haas")
	first, err := ensureLocal(home)
	if err != nil || first.Port == 0 || len(first.Secret) != 64 {
		t.Fatalf("%+v, %v", first, err)
	}
	again, err := ensureLocal(home)
	if err != nil || again != first {
		t.Errorf("the setup changed when it was read again: %+v, %v", again, err)
	}
	for path, want := range map[string]os.FileMode{home: 0o700, filepath.Join(home, "local.json"): 0o600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != want {
			t.Errorf("%s is %v, want %v (%v)", path, info.Mode().Perm(), want, err)
		}
	}
	os.WriteFile(filepath.Join(home, "local.json"), []byte("{not json"), 0o600)
	if _, err := ensureLocal(home); err == nil {
		t.Error("a damaged setup was used")
	}
}

func TestAServerForOnePersonListensOnTheLoopbackOnly(t *testing.T) {
	for address, want := range map[string]bool{"127.0.0.1:7140": true, "[::1]:7140": true, "0.0.0.0:7140": false, "192.168.1.4:7140": false, ":7140": false, "localhost:7140": false} {
		if got := onLoopback(address); got != want {
			t.Errorf("%s: %v, want %v", address, got, want)
		}
	}
	t.Setenv("HAAS_LOCAL", "1")
	t.Setenv("HAAS_PASSWORD", "x")
	t.Setenv("HAAS_ADDR", "0.0.0.0:7140")
	if code := serve(nil); code != 2 {
		t.Errorf("a server for one person on every address started: %d", code)
	}
}

// A browser that keeps running is left to run, and one that cannot open the link says so, so the link is printed instead.
func TestOpeningALinkDoesNotWaitForTheBrowser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the opener is rundll32")
	}
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	browseWait = 200 * time.Millisecond
	t.Cleanup(func() { browseWait = 3 * time.Second })
	for script, fails := range map[string]bool{"sleep 30": false, "exit 3": true} {
		bin := t.TempDir()
		if err := os.WriteFile(filepath.Join(bin, opener), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		began := time.Now()
		err := browse("http://127.0.0.1:1/local/enter")
		if took := time.Since(began); took > 5*time.Second {
			t.Errorf("%s: opening took %v", script, took)
		}
		if (err != nil) != fails {
			t.Errorf("%s: %v", script, err)
		}
	}
}
