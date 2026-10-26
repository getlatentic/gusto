package roomwork

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Conflict is a file two turns changed. The folder keeps the version that arrived first, and the later turn's is saved beside it.
type Conflict struct {
	// Path is the file in the room's folder, and Side where the turn's own version was saved, empty when the turn deleted the
	// file and so has no version to save.
	Path string
	Side string
	// Agent is the turn's agent. Other is the agent whose version is in the folder, empty when that was not an agent's merge
	// that this turn can name, such as a person's upload.
	Agent string
	Other string
	// Deleted says the turn deleted a file that something else changed, which stays.
	Deleted bool
}

// Outcome is what a merge did.
type Outcome struct {
	// Changed is how many files the turn's merge added, changed or removed in the folder.
	Changed   int
	Conflicts []Conflict
}

// change is one file the turn's copy differs on from how it was made.
type change struct {
	path string
	// gone is a file the turn deleted.
	gone bool
}

// action is one thing the merge does to the folder: put the turn's file at dest, or remove dest when source is empty.
type action struct {
	dest   string
	source string
}

// changes is every file the turn added, changed or deleted, in path order.
func (t *Turn) changes(now tree) []change {
	var found []change
	for name, was := range t.copied {
		switch is, there := now[name]; {
		case !there:
			found = append(found, change{path: name, gone: true})
		case !unchanged(was, is):
			found = append(found, change{path: name})
		}
	}
	for name := range now {
		if _, had := t.copied[name]; !had {
			found = append(found, change{path: name})
		}
	}
	sort.Slice(found, func(a, b int) bool { return found[a].path < found[b].path })
	return found
}

// plan decides, for each file the turn changed, whether it goes into the folder as it is, is already there, or conflicts with
// what another turn or a person put there since the copy was made. It reads the folder and writes nothing.
func (t *Turn) plan(changes []change, now tree) ([]action, []Conflict) {
	var actions []action
	var conflicts []Conflict
	taken := map[string]bool{}
	for _, each := range changes {
		there, has := lookup(t.room.Folder, each.path)
		was, had := t.shared[each.path]
		moved := has != had || (has && !unchanged(there, was))
		mine := filepath.Join(t.dest, filepath.FromSlash(each.path))
		switch {
		case each.gone && !has:
		case each.gone && !moved:
			actions = append(actions, action{dest: each.path})
		case each.gone:
			conflicts = append(conflicts, t.conflict(each.path, "", true))
		case has && sameFile(mine, t.inFolder(each.path), now[each.path], there) && (moved || now[each.path].mode == there.mode):
		case moved && t.rewroteAsItWas(each.path, mine, now[each.path]):
		case !moved && !t.blocked(each.path):
			actions = append(actions, action{dest: each.path, source: mine})
		default:
			side := t.sideFor(each.path, taken)
			actions = append(actions, action{dest: side, source: mine})
			conflicts = append(conflicts, t.conflict(each.path, side, false))
		}
	}
	return actions, conflicts
}

// rewroteAsItWas says whether the turn's file holds what the folder's file held when the turn began, which a merge since has
// kept: the turn wrote the file again without changing it, so it has nothing to add to what has happened to the file since.
func (t *Turn) rewroteAsItWas(name, mine string, now entry) bool {
	kept, known := t.hub.began(t.room, t, name)
	if !known {
		return false
	}
	was, ok := lookupFile(kept)
	return ok && now.mode == t.shared[name].mode && sameFile(mine, kept, now, was)
}

func (t *Turn) inFolder(name string) string {
	return filepath.Join(t.room.Folder, filepath.FromSlash(name))
}

func (t *Turn) conflict(name, side string, deleted bool) Conflict {
	other, _ := t.hub.writer(t.room, t, name)
	return Conflict{Path: name, Side: side, Agent: t.agent, Other: other, Deleted: deleted}
}

// blocked says whether a file cannot go at its path in the folder: a folder on the way is a file or a link, so putting it there
// would write through the link or fail, or the path is a folder.
func (t *Turn) blocked(name string) bool {
	if t.parentBlocked(name) {
		return true
	}
	info, err := os.Lstat(t.inFolder(name))
	return err == nil && info.IsDir()
}

func (t *Turn) parentBlocked(name string) bool {
	at := t.room.Folder
	parts := strings.Split(name, "/")
	for _, part := range parts[:len(parts)-1] {
		at = filepath.Join(at, part)
		info, err := os.Lstat(at)
		if os.IsNotExist(err) {
			return false
		}
		if err != nil || !info.IsDir() {
			return true
		}
	}
	return false
}

// sideFor is where the turn's version of a conflicting file is saved: beside the file, with the agent's name before the
// extension, or at the top of the folder when the file's own folder is in the way. Names already taken are not reused.
func (t *Turn) sideFor(name string, taken map[string]bool) string {
	if t.parentBlocked(name) {
		name = strings.ReplaceAll(name, "/", "-")
	}
	for n := 1; ; n++ {
		side := sideName(name, t.agent, n)
		if _, exists := lookup(t.room.Folder, side); !exists && !taken[side] && !t.blocked(side) {
			taken[side] = true
			return side
		}
	}
}

// sameFile says whether two files hold the same thing: the same bytes, or the same link target.
func sameFile(mine, theirs string, a, b entry) bool {
	if a.kind != b.kind {
		return false
	}
	if a.kind == link {
		x, err1 := os.Readlink(mine)
		y, err2 := os.Readlink(theirs)
		return err1 == nil && err2 == nil && x == y
	}
	if a.size != b.size {
		return false
	}
	return sameBytes(mine, theirs)
}

func sameBytes(a, b string) bool {
	left, err := os.Open(a)
	if err != nil {
		return false
	}
	defer left.Close()
	right, err := os.Open(b)
	if err != nil {
		return false
	}
	defer right.Close()
	const block = 64 << 10
	one, two := make([]byte, block), make([]byte, block)
	for {
		n, err := io.ReadFull(left, one)
		m, _ := io.ReadFull(right, two)
		if n != m || !bytes.Equal(one[:n], two[:m]) {
			return false
		}
		if err != nil {
			return true
		}
	}
}
