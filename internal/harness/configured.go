package harness

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/getlatentic/gusto/internal/job"
)

// Configured is a harness as a job runs it: the adapter, and the digest of the
// files it came from. Both are taken from one reading of those files, so the
// digest names exactly the adapter that runs, and a harness that rewrites its
// own files once it starts cannot change the digest already taken.
type Configured struct {
	Adapter Adapter
	Digest  string
}

// Open reads the configured harness a job names, looking in the job's own
// directory first, then GUSTO_HARNESSES, then the adapters built into this
// binary. In each place a harness is a directory, name/, or a bare adapter,
// name.json.
//
// A harness in the job travels with it, so a job naming a harness of your own
// runs on a runner that has never heard of it — in a container, on a fleet, on
// somebody else's machine. That is the same reason everything else about a run
// lives in the directory.
func Open(dir job.Dir, name string) (Configured, error) {
	tree, home, err := find(dir, name)
	if err != nil {
		return Configured{}, err
	}
	return configure(dir, tree, home)
}

func find(dir job.Dir, name string) (Tree, string, error) {
	for _, root := range roots(dir) {
		tree, home, found, err := lookIn(root, name)
		if err != nil || found {
			return tree, home, err
		}
	}
	text, err := builtIn.ReadFile("adapters/" + name + ".json")
	if err != nil {
		return nil, "", fmt.Errorf("no harness called %s", name)
	}
	return Tree{{Path: AdapterFile, Content: text}}, "", nil
}

// Load the adapter a job names, from the configured harness Open finds.
func Load(dir job.Dir, name string) (Adapter, error) {
	configured, err := Open(dir, name)
	return configured.Adapter, err
}

// Carrying reads the configured harness the job itself carries. Found is false
// when it carries none, which is so for a harness built into every gusto.
func Carrying(dir job.Dir, name string) (Configured, bool, error) {
	tree, home, found, err := lookIn(Carried(dir), name)
	if err != nil || !found {
		return Configured{}, found, err
	}
	configured, err := configure(dir, tree, home)
	return configured, true, err
}

// Carried is where the harnesses a job carries sit inside its directory.
func Carried(dir job.Dir) string {
	return filepath.Join(dir.Path(), "harnesses")
}

func roots(dir job.Dir) []string {
	var roots []string
	if dir != "" {
		roots = append(roots, Carried(dir))
	}
	if mine := os.Getenv("GUSTO_HARNESSES"); mine != "" {
		roots = append(roots, mine)
	}
	return roots
}

// lookIn finds a harness in one place. Home is the directory {harnessDir}
// names, which a bare adapter has none of.
func lookIn(root, name string) (Tree, string, bool, error) {
	folder := filepath.Join(root, name)
	inFolder, inFile := isDir(folder), isFile(folder+".json")
	switch {
	case inFolder && inFile:
		return nil, "", false, fmt.Errorf("the harness %s in %s %w; keep one", name, root, errBoth)
	case inFolder:
		return folderTree(folder)
	case inFile:
		tree, err := bare(folder + ".json")
		return tree, "", true, err
	}
	return nil, "", false, nil
}

func folderTree(folder string) (Tree, string, bool, error) {
	home, err := filepath.Abs(folder)
	if err != nil {
		return nil, "", true, err
	}
	tree, err := walk(home)
	if err == nil && tree.holdsSetup() {
		err = fmt.Errorf("the harness at %s has a %s folder, and that name is the job's setup's", home, SetupFolder)
	}
	return tree, home, true, err
}

// configure takes the adapter and the digest from one reading. The digest
// covers the setup the job carries as well, because skills and MCP servers
// shape the harness as much as its own files do: the same adapter given two
// setups is two configured harnesses.
func configure(dir job.Dir, tree Tree, home string) (Configured, error) {
	text, found := tree.adapter()
	if !found {
		return Configured{}, fmt.Errorf("the harness at %s has no %s", home, AdapterFile)
	}
	adapter, err := parse(text)
	if err != nil {
		return Configured{}, err
	}
	adapter.home = home
	given, err := setupOf(dir)
	if err != nil {
		return Configured{}, err
	}
	return Configured{Adapter: adapter, Digest: tree.with(given).Digest()}, nil
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Keep writes a configured harness into the job, so that whoever runs the job
// next has all of it: a bare adapter as name.json, a directory as name/. The
// one in GUSTO_HARNESSES is kept, because it is the one a runner here would
// use; a built-in one is already everywhere, so it is not copied.
func Keep(dir job.Dir, name string) error {
	mine := os.Getenv("GUSTO_HARNESSES")
	if mine != "" {
		tree, _, found, err := lookIn(mine, name)
		if err != nil || found {
			return carry(dir, name, tree, err)
		}
	}
	if _, err := builtIn.ReadFile("adapters/" + name + ".json"); err == nil || mine == "" {
		return nil
	}
	return fmt.Errorf("no harness called %s in %s: %w", name, mine, fs.ErrNotExist)
}

// carry replaces whatever the job held for the harness. A copy left in the
// other form would sit beside this one, and Open refuses a harness offered
// twice.
func carry(dir job.Dir, name string, tree Tree, err error) error {
	if err != nil {
		return err
	}
	folder := filepath.Join(Carried(dir), name)
	if err := os.RemoveAll(folder); err != nil {
		return err
	}
	if err := os.Remove(folder + ".json"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if tree.bareOnly() {
		only := tree[0]
		only.Path = name + ".json"
		return Tree{only}.writeTo(Carried(dir))
	}
	return tree.writeTo(folder)
}

// Names lists every harness this binary can run: the adapters it ships with,
// and any of your own in GUSTO_HARNESSES, bare or a directory. The pages offer
// what the server can actually run, rather than what it was compiled with.
func Names() []string {
	found := map[string]bool{}
	entries, _ := builtIn.ReadDir("adapters")
	for _, entry := range entries {
		found[strings.TrimSuffix(entry.Name(), ".json")] = true
	}
	if mine := os.Getenv("GUSTO_HARNESSES"); mine != "" {
		yours, _ := os.ReadDir(mine)
		for _, entry := range yours {
			if name, ok := offered(mine, entry); ok {
				found[name] = true
			}
		}
	}
	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// offered reads one entry of GUSTO_HARNESSES as a harness, if it is one.
func offered(root string, entry fs.DirEntry) (string, bool) {
	if entry.IsDir() {
		return entry.Name(), isFile(filepath.Join(root, entry.Name(), AdapterFile))
	}
	return strings.TrimSuffix(entry.Name(), ".json"), strings.HasSuffix(entry.Name(), ".json")
}
