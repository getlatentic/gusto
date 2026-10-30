// Package apps is the web apps that agents built and people shared, one file each. An app
// belongs to an agent's workspace, and its record says where it is and whether it answers.
package apps

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// State is what an app is doing.
type State string

const (
	// Starting is an app whose command was started and has not yet answered.
	Starting State = "starting"
	// Live is an app that answered, and is shared.
	Live State = "live"
	// Stopped is an app somebody stopped, or that stopped answering because its workspace stopped.
	Stopped State = "stopped"
	// Failed is an app that did not answer, and the note says what its command wrote.
	Failed State = "failed"
)

// App is a web app shared from an agent's workspace.
type App struct {
	ID       string `json:"id"`
	Instance string `json:"instance"`
	// Owner is the person whose agent and workspace it is.
	Owner   string `json:"owner"`
	Command string `json:"command"`
	Port    int    `json:"port"`
	// Folder, for a page, is the folder of the agent's own that this server serves as it is, with no command run for it.
	Folder  string `json:"folder,omitempty"`
	Sandbox string `json:"sandbox,omitempty"`
	// Host is the name the gateway knows the app by, and Target where the gateway is
	// reached from this machine. Together they are how a request gets to it.
	Host    string `json:"host,omitempty"`
	Target  string `json:"target,omitempty"`
	State   State  `json:"state"`
	Note    string `json:"note,omitempty"`
	Started int64  `json:"startedAt"`
}

// Store is one organization's apps.
type Store struct {
	dir string
	mu  sync.Mutex
	now func() time.Time
}

// Open is the apps under dir, which is made when it is not there.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir, now: time.Now}, nil
}

// ErrNotFound is an app that is not there.
var ErrNotFound = errors.New("no such app")

func (s *Store) file(id string) string { return filepath.Join(s.dir, id+".json") }

// Valid is what an id looks like, so that nothing that is not one names a file.
func Valid(id string) bool {
	if len(id) != 10 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

// New makes an app that is starting.
func (s *Store) New(instance, owner, command string, port int) (App, error) {
	return s.add(App{Instance: instance, Owner: strings.ToLower(owner), Command: command, Port: port, State: Starting})
}

// NewPage makes a page: a folder of the agent's served as it is, live at once.
func (s *Store) NewPage(instance, owner, folder string) (App, error) {
	return s.add(App{Instance: instance, Owner: strings.ToLower(owner), Folder: folder, State: Live})
}

func (s *Store) add(app App) (App, error) {
	raw := make([]byte, 5)
	if _, err := rand.Read(raw); err != nil {
		return App{}, err
	}
	app.ID, app.Started = hex.EncodeToString(raw), s.now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	return app, s.write(app)
}

func (s *Store) write(app App) error {
	text, err := json.MarshalIndent(app, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(s.dir, ".write-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(append(text, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), s.file(app.ID))
}

func (s *Store) read(id string) (App, error) {
	if !Valid(id) {
		return App{}, ErrNotFound
	}
	text, err := os.ReadFile(s.file(id))
	if errors.Is(err, fs.ErrNotExist) {
		return App{}, ErrNotFound
	}
	if err != nil {
		return App{}, err
	}
	var app App
	if err := json.Unmarshal(text, &app); err != nil {
		return App{}, fmt.Errorf("%s does not hold an app: %w", s.file(id), err)
	}
	return app, nil
}

// Get is the app of that id.
func (s *Store) Get(id string) (App, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(id)
}

// Change applies a change to an app and keeps the result, under the store's lock so that two
// changes to one app do not lose one another.
func (s *Store) Change(id string, change func(*App)) (App, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	app, err := s.read(id)
	if err != nil {
		return App{}, err
	}
	change(&app)
	return app, s.write(app)
}

// All is every app, newest first.
func (s *Store) All() ([]App, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var all []App
	for _, entry := range entries {
		id, isJSON := strings.CutSuffix(entry.Name(), ".json")
		if !isJSON || !Valid(id) {
			continue
		}
		app, err := s.read(id)
		if err != nil {
			return nil, err
		}
		all = append(all, app)
	}
	sort.Slice(all, func(a, b int) bool {
		return all[a].Started > all[b].Started || (all[a].Started == all[b].Started && all[a].ID > all[b].ID)
	})
	return all, nil
}

// Of is the apps of one agent, newest first.
func (s *Store) Of(instance string) ([]App, error) {
	all, err := s.All()
	var mine []App
	for _, app := range all {
		if app.Instance == instance {
			mine = append(mine, app)
		}
	}
	return mine, err
}

// Delete takes an app away.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !Valid(id) {
		return ErrNotFound
	}
	if err := os.Remove(s.file(id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
