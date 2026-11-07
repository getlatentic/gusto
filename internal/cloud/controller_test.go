package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getlatentic/gusto/internal/roster"
)

func whoIs(t *testing.T, address string) map[string]any {
	t.Helper()
	response, err := http.Get("http://" + address + "/whoami")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var said map[string]any
	if err := json.NewDecoder(response.Body).Decode(&said); err != nil {
		t.Fatal(err)
	}
	return said
}

func made(t *testing.T, s *Store, email, name string) Org {
	t.Helper()
	_, org, err := s.SignUp(t.Context(), Signup{Email: email, OrgName: name})
	if err != nil {
		t.Fatal(err)
	}
	return org
}

func TestAnOrganizationsServerIsMadeAndStartedWithOnlyItsOwnDirectoriesKeysAndSettings(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s)
		org := made(t, s, "ada@acme.com", "Acme")
		if err := c.Provision(t.Context(), org.Slug); err != nil {
			t.Fatal(err)
		}
		address, state := c.Route(t.Context(), org.Slug)
		if state != Ready || address == "" {
			t.Fatalf("%q %q", address, state)
		}
		said := whoIs(t, address)
		data := s.DataDir(org.Slug)
		for field, want := range map[string]string{
			"org": org.Slug, "orgName": "Acme", "jobs": filepath.Join(data, "jobs"), "quota": s.QuotaFile(org.Slug), "members": s.RosterFile(org.Slug), "inbox": s.InboxDir(org.Slug), "orgSettings": filepath.Join(data, "settings.json"), "home": filepath.Join(s.OrgDir(org.Slug), "home"),
			"entryKey": filepath.Join(s.OrgDir(org.Slug), "entry.key"), "signingKey": filepath.Join(data, "keys", org.Slug+".key"),
			"front": "https://cloud.test", "concurrency": "2", "operatorSetting": "given",
		} {
			if said[field] != want {
				t.Errorf("%s = %v, want %s", field, said[field], want)
			}
		}
		if said["mailVisible"] != false {
			t.Error("the front door's mail password reached an organization's server")
		}
		if !strings.HasPrefix(fmt.Sprint(said["addr"]), "127.0.0.1:") || said["addr"] == said["proxyAddr"] {
			t.Errorf("a server listens on the loopback, and its proxy is another port: %v %v", said["addr"], said["proxyAddr"])
		}
		if list, err := roster.Read(s.RosterFile(org.Slug)); err != nil || len(list.Members) != 1 || list.Members[0].Email != "ada@acme.com" {
			t.Errorf("the server is started with the list of its people already written: %+v %v", list, err)
		}
		for _, private := range []string{s.RosterFile(org.Slug), filepath.Join(s.OrgDir(org.Slug), "entry.key"), filepath.Join(data, "keys", org.Slug+".key")} {
			if info, err := os.Stat(private); err != nil || info.Mode().Perm()&0o077 != 0 {
				t.Errorf("%s should be private: %v %v", private, info, err)
			}
		}
		if key, _ := os.ReadFile(filepath.Join(s.OrgDir(org.Slug), "entry.key")); len(key) < 32 {
			t.Errorf("the entry key is too short: %d", len(key))
		}
		for _, dir := range []string{"jobs", "sessions", "setups", "comparisons", "changes", "retention", "keys", "computers", "inbox"} {
			if info, err := os.Stat(filepath.Join(data, dir)); err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
				t.Errorf("%s: %v %v", dir, info, err)
			}
		}
		runtime, _ := s.Runtime(t.Context(), org.Slug)
		if runtime.State != Ready || runtime.Port == 0 || runtime.ProxyPort == 0 || runtime.Node != "local" {
			t.Errorf("what was done is written down: %+v", runtime)
		}
	})
}

