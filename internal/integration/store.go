package integration

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/getlatentic/gusto/internal/atomicfile"
)

// Source says where a version came from, so that the same version can be found again and the person who turns it on can see it.
type Source struct {
	// Kind is "git" for an address and a commit, "upload" for files that were sent, "folder" for a folder on the server.
	Kind    string `json:"kind"`
	Address string `json:"address,omitempty"`
	// Ref is what was asked for and Commit is what it was then: a version is pinned to a commit, and not to a branch that moves.
	Ref    string `json:"ref,omitempty"`
	Commit string `json:"commit,omitempty"`
}

// Version is one imported version of an integration. Its files are kept, whole, under the store.
type Version struct {
	ID       string `json:"id"`
	Label    string `json:"label,omitempty"`
	Digest   string `json:"digest"`
	Source   Source `json:"source"`
	By       string `json:"by"`
	Imported int64  `json:"importedAt"`
	// Plugin is what was read when it was imported.
	Plugin Plugin `json:"plugin"`
	// Skills is the fingerprint of each skill's folder, which is how two versions are told apart skill by skill.
	Skills map[string]string `json:"skills,omitempty"`
}

// Event is something done to an integration.
type Event struct {
	At      int64  `json:"at"`
	By      string `json:"by"`
	Action  string `json:"action"`
	Version string `json:"version,omitempty"`
}

// Actions an administrator takes.
const (
	Imported = "imported"
	Enabled  = "enabled"
	Disabled = "disabled"
	Restored = "restored"
	// ProgramsAllowed and ProgramsRefused are an administrator letting the programs of a version run, and taking it back.
	ProgramsAllowed = "programs-allowed"
	ProgramsRefused = "programs-refused"
	// Scoped is an administrator choosing who an integration is on for.
	Scoped = "scoped"
)

// Integration is one plugin by name, with every version imported and which one is on. An integration that was imported is off
// until an administrator turns a version on: finding something does not offer it.
type Integration struct {
	Name     string    `json:"name"`
	Versions []Version `json:"versions"`
	// On is the version people get, or empty when it is off.
	On string `json:"on,omitempty"`
	// Before is the version that was on before the one that is, which Restore turns on again.
	Before string `json:"before,omitempty"`
	// Programs is the version whose programs an administrator allowed to run, which is only that version: a new version is a new
	// program, and has to be allowed again.
	Programs string `json:"programs,omitempty"`
	// For is who the integration is on for, as patterns of the address a person signed in with ("ada@acme.example", "*@acme.example").
	// None is everyone. A person is who the sign-in says: a team is a label a person chooses for a task, and nothing proves they belong
	// to it, so it cannot decide who may have something.
	For     []string `json:"for,omitempty"`
	History []Event  `json:"history,omitempty"`
}

// Store is every integration an organisation has imported, one folder each, with a file that says what is on.
type Store struct {
	root string
	mu   sync.Mutex
	Now  func() time.Time
}

// Open is the store in a folder, which is made when it is not there.
func Open(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &Store{root: root, Now: time.Now}, nil
}

var (
	ErrNotFound  = errors.New("there is no such integration or version")
	ErrNoVersion = errors.New("that version is not imported")
)

func (s *Store) file(name string) string { return filepath.Join(s.root, name, "integration.json") }

func (s *Store) folder(name, version string) string {
	return filepath.Join(s.root, name, "versions", version, "plugin")
}

