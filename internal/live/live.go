// Package live reads settings again while the server runs, so that a change to what an organisation is offered, or to its limits, takes
// effect in a minute and nobody in the middle of a task has to be sent away for a restart.
//
// A setting is a file, or a folder of files. The watcher looks at each on a fixed cadence, and when what is in it has changed it calls
// the setting's Apply, which reads it again. A file that does not read is refused whole and the setting stays as it was; the
// watcher says so, and does not try the same content again.
package live

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Source is one setting that is read again.
type Source struct {
	// Name is what the record calls it, such as GUSTO_SPEND_POLICY.
	Name string
	// Path is the file, or the folder of .json files, it is read from. A source with no path is not watched.
	Path string
	// Apply reads the setting again and makes it so. It returns an error to refuse what it read.
	Apply func() error
}

// Change is what the watcher says of a setting that changed.
type Change struct {
	Name string
	// Was and Now are fingerprints of what was there, which say that a change happened without keeping what it was.
	Was, Now string
	// Err is why it was refused, or nil when it is now in force.
	Err error
}

// Fingerprint is a short fingerprint of what a file, or the .json files of a folder, holds. A path that is not there has none.
func Fingerprint(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	if !info.IsDir() {
		text, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		hash.Write(text)
		return hex.EncodeToString(hash.Sum(nil))[:16], nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		text, err := os.ReadFile(filepath.Join(path, name))
		if err != nil {
			return "", err
		}
		hash.Write([]byte(name + "\x00"))
		hash.Write(text)
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))[:16], nil
}

// Watch looks at each source every so often until the context ends, and tells told of each change. What is there when it starts is
// taken to be what is already in force. A source that cannot be looked at this time is looked at the next.
//
// A change is taken when the same new content is there at two looks in a row, so that a file an editor has emptied and not yet
// filled, or a tool that is part of the way through writing it, is not read as the setting. Where an empty file would mean no
// rules, that is the difference between a policy and none.
func Watch(ctx context.Context, every time.Duration, sources []Source, told func(Change)) {
	seen := map[string]string{}
	waiting := map[string]string{}
	for _, source := range sources {
		if source.Path != "" {
			seen[source.Name], _ = Fingerprint(source.Path)
		}
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		for _, source := range sources {
			if source.Path == "" {
				continue
			}
			now, err := Fingerprint(source.Path)
			if err != nil || now == seen[source.Name] {
				delete(waiting, source.Name)
				continue
			}
			if waiting[source.Name] != now {
				waiting[source.Name] = now
				continue
			}
			delete(waiting, source.Name)
			change := Change{Name: source.Name, Was: seen[source.Name], Now: now, Err: source.Apply()}
			seen[source.Name] = now
			if told != nil {
				told(change)
			}
		}
	}
}
