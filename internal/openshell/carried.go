package openshell

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/getlatentic/haas/internal/catalog"
)

// Remembered is an image that says what it carries from an answer kept beside the image file, and asks the
// inner image only when there is none for this file. Asking makes a sandbox from the whole image, which
// costs a virtual machine boot, and the first one on a host waits for the gateway to import the image, so a
// server that asked at every start made a person wait for it. The answer is the file's own: another file, or
// the same one made again, has none.
func Remembered(inner catalog.Image, path string) catalog.Image {
	return &remembered{inner: inner, path: path}
}

type remembered struct {
	inner catalog.Image
	path  string
	mu    sync.Mutex
}

// kept is the answer, and the file it is the answer for.
type kept struct {
	Size    int64    `json:"size"`
	Changed int64    `json:"changed"`
	Asked   []string `json:"asked"`
	OnPath  []string `json:"onPath"`
}

func (r *remembered) file() string { return r.path + ".carried.json" }

func (r *remembered) Carried(programs []string) catalog.Carried {
	info, err := os.Stat(r.path)
	if err != nil {
		return r.inner.Carried(programs)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if answer, ok := r.recall(info, programs); ok {
		return answer
	}
	answer := r.inner.Carried(programs)
	if answer.Unanswered == nil && !answer.Unasked {
		r.keep(info, programs, answer)
	}
	return answer
}

func (r *remembered) recall(info os.FileInfo, programs []string) (catalog.Carried, bool) {
	text, err := os.ReadFile(r.file())
	if err != nil {
		return catalog.Carried{}, false
	}
	var k kept
	if json.Unmarshal(text, &k) != nil || k.Size != info.Size() || k.Changed != info.ModTime().UnixNano() {
		return catalog.Carried{}, false
	}
	for _, program := range programs {
		if !slices.Contains(k.Asked, program) {
			return catalog.Carried{}, false
		}
	}
	return catalog.Carried{OnPath: names(strings.Join(k.OnPath, "\n"))}, true
}

func (r *remembered) keep(info os.FileInfo, programs []string, answer catalog.Carried) {
	k := kept{Size: info.Size(), Changed: info.ModTime().UnixNano(), Asked: slices.Clone(programs)}
	for program, found := range answer.OnPath {
		if found {
			k.OnPath = append(k.OnPath, program)
		}
	}
	sort.Strings(k.Asked)
	sort.Strings(k.OnPath)
	text, err := json.Marshal(k)
	if err != nil {
		return
	}
	// Written whole and moved into place, so a server that starts while another writes reads all of it or none.
	temp, err := os.CreateTemp(filepath.Dir(r.path), ".carried-")
	if err != nil {
		return
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(text); err != nil || temp.Close() != nil {
		return
	}
	os.Rename(temp.Name(), r.file())
}
