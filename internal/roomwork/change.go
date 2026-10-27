package roomwork

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
)

// Hold is for a change to a room's folder that is not a turn's, such as a person's upload or edit: it waits until no merge is
// changing the folder and keeps any from starting until done is called, so neither replaces what the other wrote. The files at
// paths, relative to the folder, are kept as they were for the turns working in the room, and a turn that changed one of them
// too is merged with the change line by line, as with another turn's. done says whether the folder was changed. With no turn
// working in the room there is no merge to wait for, and Hold returns at once.
func (h *Hub) Hold(ctx context.Context, folder string, paths []string) (done func(changed bool), err error) {
	if folder, err = filepath.Abs(folder); err != nil {
		return nil, err
	}
	h.mu.Lock()
	state := h.rooms[folder]
	h.mu.Unlock()
	if state == nil {
		return func(bool) {}, nil
	}
	release, err := guard(ctx, state)
	if err != nil {
		return nil, err
	}
	seq := h.next(state)
	kept := keepBefore(Room{Folder: folder, Meta: state.meta}, seq, paths)
	return func(changed bool) {
		defer release()
		if !changed {
			for _, file := range kept {
				_ = os.Remove(file)
			}
			return
		}
		h.changed(state, seq, paths, kept)
	}, nil
}

// next numbers a change as a merge is numbered. The caller holds the guard, so no merge takes a number between.
func (h *Hub) next(state *roomState) uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	state.seq++
	return state.seq
}

// keepBefore links each file at paths into the room's kept versions, so it outlives a change that replaces or removes it. A file
// that is not there, or cannot be linked, has no entry, and a turn that changed it too conflicts with the change instead.
func keepBefore(in Room, seq uint64, paths []string) map[string]string {
	kept := map[string]string{}
	if len(paths) == 0 || os.MkdirAll(priorDir(in), 0o755) != nil {
		return kept
	}
	for i, name := range paths {
		file := filepath.Join(priorDir(in), strconv.FormatUint(seq, 10)+"-person-"+strconv.Itoa(i))
		if os.Link(filepath.Join(in.Folder, filepath.FromSlash(name)), file) == nil {
			kept[name] = file
		}
	}
	return kept
}

// changed records a held change as a merge by nobody the room can name, keeping what it replaced.
func (h *Hub) changed(state *roomState, seq uint64, paths []string, kept map[string]string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, name := range paths {
		state.history.wrote[name] = landed{"", seq}
		state.history.priors[name] = append(state.history.priors[name], prior{seq, kept[name]})
	}
}
