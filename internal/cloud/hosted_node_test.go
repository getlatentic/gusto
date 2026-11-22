package cloud

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/cloud/cloudtest"
	"github.com/getlatentic/gusto/internal/signing"
)

// fakeHost is the test binary as the host: each server it is asked for answers its health and says which organization it is,
// in which process, from the environment it was given and not the process's.
func fakeHost(socket string) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()
	host := &Host{Serve: fakeHostedServer}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		os.Exit(1)
	}
	server := &http.Server{Handler: host.Handler()}
	go func() {
		<-ctx.Done()
		host.StopAll(time.Second)
		server.Close()
	}()
	server.Serve(listener)
}

func fakeHostedServer(ctx context.Context, env []string) error {
	values := map[string]string{}
	for _, each := range env {
		name, value, _ := strings.Cut(each, "=")
		values[name] = value
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	mux.HandleFunc("/whoami", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"org": values["GUSTO_ORG"], "pid": os.Getpid(), "jobs": filepath.Join(values["GUSTO_DATA"], "jobs"),
			"mailVisible": os.Getenv("GUSTO_CLOUD_MAIL") != "", "operatorSetting": values["OPERATOR_SETTING"],
		})
	})
	mux.HandleFunc("/keys/{file}", func(w http.ResponseWriter, r *http.Request) {
		key, err := os.ReadFile(filepath.Join(values[signing.KeyringEnv], r.PathValue("file")))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Write(key)
	})
	listener, err := net.Listen("tcp", values["GUSTO_ADDR"])
	if err != nil {
		return err
	}
	server := &http.Server{Handler: mux}
	go func() { <-ctx.Done(); server.Close() }()
	server.Serve(listener)
	return nil
}

// hosted is a controller over a store whose servers all run in one fake host.
func hosted(t *testing.T, store *Store) (*Controller, *HostedNode) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GUSTO_CLOUD_MAIL", "smtp://user:secret@mail.example.com:587?from=x@example.com")
	// A socket's path is short on macOS, and a test's temporary directory is long.
	dir, err := os.MkdirTemp("", "host")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	socket := filepath.Join(dir, "s")
	env := []string{"OPERATOR_SETTING=given", "PATH=" + os.Getenv("PATH")}
	node := &HostedNode{
		Command: []string{exe}, Env: append(env, "CLOUD_FAKE_HOST_SOCKET="+socket),
		Socket: socket, Log: filepath.Join(dir, "host.log"),
	}
	c := &Controller{
		Store: store, Node: node, Command: []string{exe, "serve"}, Env: env,
		CloudURL: "https://cloud.test", PortFrom: cloudtest.PortsFrom(),
		Wait: 20 * time.Second, Concurrent: 2, StopGrace: time.Second,
	}
	t.Cleanup(c.StopAll)
	return c, node
}

func TestSharedOrganizationsRunInOneProcessEachOnItsOwnPortsAndData(t *testing.T) {
	s := open(t, Settings{PerDomain: 100})
	c, node := hosted(t, s)
	orgs := []Org{made(t, s, "a@acme.com", "Acme"), made(t, s, "b@globex.com", "Globex"), made(t, s, "c@initech.com", "Initech")}
	pids, addresses, jobs := map[any]bool{}, map[string]bool{}, map[any]bool{}
	for _, org := range orgs {
		if err := c.Provision(t.Context(), org.Slug); err != nil {
			logged, _ := os.ReadFile(node.Log)
			t.Fatalf("%s: %v; the host said:\n%s", org.Slug, err, logged)
		}
		address, state := c.Route(t.Context(), org.Slug)
		if state != Ready {
			t.Fatalf("%s: %q", org.Slug, state)
		}
		said := whoIs(t, address)
		if said["org"] != org.Slug || said["operatorSetting"] != "given" || said["mailVisible"] != false {
			t.Errorf("%s answered %v", org.Slug, said)
		}
		pids[said["pid"]], addresses[address], jobs[said["jobs"]] = true, true, true
	}
	if len(pids) != 1 || len(addresses) != 3 || len(jobs) != 3 {
		t.Errorf("one process, three addresses and three data directories: %d %d %d", len(pids), len(addresses), len(jobs))
	}
	if running := node.Running(); len(running) != 3 {
		t.Errorf("the host runs %v", running)
	}
}

func TestAHostThatEndsIsStartedAgainWithTheServersItRan(t *testing.T) {
	s := open(t, Settings{})
	c, node := hosted(t, s)
	org := made(t, s, "ada@acme.com", "Acme")
	if err := c.Provision(t.Context(), org.Slug); err != nil {
		t.Fatal(err)
	}
	address, _ := c.Route(t.Context(), org.Slug)
	first := whoIs(t, address)
	node.mu.Lock()
	node.process.Process.Kill()
	node.mu.Unlock()
	waitFor(t, func() bool { return !node.Alive(org.Slug) })
	c.supervise(t.Context())
	again, state := c.Route(t.Context(), org.Slug)
	if state != Ready || again != address {
		t.Fatalf("%q %q", again, state)
	}
	if said := whoIs(t, again); said["pid"] == first["pid"] {
		t.Error("the host that ended answered")
	}
}

func TestStopAllStopsEveryServerInTheHost(t *testing.T) {
	s := open(t, Settings{PerDomain: 100})
	c, node := hosted(t, s)
	for _, org := range []Org{made(t, s, "a@acme.com", "Acme"), made(t, s, "b@globex.com", "Globex")} {
		if err := c.Provision(t.Context(), org.Slug); err != nil {
			t.Fatal(err)
		}
	}
	c.StopAll()
	if running := node.Running(); len(running) != 0 {
		t.Errorf("still running: %v", running)
	}
}

func TestAHostRefusesWhatIsNotAServerToStart(t *testing.T) {
	host := &Host{Serve: func(ctx context.Context, _ []string) error { <-ctx.Done(); return nil }}
	for _, body := range []string{"{", `{"Slug":"../etc"}`} {
		request, _ := http.NewRequest(http.MethodPost, "/servers", strings.NewReader(body))
		recorder := &statusRecorder{header: http.Header{}}
		host.Handler().ServeHTTP(recorder, request)
		if recorder.status != http.StatusBadRequest {
			t.Errorf("%s: %d", body, recorder.status)
		}
	}
	if len(host.Running()) != 0 {
		t.Errorf("started %v", host.Running())
	}
}

func TestStoppingAServerThatDoesNotEndReturnsAfterTheGrace(t *testing.T) {
	never := make(chan struct{})
	t.Cleanup(func() { close(never) })
	host := &Host{Serve: func(context.Context, []string) error { <-never; return nil }}
	host.Start(ServerSpec{Slug: "acme"})
	began := time.Now()
	host.Stop("acme", 100*time.Millisecond)
	if waited := time.Since(began); waited > 2*time.Second {
		t.Errorf("waited %s", waited)
	}
}

type statusRecorder struct {
	header http.Header
	status int
}

func (r *statusRecorder) Header() http.Header         { return r.header }
func (r *statusRecorder) Write(b []byte) (int, error) { return len(b), nil }
func (r *statusRecorder) WriteHeader(status int)      { r.status = status }