func TestOrganizationsProvisionedTogetherShareNeitherAPortNorADirectory(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{PerDomain: 100})
		c := controlled(t, s)
		var orgs []Org
		for i := range 6 {
			orgs = append(orgs, made(t, s, fmt.Sprintf("a@company%d.com", i), fmt.Sprintf("Company %d", i)))
		}
		var wg sync.WaitGroup
		for _, org := range orgs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := c.Provision(t.Context(), org.Slug); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Wait()
		ports, jobs := map[string]bool{}, map[string]bool{}
		for _, org := range orgs {
			address, state := c.Route(t.Context(), org.Slug)
			if state != Ready {
				logged, _ := os.ReadFile(filepath.Join(s.OrgDir(org.Slug), "server.log"))
				runtime, _ := s.Runtime(t.Context(), org.Slug)
				t.Fatalf("%s: %q, port %d, server said:\n%s", org.Slug, state, runtime.Port, logged)
			}
			said := whoIs(t, address)
			if said["org"] != org.Slug {
				t.Errorf("%s answered as %v", org.Slug, said["org"])
			}
			runtime, _ := s.Runtime(t.Context(), org.Slug)
			for _, port := range []string{fmt.Sprint(runtime.Port), fmt.Sprint(runtime.ProxyPort)} {
				if ports[port] {
					t.Errorf("port %s was given twice", port)
				}
				ports[port] = true
			}
			if jobs[fmt.Sprint(said["jobs"])] {
				t.Errorf("a directory was shared: %v", said["jobs"])
			}
			jobs[fmt.Sprint(said["jobs"])] = true
		}
	})
}

func TestProvisioningAgainChangesNothingAndDoesNotRestartTheServer(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s)
		org := made(t, s, "ada@acme.com", "Acme")
		c.Provision(t.Context(), org.Slug)
		address, _ := c.Route(t.Context(), org.Slug)
		first := whoIs(t, address)
		before, _ := s.Runtime(t.Context(), org.Slug)
		key, _ := os.ReadFile(filepath.Join(s.OrgDir(org.Slug), "entry.key"))
		if err := c.Provision(t.Context(), org.Slug); err != nil {
			t.Fatal(err)
		}
		again, _ := c.Route(t.Context(), org.Slug)
		after, _ := s.Runtime(t.Context(), org.Slug)
		if again != address || whoIs(t, again)["pid"] != first["pid"] || after.Port != before.Port {
			t.Error("a second pass restarted the server or moved it")
		}
		if key2, _ := os.ReadFile(filepath.Join(s.OrgDir(org.Slug), "entry.key")); string(key2) != string(key) {
			t.Error("the entry key was made again, which signs everybody out")
		}
	})
}

func TestAServerThatStopsIsStartedAgain(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s)
		org := made(t, s, "ada@acme.com", "Acme")
		c.Provision(t.Context(), org.Slug)
		address, _ := c.Route(t.Context(), org.Slug)
		first := whoIs(t, address)
		http.Get("http://" + address + "/die")
		deadline := time.Now().Add(5 * time.Second)
		for c.Node.Alive(org.Slug) && time.Now().Before(deadline) {
			time.Sleep(20 * time.Millisecond)
		}
		if _, state := c.Route(t.Context(), org.Slug); state != Starting {
			t.Errorf("a server whose process has gone is not routed to: %q", state)
		}
		c.supervise(t.Context())
		again, state := c.Route(t.Context(), org.Slug)
		if state != Ready || again != address {
			t.Fatalf("%q %q", again, state)
		}
		if said := whoIs(t, again); said["pid"] == first["pid"] {
			t.Error("the same process answered")
		}
		if runtime, _ := s.Runtime(t.Context(), org.Slug); runtime.Restarts != 1 {
			t.Errorf("a restart is counted: %+v", runtime)
		}
	})
}

func TestAServerThatCannotStartIsFailedWithAReasonAndTriedAgainLessOften(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s, "FAKE_FAIL=1")
		c.Wait = 600 * time.Millisecond
		org := made(t, s, "ada@acme.com", "Acme")
		if err := c.Provision(t.Context(), org.Slug); err == nil {
			t.Fatal("a server that never answers is an error")
		}
		if address, state := c.Route(t.Context(), org.Slug); state != Failed || address != "" {
			t.Errorf("%q %q", address, state)
		}
		runtime, _ := s.Runtime(t.Context(), org.Slug)
		if runtime.State != Failed || !strings.Contains(runtime.Error, "server.log") {
			t.Errorf("the reason is written down, and says where to look: %+v", runtime)
		}
		// The controller gives up when Wait is over, which on a loaded machine can be before the server has said why it failed.
		waitFor(t, func() bool {
			log, _ := os.ReadFile(filepath.Join(s.OrgDir(org.Slug), "server.log"))
			return strings.Contains(string(log), "told to fail")
		})
		c.supervise(t.Context())
		first := c.retry[org.Slug]
		if first.IsZero() {
			t.Fatal("a failure sets when to try again")
		}
		before, _ := os.ReadFile(filepath.Join(s.OrgDir(org.Slug), "server.log"))
		c.supervise(t.Context()) // too soon: nothing is tried
		after, _ := os.ReadFile(filepath.Join(s.OrgDir(org.Slug), "server.log"))
		if string(before) != string(after) {
			t.Error("a server that failed was tried again at once")
		}
	})
}