func (s *Store) read(name string) (*Integration, error) {
	if !validName(name) {
		return nil, ErrNotFound
	}
	text, err := os.ReadFile(s.file(name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("the integration %s cannot be read: %w", name, err)
	}
	var held Integration
	if err := json.Unmarshal(text, &held); err != nil {
		return nil, fmt.Errorf("the integration %s is not valid: %w", name, err)
	}
	return &held, nil
}

func (s *Store) write(held *Integration) error {
	return atomicfile.Write(s.file(held.Name), held, 0o600)
}

// Import takes a plugin folder in as a new version. The plugin is read first, and one that cannot be taken is not kept. A version
// that is the same as one already imported is that one. A new version of an integration that is on does not replace it: the one
// that is on stays on until an administrator turns the new one on.
func (s *Store) Import(dir string, source Source, by string) (*Integration, Version, error) {
	plugin, err := Read(dir)
	if err != nil {
		return nil, Version{}, err
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, Version{}, err
	}
	sum, err := digest(root, root)
	if err != nil {
		return nil, Version{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	held, err := s.read(plugin.Name)
	if errors.Is(err, ErrNotFound) {
		held, err = &Integration{Name: plugin.Name}, nil
	}
	if err != nil {
		return nil, Version{}, err
	}
	id := sum[:12]
	for _, version := range held.Versions {
		if version.ID == id {
			return held, version, nil
		}
	}
	version := Version{ID: id, Label: plugin.Version, Digest: sum, Source: source, By: by, Imported: s.Now().Unix(), Plugin: *plugin, Skills: map[string]string{}}
	for _, skill := range plugin.Skills {
		if version.Skills[skill.Name], err = digest(root, filepath.Join(root, filepath.FromSlash(skill.Dir))); err != nil {
			return nil, Version{}, err
		}
	}
	target := s.folder(plugin.Name, id)
	if err := os.RemoveAll(filepath.Dir(target)); err != nil {
		return nil, Version{}, err
	}
	if err := copyTree(root, root, target); err != nil {
		os.RemoveAll(filepath.Dir(target))
		return nil, Version{}, err
	}
	held.Versions = append(held.Versions, version)
	held.History = append(held.History, Event{At: version.Imported, By: by, Action: Imported, Version: id})
	if err := s.write(held); err != nil {
		os.RemoveAll(filepath.Dir(target))
		return nil, Version{}, err
	}
	return held, version, nil
}

// Get is one integration.
func (s *Store) Get(name string) (*Integration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(name)
}

// List is every integration, by name. One that cannot be read is an error and not left out of the list, since a list that was
// missing an integration that is on would look right.
func (s *Store) List() ([]*Integration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	var all []*Integration
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		held, err := s.read(entry.Name())
		if err != nil {
			return nil, err
		}
		all = append(all, held)
	}
	sort.Slice(all, func(a, b int) bool { return all[a].Name < all[b].Name })
	return all, nil
}

func (held *Integration) version(id string) (Version, bool) {
	for _, version := range held.Versions {
		if version.ID == id {
			return version, true
		}
	}
	return Version{}, false
}

// Version is one version of an integration.
func (held *Integration) Version(id string) (Version, bool) { return held.version(id) }

// Enable turns a version on, for everyone, and the one that was on is the one Restore returns to.
func (s *Store) Enable(name, id, by string) (*Integration, error) {
	return s.change(name, by, func(held *Integration) (string, string, error) {
		if _, found := held.version(id); !found {
			return "", "", ErrNoVersion
		}
		if held.On != id {
			held.Before = held.On
		}
		held.On = id
		return Enabled, id, nil
	})
}

// Disable turns an integration off. Its versions are kept.
func (s *Store) Disable(name, by string) (*Integration, error) {
	return s.change(name, by, func(held *Integration) (string, string, error) {
		if held.On != "" {
			held.Before = held.On
		}
		held.On = ""
		return Disabled, "", nil
	})
}

// Restore turns on the version that was on before the one that is, or that was on before the integration was turned off.
func (s *Store) Restore(name, by string) (*Integration, error) {
	return s.change(name, by, func(held *Integration) (string, string, error) {
		if _, found := held.version(held.Before); !found {
			return "", "", errors.New("there is no earlier version to put back")
		}
		held.On, held.Before = held.Before, held.On
		return Restored, held.On, nil
	})
}

func (s *Store) change(name, by string, do func(*Integration) (action, version string, err error)) (*Integration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	held, err := s.read(name)
	if err != nil {
		return nil, err
	}
	action, version, err := do(held)
	if err != nil {
		return nil, err
	}
	held.History = append(held.History, Event{At: s.Now().Unix(), By: by, Action: action, Version: version})
	return held, s.write(held)
}

// AllowPrograms lets the programs of a version run, which is to say the servers it brings that are programs and not addresses. They run
// in a box, and an administrator decides they may: a program is code that somebody else wrote.
func (s *Store) AllowPrograms(name, id, by string) (*Integration, error) {
	return s.change(name, by, func(held *Integration) (string, string, error) {
		if _, found := held.version(id); !found {
			return "", "", ErrNoVersion
		}
		held.Programs = id
		return ProgramsAllowed, id, nil
	})
}

// RefusePrograms takes the permission back.
func (s *Store) RefusePrograms(name, by string) (*Integration, error) {
	return s.change(name, by, func(held *Integration) (string, string, error) {
		version := held.Programs
		held.Programs = ""
		return ProgramsRefused, version, nil
	})
}

// SetFor chooses who the integration is on for. No patterns is everyone.
func (s *Store) SetFor(name string, patterns []string, by string) (*Integration, error) {
	cleaned := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		if _, err := path.Match(pattern, ""); err != nil {
			return nil, fmt.Errorf("%q is not a pattern of addresses: %w", pattern, err)
		}
		cleaned = append(cleaned, pattern)
	}
	return s.change(name, by, func(held *Integration) (string, string, error) {
		held.For = cleaned
		return Scoped, strings.Join(cleaned, " "), nil
	})
}

// For says whether the integration is on for a person. Nobody named is everyone; somebody not known by name, such as a shared
// sign-in or a program's token, is not on any list.
func (held *Integration) OnFor(person string) bool { return onFor(held.For, person) }

// OnFor says whether the integration that is on is on for a person.
func (a Active) OnFor(person string) bool { return onFor(a.For, person) }

func onFor(patterns []string, person string) bool {
	if len(patterns) == 0 {
		return true
	}
	person = strings.ToLower(strings.TrimSpace(person))
	for _, pattern := range patterns {
		if ok, err := path.Match(pattern, person); err == nil && ok && person != "" {
			return true
		}
	}
	return false
}

// ProgramsRun says whether the programs of the version that is on may run.
func (held *Integration) ProgramsRun() bool { return held.On != "" && held.Programs == held.On }

// Active is an integration that is on, with the folder of the version people get.
type Active struct {
	Name    string
	Version Version
	// Dir is the plugin's own folder. It is read-only to everything that uses it.
	Dir string
	// Programs says an administrator allowed this version's programs to run.
	Programs bool
	// For is who it is on for: nobody named is everyone.
	For []string
}

// Active is every integration that is on.
func (s *Store) Active() ([]Active, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	var on []Active
	for _, held := range all {
		if version, found := held.version(held.On); found {
			on = append(on, Active{Name: held.Name, Version: version, Dir: s.folder(held.Name, version.ID), Programs: held.ProgramsRun(), For: held.For})
		}
	}
	return on, nil
}

// Changes is what differs between two versions, for the person who is deciding whether to turn the newer one on.
type Changes struct {
	SkillsAdded    []string `json:"skillsAdded,omitempty"`
	SkillsRemoved  []string `json:"skillsRemoved,omitempty"`
	SkillsChanged  []string `json:"skillsChanged,omitempty"`
	ServersAdded   []string `json:"serversAdded,omitempty"`
	ServersRemoved []string `json:"serversRemoved,omitempty"`
	// ServersChanged is a server whose address, command or anything else about it differs: the part most worth reading.
	ServersChanged []string `json:"serversChanged,omitempty"`
	// Manifest names the descriptive fields that differ: the version, the description, the author, the licence, the repository.
	Manifest []string `json:"manifest,omitempty"`
}

// None says whether the two versions are the same in every way this reports.
func (c Changes) None() bool {
	return len(c.SkillsAdded)+len(c.SkillsRemoved)+len(c.SkillsChanged)+len(c.ServersAdded)+len(c.ServersRemoved)+len(c.ServersChanged)+len(c.Manifest) == 0
}

// Compare says what differs from one version to another.
func (held *Integration) Compare(from, to string) (Changes, error) {
	before, ok := held.version(from)
	after, found := held.version(to)
	if !ok || !found {
		return Changes{}, ErrNoVersion
	}
	var c Changes
	for name, sum := range after.Skills {
		switch old, had := before.Skills[name]; {
		case !had:
			c.SkillsAdded = append(c.SkillsAdded, name)
		case old != sum:
			c.SkillsChanged = append(c.SkillsChanged, name)
		}
	}
	for name := range before.Skills {
		if _, has := after.Skills[name]; !has {
			c.SkillsRemoved = append(c.SkillsRemoved, name)
		}
	}
	oldServers, newServers := byName(before.Plugin.Servers), byName(after.Plugin.Servers)
	for name, server := range newServers {
		switch old, had := oldServers[name]; {
		case !had:
			c.ServersAdded = append(c.ServersAdded, name)
		case !sameJSON(old, server):
			c.ServersChanged = append(c.ServersChanged, name)
		}
	}
	for name := range oldServers {
		if _, has := newServers[name]; !has {
			c.ServersRemoved = append(c.ServersRemoved, name)
		}
	}
	c.Manifest = manifestDifferences(before.Plugin, after.Plugin)
	for _, list := range [][]string{c.SkillsAdded, c.SkillsRemoved, c.SkillsChanged, c.ServersAdded, c.ServersRemoved, c.ServersChanged} {
		sort.Strings(list)
	}
	return c, nil
}

func byName(servers []Server) map[string]Server {
	named := make(map[string]Server, len(servers))
	for _, server := range servers {
		named[server.Name] = server
	}
	return named
}

func sameJSON(a, b any) bool {
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return string(left) == string(right)
}

func manifestDifferences(a, b Plugin) []string {
	var differ []string
	for _, field := range []struct {
		name        string
		left, right any
	}{
		{"version", a.Version, b.Version}, {"description", a.Description, b.Description}, {"author", a.Author, b.Author},
		{"homepage", a.Homepage, b.Homepage}, {"repository", a.Repository, b.Repository}, {"license", a.License, b.License},
		{"keywords", a.Keywords, b.Keywords},
	} {
		if !sameJSON(field.left, field.right) {
			differ = append(differ, field.name)
		}
	}
	return differ
}
