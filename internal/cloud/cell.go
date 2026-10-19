package cloud

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"sync"
	"syscall"
	"time"
)

// ServerSpec is how to run one organization's server: the command, the whole of its
// environment, and where its output goes. Nothing of the front door's own
// environment is in it, so what the front door holds, its mail password among
// it, is not what an organization's server can read.
type ServerSpec struct {
	Slug    string
	Command []string
	Env     []string
	Log     string
}

// ErrNoCapacity says no cell has room for another organization yet. A cell may be starting for it, so an organization
// that meets it waits and is tried again soon, and is not failed.
var ErrNoCapacity = errors.New("no host has room yet")

// Cell is where organizations' servers run. The first is this machine. Another is
// a host with an agent on it, which the controller asks in the same words.
//
// An organization's data is on storage every cell and the front door see at the same paths, so a
// server started on any cell reads and writes what the front door does.
type Cell interface {
	Name() string
	// Host is where the front door reaches the servers on this cell, and Listen the address they
	// bind to. On this machine they are both loopback; on another host Listen is the address of
	// its private network, which only the front door's hosts may reach.
	Host() string
	Listen() string
	// Ports finds two ports the cell has free, from the first up, that are not in taken.
	Ports(from int, taken map[int]bool) (server, proxy int, err error)
	// Start runs the server, and does nothing when it is already running.
	Start(spec ServerSpec) error
	// Stop ends it, asking first and waiting up to grace before it insists.
	Stop(slug string, grace time.Duration) error
	// Alive says the server's process is running.
	Alive(slug string) bool
}

// LocalCell runs organizations' servers as children of this process. They end
// when it does, so a front door that is killed leaves none of them holding a port
// that its successor wants.
type LocalCell struct {
	mu       sync.Mutex
	children map[string]*child
}

type child struct {
	command *exec.Cmd
	done    chan struct{}
}

func (l *LocalCell) Name() string   { return "local" }
func (l *LocalCell) Host() string   { return "127.0.0.1" }
func (l *LocalCell) Listen() string { return "127.0.0.1" }

func (l *LocalCell) Ports(from int, taken map[int]bool) (server, proxy int, err error) {
	var found []int
	for port := from; port < from+20000 && len(found) < 2; port++ {
		if !taken[port] && free(port) {
			found = append(found, port)
		}
	}
	if len(found) < 2 {
		return 0, 0, errors.New("no ports are free for another organization's server")
	}
	return found[0], found[1], nil
}

func (l *LocalCell) Start(spec ServerSpec) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.children == nil {
		l.children = map[string]*child{}
	}
	if held, running := l.children[spec.Slug]; running && !finished(held) {
		return nil
	}
	if len(spec.Command) == 0 {
		return fmt.Errorf("no command was given for %s's server", spec.Slug)
	}
	log, err := os.OpenFile(spec.Log, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	command := exec.Command(spec.Command[0], spec.Command[1:]...)
	command.Env = spec.Env
	command.Stdout, command.Stderr = log, log
	command.SysProcAttr = dieWithParent()
	if err := command.Start(); err != nil {
		log.Close()
		return err
	}
	held := &child{command: command, done: make(chan struct{})}
	l.children[spec.Slug] = held
	go func() {
		command.Wait()
		log.Close()
		close(held.done)
	}()
	return nil
}

func finished(c *child) bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

// Running is the organizations whose servers are running here.
func (l *LocalCell) Running() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var slugs []string
	for slug, held := range l.children {
		if !finished(held) {
			slugs = append(slugs, slug)
		}
	}
	sort.Strings(slugs)
	return slugs
}

func (l *LocalCell) Alive(slug string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	held, running := l.children[slug]
	return running && !finished(held)
}

func (l *LocalCell) Stop(slug string, grace time.Duration) error {
	l.mu.Lock()
	held, running := l.children[slug]
	l.mu.Unlock()
	if !running || finished(held) {
		return nil
	}
	held.command.Process.Signal(syscall.SIGTERM)
	select {
	case <-held.done:
	case <-time.After(grace):
		held.command.Process.Kill()
		<-held.done
	}
	return nil
}

// ErrCellGone says a cell is not coming back: it is not registered, or has been silent for so long that its host is gone.
// An organization placed on one is placed again, and its computers, which were on that cell's gateway, are made anew.
var ErrCellGone = errors.New("the host is gone")

// CellSet is the cells organizations' servers may be placed on.
type CellSet interface {
	// Get is the cell with that name, or says why it cannot be reached.
	Get(name string) (Cell, error)
	// Place chooses the cell for an organization that has none: the one with the most room.
	Place() (Cell, error)
}