// A server that is slow to start is not waited for by the front door: asking where
// it is answers at once, and says it is starting.
func TestRoutingDoesNotWaitForAServerThatIsStarting(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s, "FAKE_DELAY=1500ms")
		org := made(t, s, "ada@acme.com", "Acme")
		done := make(chan error, 1)
		go func() { done <- c.Provision(t.Context(), org.Slug) }()
		deadline := time.Now().Add(3 * time.Second)
		for {
			if _, state := c.Route(t.Context(), org.Slug); state == Starting {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("the organization was never starting")
			}
			time.Sleep(10 * time.Millisecond)
		}
		began := time.Now()
		for range 200 {
			c.Route(t.Context(), org.Slug)
		}
		if took := time.Since(began); took > 200*time.Millisecond {
			t.Errorf("routing waited on a server that was starting: %s", took)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if _, state := c.Route(t.Context(), org.Slug); state != Ready {
			t.Errorf("%q", state)
		}
	})
}

// Run starts every organization, and leaves them running when it ends, for the next leader; StopAll, when the front door
// stops, asks each to stop.
func TestRunStartsEveryOrganizationAndStopAllStopsThem(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{PerDomain: 100})
		c := controlled(t, s)
		a := made(t, s, "a@acme.com", "Acme")
		b := made(t, s, "b@globex.com", "Globex")
		ctx, cancel := context.WithCancel(context.Background())
		stopped := make(chan struct{})
		go func() { c.Run(ctx, 50*time.Millisecond); close(stopped) }()
		waitFor(t, func() bool {
			_, sa := c.Route(ctx, a.Slug)
			_, sb := c.Route(ctx, b.Slug)
			return sa == Ready && sb == Ready
		})
		cancel()
		<-stopped
		for _, org := range []Org{a, b} {
			if !c.Node.Alive(org.Slug) {
				t.Errorf("%s stopped when Run ended", org.Slug)
			}
		}
		c.StopAll()
		for _, org := range []Org{a, b} {
			if c.Node.Alive(org.Slug) {
				t.Errorf("%s is still running", org.Slug)
			}
			if marker, _ := os.ReadFile(filepath.Join(s.DataDir(org.Slug), "stopped")); string(marker) != "asked\n" {
				t.Errorf("%s was killed and not asked to stop: %q", org.Slug, marker)
			}
		}
	})
}

func TestAServerThatIgnoresBeingAskedToStopIsMadeToAfterTheGrace(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s, "FAKE_IGNORE_TERM=1")
		c.StopGrace = 300 * time.Millisecond
		org := made(t, s, "ada@acme.com", "Acme")
		c.Provision(t.Context(), org.Slug)
		began := time.Now()
		c.StopAll()
		if c.Node.Alive(org.Slug) || time.Since(began) < 250*time.Millisecond || time.Since(began) > 5*time.Second {
			t.Errorf("stopped in %s, alive %v", time.Since(began), c.Node.Alive(org.Slug))
		}
	})
}

// What was Ready before the front door restarted is starting until it answers again,
// and not gone: a request in that moment is told to wait.
func TestAnOrganizationThatWasReadyIsStartingAgainWhenTheFrontDoorRestarts(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		first := controlled(t, s)
		org := made(t, s, "ada@acme.com", "Acme")
		first.Provision(t.Context(), org.Slug)
		first.StopAll()

		second := controlled(t, s, "FAKE_DELAY=1s")
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go second.Run(ctx, time.Hour)
		waitFor(t, func() bool { _, state := second.Route(ctx, org.Slug); return state == Starting })
		waitFor(t, func() bool { _, state := second.Route(ctx, org.Slug); return state == Ready })
	})
}

