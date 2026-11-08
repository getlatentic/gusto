package roomwork

import (
	"os"
	"path/filepath"
	"strconv"
)

// A turn judges a file against how it was when the turn began. The folder no longer has that version once another merge has
// replaced it, so a merge keeps what it replaces for as long as a turn that began before it is still working. That is how a
// turn that only rewrote a file the same, a formatter's no-op, is not mistaken for a turn that changed it.

// history is what the merges into a room did since its oldest working turn began.
type history struct {
	wrote  map[string]landed
	priors map[string][]prior
}

// landed is who last put a file in the folder by merging, and in which merge.
type landed struct {
	agent string
	seq   uint64
}

// prior is what a file held before a merge replaced it: kept in file, or nothing when the merge made it.
type prior struct {
	seq  uint64
	file string
}

func newHistory() history {
	return history{wrote: map[string]landed{}, priors: map[string][]prior{}}
}

func priorDir(in Room) string { return filepath.Join(in.Meta, "prior") }

// trim forgets what no turn that began at or after seq could need, and says which kept files that frees.
func (h history) trim(seq uint64) (spent []string) {
	for path, wrote := range h.wrote {
		if wrote.seq <= seq {
			delete(h.wrote, path)
		}
	}
	for path, list := range h.priors {
		kept := list[:0]
		for _, each := range list {
			if each.seq > seq {
				kept = append(kept, each)
			} else if each.file != "" {
				spent = append(spent, each.file)
			}
		}
		if len(kept) == 0 {
			delete(h.priors, path)
		} else {
			h.priors[path] = kept
		}
	}
	return spent
}

// writer is the agent whose merge put the file in the folder after the turn began, and whether any did.
func (h *Hub) writer(in Room, turn *Turn, path string) (string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if state := h.rooms[in.Folder]; state != nil {
		if wrote, found := state.history.wrote[path]; found && wrote.seq > turn.begin {
			return wrote.agent, true
		}
	}
	return "", false
}

// began is the file as it was when the turn began, if a merge since has kept it: its path, and whether the file existed. It
// answers false when nothing has replaced the file since, or the version it had is not known.
func (h *Hub) began(in Room, turn *Turn, path string) (file string, known bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if state := h.rooms[in.Folder]; state != nil {
		for _, each := range state.history.priors[path] {
			if each.seq > turn.begin {
				return each.file, each.file != ""
			}
		}
	}
	return "", false
}

// merged records that a turn's merge put these files in the folder, and keeps what it replaced when another turn is working that
// began before it. steps are the commit's, one for each of dests.
func (h *Hub) merged(in Room, turn *Turn, dests []string, steps []step) {
	h.mu.Lock()
	state := h.rooms[in.Folder]
	if state == nil {
		h.mu.Unlock()
		return
	}
	state.seq++
	seq := state.seq
	keep := len(state.live) > 1
	h.mu.Unlock()

	kept := map[string]string{}
	if keep {
		for i, each := range steps {
			kept[dests[i]] = retain(in, seq, i, each)
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, dest := range dests {
		state.history.wrote[dest] = landed{turn.agent, seq}
		if file, ok := kept[dest]; ok {
			state.history.priors[dest] = append(state.history.priors[dest], prior{seq, file})
		}
	}
}

// retain moves the file a step replaced out of the backup folder, and says where it is, or "" when the step made the file.
func retain(in Room, seq uint64, n int, replaced step) string {
	if !exists(replaced.Backup) {
		return ""
	}
	if err := os.MkdirAll(priorDir(in), 0o755); err != nil {
		return ""
	}
	file := filepath.Join(priorDir(in), strconv.FormatUint(seq, 10)+"-"+strconv.Itoa(n))
	if os.Rename(replaced.Backup, file) != nil {
		return ""
	}
	return file
}
