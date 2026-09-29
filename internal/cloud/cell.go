package cloud

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// ServerSpec is how to run one organisation's server: the command, the whole of its
// environment, and where its output goes. Nothing of the front door's own
// environment is in it, so what the front door holds, its mail password among
// it, is not what an organisation's server can read.
type ServerSpec struct {
	Slug    string
	Command []string
	Env     []string
	Log     string
}

// Cell is where organisations' servers run. The first is this machine. Another is
// a host with an agent on it, which the controller asks in the same words.
type Cell interface {
	Name() string
	// Start runs the server, and does nothing when it is already running.
	Start(spec ServerSpec) error
	// Stop ends it, asking first and waiting up to grace before it insists.
	Stop(slug string, grace time.Duration) error
	// Alive says the server's process is running.
	Alive(slug string) bool
}

// LocalCell runs organisations' servers as children of this process. They end
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

func (l *LocalCell) Name() string { return "local" }

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
