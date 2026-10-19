package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/getlatentic/gusto/internal/helper"
	"github.com/getlatentic/gusto/internal/helper/mirror"
)

const (
	// folderLook is how often the agents are listed to see which have a folder here.
	folderLook = 10 * time.Second
	// folderRetry is how long a folder that stopped waits before it is started again.
	folderRetry = time.Minute
)

// FolderSync keeps each folder the desktop app registered the same as the folder of the agent started for it. The app registers a
// folder by the key it made of its path; the agent is the one the server holds under that key, which is started by the person's first
// message in the folder, so it is looked for as it appears.
type FolderSync struct {
	Server string
	Vault  helper.Vault
	// Dir is the helper's settings folder, where the registered folders are kept.
	Dir string

	mu      sync.Mutex
	paths   map[string]string
	running map[string]*kept
	kick    chan struct{}
}

type kept struct {
	instance string
	cancel   context.CancelFunc
	stopped  time.Time
	done     chan struct{}
}

func (f *FolderSync) file() string { return filepath.Join(f.Dir, "folders.json") }

// Open reads the folders registered before.
func (f *FolderSync) Open() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paths, f.running, f.kick = map[string]string{}, map[string]*kept{}, make(chan struct{}, 1)
	text, err := os.ReadFile(f.file())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(text, &f.paths)
}

// Register keeps a folder. It will not keep one that is not a folder, or is the whole of a home or a disk, which a person who chose
// it did not mean to hand to an agent.
func (f *FolderSync) Register(key, path string) error {
	if !helper.FolderKey.MatchString(key) {
		return errors.New("that is not a folder key")
	}
	if !filepath.IsAbs(path) {
		return errors.New("a folder is named by its whole path")
	}
	path = filepath.Clean(path)
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		return errors.New("that is not a folder")
	}
	if home, err := os.UserHomeDir(); path == home || filepath.Dir(path) == path || (err == nil && filepath.Dir(home) == path) {
		return errors.New("that is a whole home or disk, which is too much to keep: choose a folder inside it")
	}
	f.mu.Lock()
	f.paths[key] = path
	err := f.save()
	f.mu.Unlock()
	select {
	case f.kick <- struct{}{}:
	default:
	}
	return err
}

// Registered is every folder kept.
func (f *FolderSync) Registered() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make(map[string]string, len(f.paths))
	for key, path := range f.paths {
		out[key] = path
	}
	return out
}

func (f *FolderSync) save() error {
	text, err := json.Marshal(f.paths)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		return err
	}
	scratch := f.file() + ".new"
	if err := os.WriteFile(scratch, text, 0o600); err != nil {
		return err
	}
	return os.Rename(scratch, f.file())
}

// Run looks for the agents of the registered folders and keeps each, until the context ends.
func (f *FolderSync) Run(ctx context.Context) {
	defer f.stopAll()
	for {
		f.look(ctx)
		select {
		case <-ctx.Done():
			return
		case <-f.kick:
		case <-time.After(folderLook):
		}
	}
}

func (f *FolderSync) look(ctx context.Context) {
	token, err := f.Vault.Get(f.Server)
	if err != nil {
		return
	}
	client := helper.Client{Server: f.Server, Token: token}
	agents, err := client.Instances(ctx)
	if err != nil {
		return
	}
	held := map[string]helper.Instance{}
	for _, agent := range agents {
		if agent.Folder != "" {
			held[agent.Folder] = agent
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for key, going := range f.running {
		if agent, found := held[key]; !found || agent.ID != going.instance {
			going.cancel()
			delete(f.running, key)
		}
	}
	for key, path := range f.paths {
		agent, found := held[key]
		if !found {
			continue
		}
		if going := f.running[key]; going != nil && !going.finished() {
			continue
		} else if going != nil && time.Since(going.stopped) < folderRetry {
			continue
		}
		f.start(ctx, key, path, agent, client)
	}
}

func (k *kept) finished() bool {
	select {
	case <-k.done:
		return true
	default:
		return false
	}
}

func (f *FolderSync) start(ctx context.Context, key, path string, agent helper.Instance, client helper.Client) {
	engine, err := newEngine(path, f.Server, agent, client, false)
	if err != nil {
		fmt.Fprintf(os.Stderr, "the folder %s is not kept: %v\n", path, err)
		return
	}
	run, cancel := context.WithCancel(ctx)
	going := &kept{instance: agent.ID, cancel: cancel, done: make(chan struct{})}
	f.running[key] = going
	fmt.Fprintf(os.Stderr, "keeping %s the same as %s's folder in the cloud\n", path, firstNonEmpty(agent.Title, agent.Harness))
	go func() {
		defer close(going.done)
		wake, slow := wakes(run, path, engine.Ignore, client, agent.ID)
		_ = engine.Keep(run, 3*time.Second, slow, wake, func(report *mirror.Report, err error) {
			if err != nil || (report != nil && len(report.Errors) > 0) {
				said(report, err)
			}
		})
		f.mu.Lock()
		going.stopped = time.Now()
		f.mu.Unlock()
	}()
}

func (f *FolderSync) stopAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for key, going := range f.running {
		going.cancel()
		delete(f.running, key)
	}
}
