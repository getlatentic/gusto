//go:build linux || windows

package mirror

import (
	"context"
	"errors"
	"io/fs"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/fsnotify/fsnotify"
)

const limitAdvice = "the folder has more subfolders than the system lets one program watch; the timed runs still keep it, and `sysctl fs.inotify.max_user_watches` raises the limit on Linux"

// watch watches every folder under root that is not left out, and each folder made later. The system tells of one folder and not
// of what is under it, so each is added on its own.
func watch(ctx context.Context, root string, ignore *Ignore, told func(string)) (<-chan struct{}, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	wake := make(chan struct{}, 1)
	w := &folders{watcher: watcher, root: root, ignore: ignore, told: told}
	w.add(root)
	go func() {
		defer watcher.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case event, open := <-watcher.Events:
				if !open {
					return
				}
				if w.heard(event) {
					pulse(wake)
				}
			case err, open := <-watcher.Errors:
				if !open {
					return
				}
				if errors.Is(err, fsnotify.ErrEventOverflow) {
					pulse(wake)
				}
			}
		}
	}()
	return wake, nil
}

type folders struct {
	watcher *fsnotify.Watcher
	root    string
	ignore  *Ignore
	told    func(string)
	full    bool
}

// heard says whether an event is about something the sync keeps, and starts watching a folder that was made.
func (w *folders) heard(event fsnotify.Event) bool {
	name, err := filepath.Rel(w.root, event.Name)
	if err != nil || name == "." {
		return err == nil
	}
	name = filepath.ToSlash(name)
	if event.Has(fsnotify.Create) {
		if info, err := statDir(event.Name); err == nil && info {
			if w.ignore.Skips(name, true) {
				return false
			}
			w.add(event.Name)
			return true
		}
	}
	return !w.ignore.Skips(name, false)
}

// add watches a folder and the folders under it.
func (w *folders) add(top string) {
	_ = filepath.WalkDir(top, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.IsDir() {
			return nil
		}
		if name, err := filepath.Rel(w.root, path); err == nil && name != "." && w.ignore.Skips(filepath.ToSlash(name), true) {
			return filepath.SkipDir
		}
		if err := w.watcher.Add(path); err != nil {
			if errors.Is(err, syscall.ENOSPC) || (runtime.GOOS == "linux" && errors.Is(err, syscall.EMFILE)) {
				if !w.full && w.told != nil {
					w.told(limitAdvice)
				}
				w.full = true
				return filepath.SkipAll
			}
		}
		return nil
	})
}
