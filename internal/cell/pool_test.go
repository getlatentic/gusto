package cell

import (
	"context"
	"encoding/json"
	"fmt"
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

// pooled is a pool over a directory of registrations, with the cells given.
func pooled(t *testing.T, min int, cells map[string]*agentHost) *Pool {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "cells")
	for name, host := range cells {
		if err := Register(dir, host.registration(name, time.Now())); err != nil {
			t.Fatal(err)
		}
	}
	return &Pool{Dir: dir, Secret: secret, MaxAge: time.Minute, MinFreeMiB: min}
}

func TestAnOrganisationIsPlacedOnTheCellWithTheMostRoomThatHasEnough(t *testing.T) {
	pool := pooled(t, 4096, map[string]*agentHost{
		"cell-a": newAgent(t, "cell-a", 6000),
		"cell-b": newAgent(t, "cell-b", 12000),
		"cell-c": newAgent(t, "cell-c", 9000),
	})
	placed, err := pool.Place()
	if err != nil || placed.Name() != "cell-b" {
		t.Fatalf("%v %v", placed, err)
	}
	// Equal room is broken by the name, so the choice does not depend on the order of a listing.
	tie := pooled(t, 0, map[string]*agentHost{"cell-b": newAgent(t, "cell-b", 8000), "cell-a": newAgent(t, "cell-a", 8000)})
	if placed, _ := tie.Place(); placed.Name() != "cell-a" {
		t.Errorf("a tie went to %s", placed.Name())
	}
	tight := pooled(t, 4096, map[string]*agentHost{"cell-a": newAgent(t, "cell-a", 3000), "cell-b": newAgent(t, "cell-b", 2000)})
	if _, err := tight.Place(); err == nil || !strings.Contains(err.Error(), "4096 MiB") {
		t.Errorf("a cell without room was placed on: %v", err)
	}
	if _, err := (&Pool{Dir: filepath.Join(t.TempDir(), "none"), Secret: secret}).Place(); err == nil {
		t.Error("no cells is nowhere to place")
	}
}

func TestACellThatIsSilentOrDoesNotAnswerIsNeverChosenOrReached(t *testing.T) {
	quiet := newAgent(t, "cell-a", 20000)
	broken := newAgent(t, "cell-b", 30000)
	well := newAgent(t, "cell-c", 5000)
	dir := filepath.Join(t.TempDir(), "cells")
	Register(dir, quiet.registration("cell-a", time.Now().Add(-5*time.Minute)))
	Register(dir, broken.registration("cell-b", time.Now()))
	Register(dir, well.registration("cell-c", time.Now()))
	broken.server.Close()
	pool := &Pool{Dir: dir, Secret: secret, MaxAge: time.Minute}

	if placed, err := pool.Place(); err != nil || placed.Name() != "cell-c" {
		t.Errorf("the roomiest is silent and the next does not answer: %v %v", placed, err)
	}
	if _, err := pool.Get("cell-a"); err == nil || !strings.Contains(err.Error(), "has not been heard from") {
		t.Errorf("%v", err)
	}
	if _, err := pool.Get("nobody"); err == nil || !strings.Contains(err.Error(), "no cell called") {
		t.Errorf("%v", err)
	}
	if got, err := pool.Get("cell-c"); err != nil || got.Name() != "cell-c" || got.Host() != "127.0.0.1" {
		t.Errorf("%v %v", got, err)
	}
}

func TestACellThatDoesNotAnswerIsNotPlacedOnEvenWhenItIsTheOnlyOne(t *testing.T) {
	broken := newAgent(t, "cell-a", 30000)
	pool := pooled(t, 0, map[string]*agentHost{"cell-a": broken})
	broken.server.Close()
	if placed, err := pool.Place(); err == nil {
		t.Errorf("placed on %s, which does not answer", placed.Name())
	}
}

