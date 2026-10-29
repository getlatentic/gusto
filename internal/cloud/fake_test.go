package cloud

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/entry"
)

// The test binary is also the organization's server: a controller that is told to
// start `<this binary> serve` with CLOUD_FAKE_ORG=1 in its environment gets a real
// process on a real port, which is what starting, stopping and losing one are
// tested against.
func TestMain(m *testing.M) {
	if os.Getenv("CLOUD_FAKE_ORG") == "1" {
		fakeOrgServer()
		return
	}
	os.Exit(m.Run())
}

func fakeOrgServer() {
	if os.Getenv("FAKE_FAIL") != "" {
		fmt.Fprintln(os.Stderr, "the fake server was told to fail")
		os.Exit(2)
	}
	if delay, err := time.ParseDuration(os.Getenv("FAKE_DELAY")); err == nil {
		time.Sleep(delay)
	}
	org, addr := os.Getenv("GUSTO_ORG"), os.Getenv("GUSTO_ADDR")
	terms := make(chan os.Signal, 1)
	signal.Notify(terms, syscall.SIGTERM)
	go func() {
		<-terms
		if os.Getenv("FAKE_IGNORE_TERM") != "" {
			select {}
		}
		os.WriteFile(filepath.Join(os.Getenv("GUSTO_JOBS"), "..", "stopped"), []byte("asked\n"), 0o600)
		os.Exit(0)
	}()
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	mux.HandleFunc("/whoami", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"org": org, "pid": os.Getpid(), "addr": addr, "host": r.Host, "xff": r.Header.Get("X-Forwarded-For"),
			"proto": r.Header.Get("X-Forwarded-Proto"), "cookie": r.Header.Get("Cookie"), "app": r.Header.Get("X-Gusto-App"),
			"jobs": os.Getenv("GUSTO_JOBS"), "quota": os.Getenv("GUSTO_QUOTA"), "members": os.Getenv("GUSTO_MEMBERS"), "inbox": os.Getenv("GUSTO_INBOX"), "orgSettings": os.Getenv("GUSTO_ORG_SETTINGS"), "home": os.Getenv("HOME"),
			"entryKey": os.Getenv("GUSTO_ENTRY_KEY"), "signingKey": os.Getenv("GUSTO_SIGNING_KEY"),
			"proxyAddr": os.Getenv("GUSTO_PROXY_ADDR"), "front": os.Getenv("GUSTO_FRONT_DOOR"), "orgName": os.Getenv("GUSTO_ORG_NAME"), "concurrency": os.Getenv("GUSTO_JOB_CONCURRENCY"),
			"mailVisible": os.Getenv("GUSTO_CLOUD_MAIL") != "", "operatorSetting": os.Getenv("OPERATOR_SETTING"),
		})
	})
	mux.HandleFunc("/die", func(http.ResponseWriter, *http.Request) { os.Exit(3) })
	// A WebSocket's upgrade, answered with the host and origin it was given, and then a line echoed for each line sent.
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			http.Error(w, "that is not an upgrade", http.StatusBadRequest)
			return
		}
		conn, buffered, err := http.NewResponseController(w).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		fmt.Fprintf(buffered, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nX-Seen-Host: %s\r\nX-Seen-Origin: %s\r\nX-Seen-Cookie: %s\r\n\r\n",
			r.Host, r.Header.Get("Origin"), r.Header.Get("Cookie"))
		buffered.Flush()
		lines := bufio.NewScanner(buffered)
		for lines.Scan() {
			fmt.Fprintf(buffered, "echo %s\n", lines.Text())
			buffered.Flush()
		}
	})
	mux.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		flusher := w.(http.Flusher)
		for i := range 3 {
			fmt.Fprintf(w, "line %d\n", i)
			flusher.Flush()
			time.Sleep(150 * time.Millisecond)
		}
	})
	mux.HandleFunc("/auth/entry", func(w http.ResponseWriter, r *http.Request) {
		key, _ := os.ReadFile(os.Getenv("GUSTO_ENTRY_KEY"))
		claims, err := entry.Verify(key, org, r.URL.Query().Get("t"), time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusForbidden)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "gusto", Value: claims.Email + "|" + claims.Role, Path: "/"})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	http.Serve(listener, mux)
}

// controlled is a controller over a store, whose servers are the fake.
func controlled(t *testing.T, store *Store, extra ...string) *Controller {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// The operator's settings, and what a careless one would not want a server to see is
	// in this process's environment and not in them.
	t.Setenv("GUSTO_CLOUD_MAIL", "smtp://user:secret@mail.example.com:587?from=x@example.com")
	c := &Controller{
		Store: store, Cell: &LocalCell{}, Command: []string{exe, "serve"},
		Env:       append([]string{"CLOUD_FAKE_ORG=1", "OPERATOR_SETTING=given", "PATH=" + os.Getenv("PATH")}, extra...),
		FrontDoor: "https://cloud.test", PortFrom: 38000 + int(time.Now().UnixNano()%5000),
		Wait: 20 * time.Second, Concurrent: 2, StopGrace: time.Second,
	}
	t.Cleanup(c.stopAll)
	return c
}
