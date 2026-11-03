package compare

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/getlatentic/gusto/internal/atomicfile"
	"github.com/getlatentic/gusto/pkg/job"
)

// Board keeps the comparisons started from the pages. Their runs are tasks in
// the server's own queue, named by id, so each is an ordinary task everywhere
// else: in Activity, in Spending, and in its own signed record.
type Board struct {
	root  string
	guard sync.Mutex
}

// Lineup is one task given to several engines on the same commit, with the
// same model and the same check.
type Lineup struct {
	ID       string   `json:"id"`
	Brief    string   `json:"brief"`
	Template string   `json:"template"`
	Model    string   `json:"model"`
	Commit   string   `json:"commit"`
	Check    []string `json:"check"`
	At       int64    `json:"at"`
	// Who is the person who started it, and the only one besides an admin who
	// may read it.
	Who  string  `json:"who,omitempty"`
	Runs []Entry `json:"runs"`
}

// Entry is one engine's run in a lineup.
type Entry struct {
	Harness string `json:"harness"`
	Job     string `json:"job"`
}

func OpenBoard(root string) (*Board, error) {
	if err := os.MkdirAll(filepath.Join(root, "samples"), 0o755); err != nil {
		return nil, err
	}
	return &Board{root: root}, nil
}

func (b *Board) file(id string) string { return filepath.Join(b.root, id+".json") }

// Reserve keeps a new lineup under an id no lineup has, the one proposed or that
// numbered when it is taken, and says which. The lineup is whole in its file
// from the moment the id is taken, so one whose runs are still being queued is
// listed with the runs queued so far, and never as a file nothing can read.
func (b *Board) Reserve(proposed string, lineup Lineup) (string, error) {
	for n := 1; ; n++ {
		lineup.ID = proposed
		if n > 1 {
			lineup.ID = fmt.Sprintf("%s-%d", proposed, n)
		}
		taken, err := b.claim(lineup)
		if err != nil || !taken {
			return lineup.ID, err
		}
	}
}

// claim writes the lineup beside its file and links it into place, which only
// one caller can do for a name. It says whether the name was taken already.
func (b *Board) claim(lineup Lineup) (taken bool, err error) {
	if !job.Named(lineup.ID) {
		return false, fmt.Errorf("%q cannot name a comparison", lineup.ID)
	}
	text, err := json.MarshalIndent(lineup, "", "  ")
	if err != nil {
		return false, err
	}
	temp, err := os.CreateTemp(b.root, ".lineup-")
	if err != nil {
		return false, err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(append(text, '\n')); err != nil {
		temp.Close()
		return false, err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return false, err
	}
	if err := temp.Close(); err != nil {
		return false, err
	}
	err = os.Link(temp.Name(), b.file(lineup.ID))
	if errors.Is(err, os.ErrExist) {
		return true, nil
	}
	return false, err
}

// Save keeps a lineup under its id.
func (b *Board) Save(lineup Lineup) error {
	if !job.Named(lineup.ID) {
		return fmt.Errorf("%q cannot name a comparison", lineup.ID)
	}
	text, err := json.MarshalIndent(lineup, "", "  ")
	if err != nil {
		return err
	}
	b.guard.Lock()
	defer b.guard.Unlock()
	return atomicfile.WriteFile(b.file(lineup.ID), append(text, '\n'), 0o644)
}

func (b *Board) Get(id string) (Lineup, bool) {
	if !job.Named(id) {
		return Lineup{}, false
	}
	text, err := os.ReadFile(b.file(id))
	if err != nil {
		return Lineup{}, false
	}
	var lineup Lineup
	if json.Unmarshal(text, &lineup) != nil {
		return Lineup{}, false
	}
	return lineup, true
}

// List is every lineup, newest first.
func (b *Board) List() []Lineup {
	paths, _ := filepath.Glob(filepath.Join(b.root, "*.json"))
	var all []Lineup
	for _, path := range paths {
		if lineup, found := b.Get(strings.TrimSuffix(filepath.Base(path), ".json")); found {
			all = append(all, lineup)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].At > all[j].At })
	return all
}

// Sample is a repository holding a template's sample files, for every run of
// a lineup to clone, and the commit they start from. The repository follows
// the template: files that changed since it was last used are committed.
func (b *Board) Sample(name string, files map[string]string) (string, string, error) {
	if !job.Named(name) {
		return "", "", fmt.Errorf("%q cannot name a sample", name)
	}
	b.guard.Lock()
	defer b.guard.Unlock()
	repo := filepath.Join(b.root, "samples", name)
	if _, err := os.Stat(filepath.Join(repo, ".git")); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(repo, 0o755); err != nil {
			return "", "", err
		}
		if _, err := git(repo, "init", "--quiet"); err != nil {
			return "", "", err
		}
	}
	for name, text := range files {
		clean := filepath.Clean(name)
		if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
			return "", "", fmt.Errorf("%q is not a file in the sample", name)
		}
		path := filepath.Join(repo, clean)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", "", err
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return "", "", err
		}
	}
	if _, err := git(repo, "add", "-A"); err != nil {
		return "", "", err
	}
	if changed, _ := git(repo, "status", "--porcelain"); changed != "" {
		if _, err := git(repo, "-c", "user.name=gusto", "-c", "user.email=gusto@localhost",
			"commit", "--quiet", "-m", "the template's sample files"); err != nil {
			return "", "", err
		}
	}
	commit, err := git(repo, "rev-parse", "HEAD")
	return repo, commit, err
}

func git(at string, args ...string) (string, error) {
	command := exec.Command("git", args...)
	command.Dir = at
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, output)
	}
	return strings.TrimSpace(string(output)), nil
}