func TestThisMachineIsACellToPlaceOnOnlyWhenTheFrontDoorSaysSo(t *testing.T) {
	remote := newAgent(t, "cell-a", 6000)
	pool := pooled(t, 1000, map[string]*agentHost{"cell-a": remote})
	pool.Measure = func() Capacity { return Capacity{MemoryAvailableMiB: 15000} }
	if placed, _ := pool.Place(); placed.Name() != "cell-a" {
		t.Errorf("a front door that runs no servers placed on %s", placed.Name())
	}
	pool.Local = &cloud.LocalCell{}
	if placed, _ := pool.Place(); placed.Name() != "local" {
		t.Errorf("this machine has the most room: %s", placed.Name())
	}
	if got, err := pool.Get("local"); err != nil || got != pool.Local {
		t.Errorf("%v %v", got, err)
	}
}

// controlledOn is a controller that places organisations on the pool, running the test binary as each server.
func controlledOn(t *testing.T, pool *Pool) (*cloud.Controller, *cloud.Store) {
	t.Helper()
	store, err := cloud.Open(filepath.Join(t.TempDir(), "cloud"), cloud.Settings{PerDomain: 100, Limits: cloud.DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	c := &cloud.Controller{Store: store, Cells: pool, Command: []string{exe, "serve"}, Env: []string{"CELL_FAKE_ORG=1", "PATH=" + os.Getenv("PATH")},
		FrontDoor: "https://cloud.test", PortFrom: 26000 + int(time.Now().UnixNano()%9000), Wait: 20 * time.Second, Concurrent: 1, StopGrace: time.Second}
	return c, store
}

func whoami(t *testing.T, address string) map[string]string {
	t.Helper()
	response, err := http.Get("http://" + address + "/whoami")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var seen map[string]string
	json.NewDecoder(response.Body).Decode(&seen)
	return seen
}

func TestOrganisationsAreSpreadAcrossCellsByRoomStayWhereTheyWerePlacedAndAreReachedThere(t *testing.T) {
	a, b := newAgent(t, "cell-a", 12000), newAgent(t, "cell-b", 11000)
	pool := pooled(t, 4096, map[string]*agentHost{"cell-a": a, "cell-b": b})
	c, store := controlledOn(t, pool)
	signUp := func(email, name string) string {
		_, org, err := store.SignUp(cloud.Signup{Email: email, OrgName: name})
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Provision(org.Slug); err != nil {
			t.Fatal(err)
		}
		return org.Slug
	}
	first := signUp("ada@acme.com", "Acme")
	a.setFree(2000) // the first organisation took what cell-a had
	second := signUp("bob@globex.com", "Globex")

	placed := func(slug string) string { r, _ := store.Runtime(slug); return r.Cell }
	if placed(first) != "cell-a" || placed(second) != "cell-b" {
		t.Fatalf("placed on %q and %q", placed(first), placed(second))
	}
	if len(a.servers.Running()) != 1 || a.servers.Running()[0] != first || len(b.servers.Running()) != 1 || b.servers.Running()[0] != second {
		t.Errorf("each cell runs its own: %v and %v", a.servers.Running(), b.servers.Running())
	}
	address, state := c.Route(first)
	if state != cloud.Ready || !strings.HasPrefix(address, "127.0.0.1:") {
		t.Fatalf("%q %v", address, state)
	}
	if seen := whoami(t, address); seen["org"] != first || seen["cell"] != "cell-a" || !strings.HasPrefix(seen["addr"], "127.0.0.1:") {
		t.Errorf("the server knows its own cell and where it listens: %v", seen)
	}
	// An organisation is not moved when another cell has more room: its computers are on its cell.
	a.setFree(50000)
	b.setFree(1)
	if err := c.Provision(second); err != nil {
		t.Fatal(err)
	}
	if placed(second) != "cell-b" {
		t.Errorf("an organisation was moved to %s", placed(second))
	}
}

func TestAServerWhoseCellStopsAnsweringIsNotRoutedToAndIsStartedAgainWhenTheCellIsBack(t *testing.T) {
	host := newAgent(t, "cell-a", 12000)
	pool := pooled(t, 1000, map[string]*agentHost{"cell-a": host})
	c, store := controlledOn(t, pool)
	_, org, _ := store.SignUp(cloud.Signup{Email: "ada@acme.com", OrgName: "Acme"})
	if err := c.Provision(org.Slug); err != nil {
		t.Fatal(err)
	}
	// The cell goes quiet: its registration is old.
	Register(pool.Dir, host.registration("cell-a", time.Now().Add(-10*time.Minute)))
	if address, state := c.Route(org.Slug); address != "" || state != cloud.Starting {
		t.Errorf("a server on a cell that is not heard from is routed to: %q %v", address, state)
	}
	if err := c.Provision(org.Slug); err == nil || !strings.Contains(err.Error(), "has not been heard from") {
		t.Errorf("provisioning on a silent cell: %v", err)
	}
	if r, _ := store.Runtime(org.Slug); r.State != cloud.Failed || r.Cell != "cell-a" {
		t.Errorf("it is failed, and still placed where it was: %+v", r)
	}
	Register(pool.Dir, host.registration("cell-a", time.Now()))
	if err := c.Provision(org.Slug); err != nil {
		t.Fatal(err)
	}
	if _, state := c.Route(org.Slug); state != cloud.Ready {
		t.Errorf("%v", state)
	}
}

func TestPortsAreGivenPerCellSoTwoCellsMayUseTheSameOnesAndOneCellNeverDoes(t *testing.T) {
	a, b := newAgent(t, "cell-a", 12000), newAgent(t, "cell-b", 12000)
	pool := pooled(t, 1000, map[string]*agentHost{"cell-a": a, "cell-b": b})
	c, store := controlledOn(t, pool)
	seen := map[string]bool{}
	for i := 0; i < 4; i++ {
		_, org, err := store.SignUp(cloud.Signup{Email: fmt.Sprintf("u%d@company%d.com", i, i), OrgName: fmt.Sprintf("Company %d", i)})
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Provision(org.Slug); err != nil {
			t.Fatal(err)
		}
		r, _ := store.Runtime(org.Slug)
		for _, port := range []int{r.Port, r.ProxyPort} {
			key := fmt.Sprintf("%s:%d", r.Cell, port)
			if seen[key] {
				t.Errorf("%s was given twice", key)
			}
			seen[key] = true
		}
		// Placement follows the room, which two organisations on one cell would drain.
		a.setFree(a.free - 4000)
	}
}

// aCell is a cell that is not this machine: its servers are at an address of its own, and it says which ports
// it gave and what it was asked to start with. Every port it can give answers as a server.
type aCell struct {
	name, private string
	ports         []int
	mu            sync.Mutex
	asked         []map[int]bool
	started       []cloud.ServerSpec
}

func newACell(t *testing.T, name, private string) *aCell {
	t.Helper()
	c := &aCell{name: name, private: private}
	for i := 0; i < 8; i++ {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) }))
		t.Cleanup(server.Close)
		var port int
		fmt.Sscanf(server.URL[strings.LastIndex(server.URL, ":")+1:], "%d", &port)
		c.ports = append(c.ports, port)
	}
	return c
}