func waitFor(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatal("it did not happen in time")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

var _ = io.Discard

// Ports are written down as they are given, so the next organization to be given
// some, at the same moment, is not given the same. Nothing is listening on them yet,
// so the record is all that keeps two apart.
func TestPortsGivenAtTheSameMomentAreAllDifferent(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{PerDomain: 100})
		c := controlled(t, s)
		var orgs []Org
		for i := range 25 {
			orgs = append(orgs, made(t, s, fmt.Sprintf("a@company%d.com", i), fmt.Sprintf("Company %d", i)))
		}
		var wg sync.WaitGroup
		var mu sync.Mutex
		given := map[int]string{}
		for _, org := range orgs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				runtime, err := c.givePorts(t.Context(), org.Slug, Runtime{Node: "local"}, c.Node)
				if err != nil {
					t.Error(err)
					return
				}
				mu.Lock()
				defer mu.Unlock()
				for _, port := range []int{runtime.Port, runtime.ProxyPort} {
					if other, twice := given[port]; twice {
						t.Errorf("port %d was given to %s and to %s", port, other, org.Slug)
					}
					given[port] = org.Slug
				}
			}()
		}
		wg.Wait()
		if len(given) != 50 {
			t.Errorf("%d ports were given to 25 organizations", len(given))
		}
	})
}

// An organization the supervisor saw stop, and that is removed before it is started again, stays removed: its server is not
// started, and the controller does not hold it again.
func TestARemovedOrganizationIsNotStartedAgain(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s)
		org := made(t, s, "ada@acme.com", "Acme")
		c.Provision(t.Context(), org.Slug)
		c.Remove(t.Context(), org.Slug)
		c.restart(t.Context(), org.Slug)
		if _, state := c.Route(t.Context(), org.Slug); state != "" {
			t.Fatalf("a removed organization is %q", state)
		}
		if c.Node.Alive(org.Slug) {
			t.Error("a removed organization's server was started again")
		}
		if err := c.reprovision(t.Context(), org.Slug); err != nil || c.Node.Alive(org.Slug) {
			t.Errorf("a retry for a removed organization started it: %v", err)
		}
	})
}

// A call to provision decided before Remove, as the signup's and Run's are, that runs after it is refused: the trace TLC
// finds in spec/OrgSupervisor.before-provision.cfg.
func TestAProvisionThatRunsAfterRemoveIsRefused(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s)
		org := made(t, s, "ada@acme.com", "Acme")
		c.Remove(t.Context(), org.Slug)
		if err := c.Provision(t.Context(), org.Slug); !errors.Is(err, ErrRemoved) {
			t.Errorf("provisioning a removed organization said %v", err)
		}
		if c.Node.Alive(org.Slug) {
			t.Error("a removed organization's server was started")
		}
		if _, state := c.Route(t.Context(), org.Slug); state != "" {
			t.Errorf("a removed organization is %q", state)
		}
	})
}

// A demo the store has deleted is not made again: no server, and no directory.
func TestAnOrganizationGoneFromTheStoreIsNotProvisioned(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s, _ := clocked(t, demoSet)
		c := controlled(t, s)
		_, org, err := s.StartDemo(t.Context(), "Ada", "ada@acme.com", "Acme")
		if err != nil {
			t.Fatal(err)
		}
		c.Remove(t.Context(), org.Slug)
		if err := s.EndDemo(t.Context(), org.Slug, "test"); err != nil {
			t.Fatal(err)
		}
		c.Ended(org.Slug)
		if err := c.Provision(t.Context(), org.Slug); !errors.Is(err, ErrRemoved) {
			t.Errorf("provisioning a deleted organization said %v", err)
		}
		if c.Node.Alive(org.Slug) {
			t.Error("a deleted organization's server was started")
		}
		if _, err := os.Stat(s.OrgDir(org.Slug)); !os.IsNotExist(err) {
			t.Errorf("a deleted organization's directory is back: %v", err)
		}
	})
}

// A new organization at the address of one that was removed is provisioned: it is told apart by when it was made.
func TestANewOrganizationAtARemovedAddressIsProvisioned(t *testing.T) {
	onEachDatabase(t, func(t *testing.T) {
		s := open(t, Settings{})
		c := controlled(t, s)
		org := made(t, s, "ada@acme.com", "Acme")
		c.removed = map[string]time.Time{org.Slug: org.Created.Add(-time.Hour)}
		if err := c.Provision(t.Context(), org.Slug); err != nil {
			t.Fatal(err)
		}
		if !c.Node.Alive(org.Slug) {
			t.Error("a new organization at a removed address was not started")
		}
	})
}
