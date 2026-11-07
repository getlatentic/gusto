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
	"github.com/getlatentic/gusto/internal/workspace"
)

// Registry is one organization's computers, one file each. It belongs to that
// organization: it makes computers for nobody else, so a record of another's
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
		return nil, errors.New("a record of computers belongs to an organization")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &Registry{dir: dir, org: org, now: time.Now}, nil
}

// UseQuota puts the number of computers under the organization's quota.
func (r *Registry) UseQuota(q *quota.File) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.limit = q
}

func (r *Registry) file(id string) string { return filepath.Join(r.dir, id+".json") }

// Ensure is the person's computer in a workspace, made when they have none. A person
// has one in each workspace, so asking again finds it. Making a new one counts against
// the organization's quota, and a computer that was deleted is made anew.
func (r *Registry) Ensure(user, space, host string) (computer Computer, made bool, err error) {
	return r.EnsureFrom(user, space, host, nil)
}

// EnsureFrom is Ensure that names a new computer's sandbox after a spare one, when spares has one to give. The spare is taken
// and the record written under one lock, so a person's second request finds the record and not a sandbox made for it twice.
func (r *Registry) EnsureFrom(user, space, host string, spares Spares) (computer Computer, made bool, err error) {
	user = strings.ToLower(strings.TrimSpace(user))
	if user == "" {
		return Computer{}, false, errors.New("a computer is a person's, and no person was named")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id := ID(r.org, space, user)
	found, err := r.read(id)
	if err == nil && (found.User != user || !workspace.Same(found.Workspace, space)) {
		return Computer{}, false, fmt.Errorf("the record %s is of %s in %s, and %s in %s hashes to it too: no computer is shared",
			id, found.User, workspace.Of(found.Workspace), user, workspace.Of(space))
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
	// A mark asking for a removed computer to be removed is not for this one, which is made now: a gusto cloud server of an earlier
	// release, or a crash between Retired's two steps, can leave one.
	if err := os.Remove(r.retireFile(id)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Computer{}, false, err
	}
	now := r.now().UTC().Truncate(time.Second)
	computer = Computer{ID: id, Org: r.org, User: user, Workspace: stored(space), Sandbox: Name(r.org, space, user), Host: host, State: Creating, Created: now}
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
		return fmt.Errorf("no computer is made, because %w", err)
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

// stored is how a computer's record names its workspace: not at all for the first, as the records made before workspaces do.
func stored(space string) string {
	if space = workspace.Of(space); space != workspace.First {
		return space
	}
	return ""
}

// Of is a person's computer in a workspace.
func (r *Registry) Of(user, space string) (Computer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	found, err := r.read(ID(r.org, space, strings.ToLower(strings.TrimSpace(user))))
	if errors.Is(err, fs.ErrNotExist) || (err == nil && !found.Live()) {
		return Computer{}, ErrNone
	}
	return found, err
}

// Change makes a change to a computer's record and keeps it.
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

// All is every computer that is not deleted, the oldest first.
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
		return Computer{}, fmt.Errorf("the record of a computer in %s is not valid: %w", r.file(id), err)
	}
	if computer.Org != r.org {
		return Computer{}, fmt.Errorf("%s is a computer of the organization %q, and this record is of %q", r.file(id), computer.Org, r.org)
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
// its own and not a change to the record, because gusto cloud makes it and the
// organization's server, which reaches the gateway, acts on it, and two processes
// changing one record would lose a change.
func (r *Registry) retireFile(id string) string { return filepath.Join(r.dir, id+".retire") }

// RetireIn asks for a person's computer in a workspace to be removed, and does nothing when they have none there, or theirs
// was removed already: a mark on a removed one would remove the next one made for them.
func (r *Registry) RetireIn(user, space string) error {
	id := ID(r.org, space, strings.ToLower(strings.TrimSpace(user)))
	r.mu.Lock()
	defer r.mu.Unlock()
	held, err := r.read(id)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && !held.Live()) {
		return nil
	}
	if err != nil {
		return err
	}
	return r.mark(id)
}

// Retire asks for each of a person's computers to be removed, in every workspace, which is what leaving the organization does.
func (r *Registry) Retire(user string) error {
	user = strings.ToLower(strings.TrimSpace(user))
	r.mu.Lock()
	defer r.mu.Unlock()
	all, err := r.all()
	if err != nil {
		return err
	}
	for _, each := range all {
		if each.User == user && each.Live() {
			if err := r.mark(each.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// mark leaves the file that asks for a computer to be removed.
func (r *Registry) mark(id string) error {
	file, err := os.OpenFile(r.retireFile(id), os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	return file.Close()
}

// RetireAll asks for every computer of the organization to be removed, which is what ending the organization does.
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
		if err := r.mark(each.ID); err != nil {
			return err
		}
	}
	return nil
}

// Retiring is the computers that were asked to be removed and have not been.
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

// Retired records that a computer was removed, and clears the mark.
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