func (c *aCell) Name() string   { return c.name }
func (c *aCell) Host() string   { return "127.0.0.1" }
func (c *aCell) Listen() string { return c.private }
func (c *aCell) Ports(from int, taken map[int]bool) (int, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	copied := map[int]bool{}
	for port := range taken {
		copied[port] = true
	}
	c.asked = append(c.asked, copied)
	var found []int
	for _, port := range c.ports {
		if !taken[port] && len(found) < 2 {
			found = append(found, port)
		}
	}
	if len(found) < 2 {
		return 0, 0, fmt.Errorf("no ports")
	}
	return found[0], found[1], nil
}
func (c *aCell) Start(spec cloud.ServerSpec) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.started = append(c.started, spec)
	return nil
}
func (c *aCell) Stop(string, time.Duration) error { return nil }
func (c *aCell) Alive(string) bool                { return true }

type cellsOf map[string]*aCell

func (s cellsOf) Get(name string) (cloud.Cell, error) {
	if c, ok := s[name]; ok {
		return c, nil
	}
	return nil, fmt.Errorf("no cell called %q", name)
}
func (s cellsOf) Place() (cloud.Cell, error) { return s["cell-a"], nil }

func TestAServerIsToldToListenOnItsCellsPrivateAddressAndPortsAreTakenOnlyFromTheOrganisationsOnThatCell(t *testing.T) {
	a, b := newACell(t, "cell-a", "10.0.1.5"), newACell(t, "cell-b", "10.0.2.9")
	store, err := cloud.Open(filepath.Join(t.TempDir(), "cloud"), cloud.Settings{PerDomain: 100, Limits: cloud.DefaultLimits()})
	if err != nil {
		t.Fatal(err)
	}
	c := &cloud.Controller{Store: store, Cells: cellsOf{"cell-a": a, "cell-b": b}, Command: []string{"serve"}, FrontDoor: "https://cloud.test", PortFrom: 9100, Wait: 5 * time.Second}
	provision := func(email, name string) (string, cloud.Runtime) {
		_, org, err := store.SignUp(cloud.Signup{Email: email, OrgName: name})
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Provision(org.Slug); err != nil {
			t.Fatal(err)
		}
		r, _ := store.Runtime(org.Slug)
		return org.Slug, r
	}
	_, first := provision("ada@acme.com", "Acme")
	// The second is placed on the other cell, by writing down where it goes before it is provisioned.
	_, org, _ := store.SignUp(cloud.Signup{Email: "bob@globex.com", OrgName: "Globex"})
	store.SetRuntime(org.Slug, cloud.Runtime{Cell: "cell-b"})
	if err := c.Provision(org.Slug); err != nil {
		t.Fatal(err)
	}
	_, third := provision("cy@initech.com", "Initech")

	if got := a.started[0]; !containsEnv(got.Env, "HAAS_ADDR=10.0.1.5:"+fmt.Sprint(first.Port)) || !containsEnv(got.Env, "HAAS_CELL=cell-a") {
		t.Errorf("a server on a cell listens on the cell's own address: %v", got.Env)
	}
	if got := b.started[0]; !containsEnv(got.Env, "HAAS_ADDR=10.0.2.9:") || !containsEnv(got.Env, "HAAS_CELL=cell-b") {
		t.Errorf("%v", got.Env)
	}
	taken := 0
	for port := range b.asked[0] {
		if port != 0 {
			taken++
		}
	}
	if len(b.asked) != 1 || taken != 0 {
		t.Errorf("cell-b is asked for ports as though nothing were taken, since nothing is on it: %v", b.asked)
	}
	if last := a.asked[len(a.asked)-1]; !last[first.Port] || !last[first.ProxyPort] || third.Port == first.Port {
		t.Errorf("cell-a is told which of its ports are given: %v (%d then %d)", last, first.Port, third.Port)
	}
}

