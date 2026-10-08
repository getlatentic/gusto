//go:build darwin && cgo

package mirror

import (
	"context"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsevents"
)

// watch asks macOS for the changes under root: it reports a whole tree from one stream, so there is no limit to reach.
func watch(ctx context.Context, root string, ignore *Ignore, _ func(string)) (<-chan struct{}, error) {
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	stream := &fsevents.EventStream{Paths: []string{real}, Flags: fsevents.FileEvents | fsevents.WatchRoot, Latency: 50 * time.Millisecond}
	if err := stream.Start(); err != nil {
		return nil, err
	}
	wake := make(chan struct{}, 1)
	go func() {
		defer stream.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case events, open := <-stream.Events:
				if !open {
					return
				}
				if matters(events, real, ignore) {
					pulse(wake)
				}
			}
		}
	}()
	return wake, nil
}

// matters says whether any of the events is about something the sync keeps.
func matters(events []fsevents.Event, root string, ignore *Ignore) bool {
	for _, event := range events {
		name, err := filepath.Rel(root, event.Path)
		switch {
		case err != nil || event.Flags&fsevents.RootChanged != 0:
			return true
		case name == ".":
			// The folder's own entry changes whenever something in it does, and what changed is told on its own.
		case !ignore.Skips(filepath.ToSlash(name), event.Flags&fsevents.ItemIsDir != 0):
			return true
		}
	}
	return false
}
