package computer

import (
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

	"github.com/getlatentic/gusto/internal/quota"
)

// Registry is one organization's workspaces, one file each. It belongs to that
// organization: it makes workspaces for nobody else, so a record of another's
// cannot get in by a mistake in what calls it.
type Registry struct {
	dir   string
	org   string
	mu    sync.Mutex
	limit *quota.File
	now   func() time.Time
}

// Open is the organization's record under dir, which is made when it is not there.
func Open(dir, org string) (*Registry, error) {
	if org == "" {
		return nil, errors.New("a record of workspaces belongs to an organization")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Registry{dir: dir, org: org, now: time.Now}, nil
}

// UseQuota puts the number of workspaces under the organization's quota.
func (r *Registry) UseQuota(q *quota.File) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.limit = q
}

func (r *Registry) file(id string) string { return filepath.Join(r.dir, id+".json") }

// Ensure is the person's workspace, made when they have none. A person has one in
// an organization, so asking again finds it. Making a new one counts against the
// organization's quota, and a workspace that was deleted is made anew.
func (r *Registry) Ensure(user, host string) (computer Computer, made bool, err error) {
	return r.EnsureFrom(user, host, nil)
}

// EnsureFrom is Ensure that names a new computer's sandbox after a spare one, when spares has one to give. The spare is taken
// and the record written under one lock, so a person's second request finds the record and not a sandbox made for it twice.
func (r *Registry) EnsureFrom(user, host string, spares Spares) (computer Computer, made bool, err error) {
	user = strings.ToLower(strings.TrimSpace(user))
	if user == "" {
		return Computer{}, false, errors.New("a workspace is a person's, and no person was named")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id := ID(r.org, user)
	found, err := r.read(id)
	if err == nil && found.User != user {
		return Computer{}, false, fmt.Errorf("the record %s is of %s, and %s hashes to it too: no workspace is shared", id, found.User, user)
	}
	if err == nil && found.Live() {
		return found, false, nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Computer{}, false, err
	}
	if err := r.underLimit(); err != nil {
		return Computer{}, false, err
	}
	now := r.now().UTC().Truncate(time.Second)
	computer = Computer{ID: id, Org: r.org, User: user, Sandbox: Name(r.org, user), Host: host, State: Creating, Created: now}
	spare, spared := takeFrom(spares)
	if spared {
		computer.Sandbox = spare
	}
	if err := r.write(computer); err != nil {
		if spared {
			spares.Return(spare)
		}
		return Computer{}, false, err
	}
	return computer, true, nil
}

// underLimit is a refusal when one more computer would pass the quota.
func (r *Registry) underLimit() error {
	if r.limit == nil {
		return nil
	}
	q, err := r.limit.Current()
	if err != nil {
		return fmt.Errorf("no workspace is made, because %w", err)
	}
	all, err := r.all()
	if err != nil {
		return err
	}
	live := 0
	for _, each := range all {
		if each.Live() {
			live++
		}
	}
	return q.RefuseComputers(live)
}

// Of is a person's computer.
func (r *Registry) Of(user string) (Computer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	found, err := r.read(ID(r.org, strings.ToLower(strings.TrimSpace(user))))
	if errors.Is(err, fs.ErrNotExist) || (err == nil && !found.Live()) {
		return Computer{}, ErrNone
	}
	return found, err
}

// Change makes a change to a workspace's record and keeps it.
func (r *Registry) Change(id string, change func(*Computer)) (Computer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	computer, err := r.read(id)
	if err != nil {
		return Computer{}, err
	}
	change(&computer)
	return computer, r.write(computer)
}

// All is every workspace that is not deleted, the oldest first.
func (r *Registry) All() ([]Computer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	all, err := r.all()
	if err != nil {
		return nil, err
	}
	live := all[:0]
	for _, each := range all {
		if each.Live() {
			live = append(live, each)
		}
	}
	return live, nil
}

func (r *Registry) all() ([]Computer, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, err
	}
	var all []Computer
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		computer, err := r.read(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		all = append(all, computer)
	}
	sort.Slice(all, func(a, b int) bool { return all[a].Created.Before(all[b].Created) })
	return all, nil
}

func (r *Registry) read(id string) (Computer, error) {
	text, err := os.ReadFile(r.file(id))
	if err != nil {
		return Computer{}, err
	}
	var computer Computer
	if err := json.Unmarshal(text, &computer); err != nil {
		return Computer{}, fmt.Errorf("the record of a workspace in %s is not valid: %w", r.file(id), err)
	}
	if computer.Org != r.org {
		return Computer{}, fmt.Errorf("%s is a workspace of the organization %q, and this record is of %q", r.file(id), computer.Org, r.org)
	}
	return computer, nil
}

func (r *Registry) write(computer Computer) error {
	text, err := json.MarshalIndent(computer, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(r.dir, ".write-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(append(text, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), r.file(computer.ID))
}

// retireFile is the mark that asks for a person's computer to be removed. It is a file of
// its own and not a change to the record, because the front door makes it and the
// organization's server, which reaches the gateway, acts on it, and two processes
// changing one record would lose a change.
func (r *Registry) retireFile(id string) string { return filepath.Join(r.dir, id+".retire") }

// Retire asks for a person's workspace to be removed, and does nothing when they have none.
func (r *Registry) Retire(user string) error {
	id := ID(r.org, strings.ToLower(strings.TrimSpace(user)))
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, err := r.read(id); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	file, err := os.OpenFile(r.retireFile(id), os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	return file.Close()
}

// RetireAll asks for every workspace of the organization to be removed, which is what ending the organization does.
func (r *Registry) RetireAll() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	all, err := r.all()
	if err != nil {
		return err
	}
	for _, each := range all {
		if !each.Live() {
			continue
		}
		file, err := os.OpenFile(r.retireFile(each.ID), os.O_WRONLY|os.O_CREATE, 0o600)
		if err != nil {
			return err
		}
		file.Close()
	}
	return nil
}

// Retiring is the workspaces that were asked to be removed and have not been.
func (r *Registry) Retiring() ([]Computer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	all, err := r.all()
	if err != nil {
		return nil, err
	}
	var retiring []Computer
	for _, each := range all {
		if _, err := os.Stat(r.retireFile(each.ID)); err == nil && each.Live() {
			retiring = append(retiring, each)
		}
	}
	return retiring, nil
}

// Retired records that a workspace was removed, and clears the mark.
func (r *Registry) Retired(id string) error {
	if _, err := r.Change(id, func(c *Computer) { c.State, c.Note = Deleted, "removed" }); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := os.Remove(r.retireFile(id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