func containsEnv(env []string, want string) bool {
	for _, each := range env {
		if strings.HasPrefix(each, want) {
			return true
		}
	}
	return false
}

// A front door is restarted at every deploy, and the servers on other hosts keep running through it and are found
// running by the one that starts.
func TestServersOnOtherHostsOutliveTheFrontDoorThatStartedThemAndAreAdoptedByTheNext(t *testing.T) {
	host := newAgent(t, "cell-a", 12000)
	pool := pooled(t, 1000, map[string]*agentHost{"cell-a": host})
	first, store := controlledOn(t, pool)
	_, org, _ := store.SignUp(cloud.Signup{Email: "ada@acme.com", OrgName: "Acme"})
	if err := first.Provision(org.Slug); err != nil {
		t.Fatal(err)
	}
	before, state := first.Route(org.Slug)
	if state != cloud.Ready {
		t.Fatal(state)
	}
	first.Run(canceled(), time.Hour)
	if !host.servers.Alive(org.Slug) {
		t.Fatal("the server ended with the front door that started it")
	}

	next := &cloud.Controller{Store: store, Cells: pool, Command: first.Command, Env: first.Env, FrontDoor: "https://cloud.test", PortFrom: first.PortFrom, Wait: 20 * time.Second, Concurrent: 1}
	pids := func() string { return strings.Join(host.servers.Running(), ",") }
	was := pids()
	if err := next.Provision(org.Slug); err != nil {
		t.Fatal(err)
	}
	after, _ := next.Route(org.Slug)
	if after != before || pids() != was {
		t.Errorf("the next front door started another: %q then %q, %q then %q", before, after, was, pids())
	}
}

