package roomwork

import (
	"io/fs"
	"os"
)

// joinLines merges a text file the turn changed with the folder's file, which something else changed since the turn began, from
// the version both began with, and writes the result over the turn's copy. It says false, and writes nothing, when the file is
// not text on both sides, the version both began with is not known, or the two changed the same lines.
func (t *Turn) joinLines(name, mine string, ours, there entry) bool {
	if ours.kind != file || there.kind != file || ours.mode != there.mode || t.blocked(name) {
		return false
	}
	kept, known := t.hub.began(t.room, t, name)
	if !known {
		return false
	}
	base, ok := readText(kept, t.shared[name].size)
	if !ok {
		return false
	}
	mineText, ok := readText(mine, ours.size)
	if !ok {
		return false
	}
	theirs, ok := readText(t.inFolder(name), there.size)
	if !ok {
		return false
	}
	merged, ok := mergeLines(base, mineText, theirs)
	return ok && replaceWith(mine, merged, ours.mode)
}

// readText is a file's bytes when it holds text of the size it was seen at.
func readText(path string, size int64) ([]byte, bool) {
	if size > mostMergeBytes {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil || int64(len(data)) != size || !isText(data) {
		return nil, false
	}
	return data, true
}

// replaceWith writes data to path whole: beside it first, then moved over it.
func replaceWith(path string, data []byte, mode fs.FileMode) bool {
	next := path + ".joined"
	if os.WriteFile(next, data, mode) != nil {
		return false
	}
	if os.Chmod(next, mode) != nil || os.Rename(next, path) != nil {
		_ = os.Remove(next)
		return false
	}
	return true
}
