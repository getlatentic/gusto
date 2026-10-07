package cell

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/haas/internal/cloud"
)

// The test binary is also an organisation's server: a cell that is told to start it with CELL_FAKE_ORG=1
// gets a real process on a real port.
func TestMain(m *testing.M) {
	if os.Getenv("CELL_FAKE_ORG") == "1" {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
		mux.HandleFunc("/whoami", func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(map[string]string{"org": os.Getenv("HAAS_ORG"), "cell": os.Getenv("HAAS_CELL"), "addr": os.Getenv("HAAS_ADDR")})
		})
		listener, err := net.Listen("tcp", os.Getenv("HAAS_ADDR"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		http.Serve(listener, mux)
		return
	}
	os.Exit(m.Run())
}

var secret = []byte("0123456789abcdef0123456789abcdef")

// agentHost is a cell: an agent on a listener of its own, its servers, and what it says it has to spare.
type agentHost struct {
	server  *httptest.Server
	servers *cloud.LocalCell
	mu      sync.Mutex
	free    int
}

func newAgent(t *testing.T, name string, free int) *agentHost {
	t.Helper()
	h := &agentHost{servers: &cloud.LocalCell{}, free: free}
	agent := &Agent{Name: name, Private: "127.0.0.1", Secret: secret, Servers: h.servers, Measure: func() Capacity {
		h.mu.Lock()
		defer h.mu.Unlock()
		return Capacity{CPUs: 4, MemoryAvailableMiB: h.free}
	}}
	h.server = httptest.NewServer(agent.Handler())
	t.Cleanup(func() {
		h.stopServers()
		h.server.Close()
	})
	return h
}

// stopServers ends every organisation's server on the host. A test that leaves servers running registers it after it
// makes the folders they write to, because cleanups run last first and a folder cannot be removed under a server.
func (h *agentHost) stopServers() {
	for _, slug := range h.servers.Running() {
		h.servers.Stop(slug, time.Second)
	}
}

func (h *agentHost) setFree(n int) { h.mu.Lock(); h.free = n; h.mu.Unlock() }

func (h *agentHost) registration(name string, at time.Time) Registration {
	return Registration{Name: name, Agent: strings.TrimPrefix(h.server.URL, "http://"), Private: "127.0.0.1", Beat: at.Unix()}
}

func remoteTo(h *agentHost, name string) *Remote {
	return newRemote(h.registration(name, time.Now()), secret)
}

func fakeOrg(t *testing.T, slug string, port int) cloud.ServerSpec {
	t.Helper()
	exe, _ := os.Executable()
	return cloud.ServerSpec{Slug: slug, Command: []string{exe, "serve"}, Log: filepath.Join(t.TempDir(), "server.log"),
		Env: []string{"CELL_FAKE_ORG=1", "HAAS_ORG=" + slug, fmt.Sprintf("HAAS_ADDR=127.0.0.1:%d", port), "HAAS_CELL=fake"}}
}

func answers(port int) bool {
	response, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", port))
	if err != nil {
		return false
	}
	response.Body.Close()
	return response.StatusCode == http.StatusOK
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if ok() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s did not happen", what)
}

func TestARequestIsAcceptedOnlyIfSignedWithTheSecretForWhatItSaysAndRecently(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	build := func(method, target string, body []byte) *http.Request {
		r := httptest.NewRequest(method, target, strings.NewReader(string(body)))
		sign(r, secret, now, body)
		return r
	}
	if _, err := verify(build("POST", "/v1/servers", []byte(`{"a":1}`)), secret, now); err != nil {
		t.Fatalf("a signed request: %v", err)
	}
	for name, tamper := range map[string]func(*http.Request){
		"a changed body":   func(r *http.Request) { r.Body = http.NoBody },
		"a changed path":   func(r *http.Request) { r.URL.Path = "/v1/status" },
		"a changed query":  func(r *http.Request) { r.URL.RawQuery = "from=1" },
		"a changed method": func(r *http.Request) { r.Method = "GET" },
		"a changed time":   func(r *http.Request) { r.Header.Set(timeHeader, "1000001") },
		"no signature":     func(r *http.Request) { r.Header.Del(signatureHeader) },
		"a bad signature":  func(r *http.Request) { r.Header.Set(signatureHeader, "zz") },
		"no time":          func(r *http.Request) { r.Header.Del(timeHeader) },
	} {
		r := build("POST", "/v1/servers", []byte(`{"a":1}`))
		tamper(r)
		if _, err := verify(r, secret, now); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	old := build("POST", "/v1/servers", nil)
	if _, err := verify(old, secret, now.Add(skew+time.Second)); err == nil {
		t.Error("a request from a minute ago was accepted, and could be sent again")
	}
	if _, err := verify(build("POST", "/v1/servers", nil), secret, now.Add(-skew-time.Second)); err == nil {
		t.Error("a request from the future was accepted")
	}
	if _, err := verify(build("POST", "/v1/servers", nil), []byte("another secret of thirty two byte"), now); err == nil {
		t.Error("a request signed with another secret was accepted")
	}
	if _, err := verify(build("POST", "/v1/servers", nil), nil, now); err == nil {
		t.Error("an agent with no secret accepted a request")
	}
	// Both sides having no secret is a front door and a cell that were never given one, and is not agreement.
	unsecured := httptest.NewRequest("POST", "/v1/servers", nil)
	sign(unsecured, nil, now, nil)
	if _, err := verify(unsecured, nil, now); err == nil {
		t.Error("a request signed with no secret, to an agent with none, was accepted")
	}
}

func TestACellRegistersReadsBackAndIsFreshOnlyWhileItIsHeardFrom(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cells")
	now := time.Unix(2_000_000, 0)
	if err := Register(dir, Registration{Name: "cell-a", Agent: "10.0.0.5:8060", Private: "10.0.0.5", Beat: now.Unix()}); err != nil {
		t.Fatal(err)
	}
	Register(dir, Registration{Name: "cell-b", Agent: "10.0.0.6:8060", Private: "10.0.0.6", Beat: now.Add(-time.Minute).Unix()})
	got, age, err := Registered(dir, "cell-a", now.Add(5*time.Second))
	if err != nil || got.Agent != "10.0.0.5:8060" || age != 5*time.Second {
		t.Fatalf("%+v %v %v", got, age, err)
	}
	fresh, _ := Fresh(dir, now, 30*time.Second)
	if len(fresh) != 1 || fresh[0].Name != "cell-a" {
		t.Errorf("only a cell heard from lately is fresh: %+v", fresh)
	}
	for _, bad := range []string{"", "../x", "a/b", "UPPER", "-lead", "orchestrator", strings.Repeat("a", 40), "a.b"} {
		if Register(dir, Registration{Name: bad, Agent: "x", Private: "y"}) == nil {
			t.Errorf("%q may not be a cell's name", bad)
		}
		if _, _, err := Registered(dir, bad, now); err == nil {
			t.Errorf("%q was read", bad)
		}
	}
	os.WriteFile(filepath.Join(dir, "cell-c.json"), []byte("not json"), 0o644)
	if _, _, err := Registered(dir, "cell-c", now); err == nil {
		t.Error("a file that is not a registration was read")
	}
	if fresh, _ := Fresh(dir, now, 30*time.Second); len(fresh) != 1 {
		t.Errorf("a broken file is not a cell: %+v", fresh)
	}
	if none, err := Fresh(filepath.Join(t.TempDir(), "nothing"), now, time.Minute); err != nil || none != nil {
		t.Errorf("no directory is no cells: %v %v", none, err)
	}
	if err := Beat(dir, now); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(filepath.Join(dir, "orchestrator.beat")); info == nil || !info.ModTime().Equal(now) {
		t.Errorf("the front door's beat: %v", info)
	}
	if names, _ := Fresh(dir, now, time.Hour); len(names) != 2 {
		t.Errorf("the beat is not a cell: %+v", names)
	}
}

func TestTheFrontDoorStartsAsksAndStopsAServerOnACellThroughItsAgent(t *testing.T) {
	host := newAgent(t, "cell-a", 9000)
	remote := remoteTo(host, "cell-a")
	server, proxy, err := remote.Ports(23000+int(time.Now().UnixNano()%9000), map[int]bool{})
	if err != nil || server == 0 || proxy == 0 || server == proxy {
		t.Fatalf("ports %d %d: %v", server, proxy, err)
	}
	if err := remote.Start(fakeOrg(t, "acme-com", server)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the server answering", func() bool { return answers(server) })
	if !remote.Alive("acme-com") || remote.Alive("other-com") {
		t.Error("alive is what is running there")
	}
	status, err := remote.Status()
	if err != nil || status.Name != "cell-a" || status.MemoryAvailableMiB != 9000 || len(status.Servers) != 1 || status.Servers[0] != "acme-com" {
		t.Errorf("%+v %v", status, err)
	}
	if again := remote.Start(fakeOrg(t, "acme-com", server)); again != nil {
		t.Errorf("starting one that is running does nothing: %v", again)
	}
	if err := remote.Stop("acme-com", 3*time.Second); err != nil {
		t.Fatal(err)
	}
	if remote.Alive("acme-com") || answers(server) {
		t.Error("the server is still there")
	}
}

func TestAPortAlreadyGivenOrInUseIsNotGivenAgain(t *testing.T) {
	host := newAgent(t, "cell-a", 9000)
	remote := remoteTo(host, "cell-a")
	from := 24000 + int(time.Now().UnixNano()%9000)
	held, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", from+1))
	if err != nil {
		t.Skip("the port was taken")
	}
	defer held.Close()
	server, proxy, err := remote.Ports(from, map[int]bool{from: true})
	if err != nil || server < from+2 || proxy < from+2 || server == proxy {
		t.Errorf("%d %d: %v, and %d is given and %d is in use", server, proxy, err, from, from+1)
	}
}

func TestAnAgentDoesWhatIsSignedAndRefusesEverythingElse(t *testing.T) {
	host := newAgent(t, "cell-a", 9000)
	wrong := newRemote(host.registration("cell-a", time.Now()), []byte("another secret of thirty two byte"))
	if _, err := wrong.Status(); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("a wrong secret: %v", err)
	}
	if wrong.Start(fakeOrg(t, "acme-com", 25000)) == nil || len(host.servers.Running()) != 0 {
		t.Error("something was started for a request that was not signed")
	}
	plain, err := http.Get(host.server.URL + "/v1/status")
	if err != nil || plain.StatusCode != http.StatusForbidden {
		t.Errorf("an unsigned request: %v %v", plain, err)
	}
	remote := remoteTo(host, "cell-a")
	for name, spec := range map[string]cloud.ServerSpec{
		"a slug that names a path": {Slug: "../x", Command: []string{"true"}, Log: "/tmp/x"},
		"no command":               {Slug: "acme-com", Log: "/tmp/x"},
		"no log":                   {Slug: "acme-com", Command: []string{"true"}},
	} {
		if err := remote.Start(spec); err == nil || !strings.Contains(err.Error(), "400") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestAnUnreachableCellIsNotAliveAndSaysWhy(t *testing.T) {
	host := newAgent(t, "cell-a", 9000)
	remote := remoteTo(host, "cell-a")
	host.server.Close()
	if remote.Alive("acme-com") {
		t.Error("a cell that cannot be asked has running servers")
	}
	if _, err := remote.Status(); err == nil || !strings.Contains(err.Error(), "cell-a did not answer") {
		t.Errorf("%v", err)
	}
}