func canceled() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestACellRegistersItselfBeatsAndTakesItsRegistrationAwayWhenItStops(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cells")
	servers := &cloud.LocalCell{}
	agent := &Agent{Name: "cell-a", Private: "127.0.0.1", Secret: secret, Servers: servers}
	ctx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- agent.Run(ctx, "127.0.0.1:0", dir, 50*time.Millisecond) }()
	waitFor(t, "the cell registering", func() bool { fresh, _ := Fresh(dir, time.Now(), time.Second); return len(fresh) == 1 })
	first, _, _ := Registered(dir, "cell-a", time.Now())
	if first.Private != "127.0.0.1" || !strings.HasPrefix(first.Agent, "127.0.0.1:") {
		t.Fatalf("%+v", first)
	}
	waitFor(t, "a later beat", func() bool { again, _, _ := Registered(dir, "cell-a", time.Now()); return again.Beat > first.Beat })
	// It answers, at the address it registered, to the front door.
	remote := newRemote(first, secret)
	if status, err := remote.Status(); err != nil || status.Name != "cell-a" {
		t.Fatalf("%+v %v", status, err)
	}
	spec := fakeOrg(t, "acme-com", 27000+int(time.Now().UnixNano()%9000))
	if err := remote.Start(spec); err != nil {
		t.Fatal(err)
	}
	stop()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if servers.Alive("acme-com") {
		t.Error("a cell that is going left its servers running")
	}
	if fresh, _ := Fresh(dir, time.Now(), time.Hour); len(fresh) != 0 {
		t.Errorf("a cell that stopped is still registered: %+v", fresh)
	}
}

func TestAnOrganisationWhoseCellIsGoneForGoodIsPlacedAgainAndOneWhoseCellIsOnlySilentIsNot(t *testing.T) {
	old, fresh := newAgent(t, "cell-old", 12000), newAgent(t, "cell-new", 9000)
	pool := pooled(t, 1000, map[string]*agentHost{"cell-old": old, "cell-new": fresh})
	pool.GoneAfter = 10 * time.Minute
	c, store := controlledOn(t, pool)
	_, org, _ := store.SignUp(cloud.Signup{Email: "ada@acme.com", OrgName: "Acme"})
	if err := c.Provision(org.Slug); err != nil {
		t.Fatal(err)
	}
	if r, _ := store.Runtime(org.Slug); r.Cell != "cell-old" {
		t.Fatalf("placed on %s", r.Cell)
	}
	// Silent for five minutes: the host may come back, and the organisation's computers are on it.
	Register(pool.Dir, old.registration("cell-old", time.Now().Add(-5*time.Minute)))
	if err := c.Provision(org.Slug); err == nil {
		t.Fatal("an organisation was moved off a cell that was only quiet")
	}
	if r, _ := store.Runtime(org.Slug); r.Cell != "cell-old" {
		t.Errorf("moved to %s", r.Cell)
	}
	// Silent for a quarter of an hour: it is gone.
	Register(pool.Dir, old.registration("cell-old", time.Now().Add(-15*time.Minute)))
	if err := c.Provision(org.Slug); err != nil {
		t.Fatalf("it was not placed again: %v", err)
	}
	r, _ := store.Runtime(org.Slug)
	if r.Cell != "cell-new" || r.State != cloud.Ready || r.Port == 0 {
		t.Errorf("%+v", r)
	}
	if address, state := c.Route(org.Slug); state != cloud.Ready || whoami(t, address)["cell"] != "cell-new" {
		t.Errorf("%q %v", address, state)
	}
}
