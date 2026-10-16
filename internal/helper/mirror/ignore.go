package mirror

import (
	"bufio"
	"os"
	"path"
	"strings"
)

// Defaults are what is never kept in step, whatever a folder's own file says: git's own directory (the agent's copy and the
// person's are two clones, and are not one folder), what a build or a package manager makes and what an operating system
// leaves about, and the sync's own scratch.
var Defaults = []string{".git/", "node_modules/", ".DS_Store", "Thumbs.db", "__pycache__/", "*.pyc", ".venv/", ".gusto-write-*", ".gusto-pull-*", ".gustoignore"}

// IgnoreFile is the name of the file in a folder's root that adds to Defaults, one pattern to a line.
const IgnoreFile = ".gustoignore"

// Ignore says which paths are left out. A pattern is, as in a gitignore:
//
//   - a name, `*.log` or `build`, which matches that name at any depth;
//   - a name with a slash at the end, `cache/`, which matches a folder of that name and so everything in it;
//   - a path with a slash in the middle, `docs/drafts`, which matches from the folder's root;
//   - a `#` line, which is a comment, and a blank line, which is nothing.
//
// A `!` to take a path back in is not understood: a pattern that starts with one is ignored, and said to be, so it is not
// taken to mean the opposite of what it says.
type Ignore struct {
	patterns []pattern
	// Unsupported are the lines that were not understood.
	Unsupported []string
}

type pattern struct {
	text     string
	foldersO bool
	anchored bool
}

// NewIgnore reads the patterns, Defaults first.
func NewIgnore(lines ...string) *Ignore {
	ignore := &Ignore{}
	for _, line := range append(append([]string{}, Defaults...), lines...) {
		ignore.add(line)
	}
	return ignore
}

func (i *Ignore) add(line string) {
	line = strings.TrimSpace(line)
	switch {
	case line == "" || strings.HasPrefix(line, "#"):
		return
	case strings.HasPrefix(line, "!"):
		i.Unsupported = append(i.Unsupported, line)
		return
	}
	p := pattern{text: strings.TrimPrefix(line, "/")}
	if strings.HasSuffix(p.text, "/") {
		p.foldersO = true
		p.text = strings.TrimSuffix(p.text, "/")
	}
	p.anchored = strings.HasPrefix(line, "/") || strings.Contains(p.text, "/")
	i.patterns = append(i.patterns, p)
}

// ReadIgnore is NewIgnore with the folder's own file added, when it has one.
func ReadIgnore(root string) (*Ignore, error) {
	file, err := os.Open(root + string(os.PathSeparator) + IgnoreFile)
	if os.IsNotExist(err) {
		return NewIgnore(), nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return NewIgnore(lines...), scanner.Err()
}

// Skips says whether a path is left out. isDir says it is a folder, which a folders-only pattern is about. A path is left out
// when it or any folder above it matches.
func (i *Ignore) Skips(name string, isDir bool) bool {
	parts := strings.Split(name, "/")
	for depth := range parts {
		here := strings.Join(parts[:depth+1], "/")
		last := depth == len(parts)-1
		for _, p := range i.patterns {
			if p.foldersO && last && !isDir {
				continue
			}
			target := parts[depth]
			if p.anchored {
				target = here
			}
			if ok, _ := path.Match(p.text, target); ok {
				return true
			}
		}
	}
	return false
}
