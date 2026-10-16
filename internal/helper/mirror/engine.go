package mirror

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/getlatentic/gusto/internal/helper"
)

// Engine keeps one local folder the same as one agent's folder in the cloud.
type Engine struct {
	Root   string
	Remote Remote
	State  *State
	Ignore *Ignore
	// MaxFileBytes is the largest file moved, 64 MiB when it is zero. A larger one is left as it is, on either side.
	MaxFileBytes int64
	// Force lets a run go on that the guards would stop: a first run over a very large folder, and a run that would delete most
	// of what was kept.
	Force bool
	Now   func() time.Time
	Log   func(format string, args ...any)
}

// mostFirstRun is how many files a folder may have on a first run, which is a sign that the wrong folder was named.
const mostFirstRun = 20000

// Report is what a run found and did.
type Report struct {
	Done      map[Kind]int
	Conflicts []string
	Errors    []string
	Skipped   Skipped
	Plan      []Action
}

func (e *Engine) mostBytes() int64 {
	if e.MaxFileBytes > 0 {
		return e.MaxFileBytes
	}
	return 64 << 20
}

func (e *Engine) logf(format string, args ...any) {
	if e.Log != nil {
		e.Log(format, args...)
	}
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// ErrGuard is a run that was stopped before it did anything, because what it would do looks like a mistake.
var ErrGuard = errors.New("stopped before doing anything")

// refuseRoot says why a folder may not be kept in step: the whole disk and a person's whole home are not a project.
func refuseRoot(root string) error {
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New(root + " is not a folder")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if home, err := os.UserHomeDir(); err == nil && filepath.Clean(home) == filepath.Clean(absolute) {
		return errors.New("that is your home folder, which is not one project: name a folder inside it")
	}
	if filepath.Dir(absolute) == absolute {
		return errors.New("that is the root of a disk, which is not one project: name a folder inside it")
	}
	return nil
}

// picture is both sides as a run sees them, after what is left out has been taken out.
type picture struct {
	local, remote Snapshot
	scanned       map[string]scanned
	remoteFiles   map[string]helper.RemoteFile
	base          Snapshot
	skipped       Skipped
	frozen        map[string]bool
}

// look reads both sides. A path that is too large on either side is frozen: it is taken out of all three pictures, so that a
// file the sync does not move is never taken for one that was deleted.
func (e *Engine) look(ctx context.Context) (*picture, error) {
	p := &picture{local: Snapshot{}, remote: Snapshot{}, base: Snapshot{}, frozen: map[string]bool{}, remoteFiles: map[string]helper.RemoteFile{}}
	var err error
	if p.scanned, err = e.scanLocal(&p.skipped); err != nil {
		return nil, fmt.Errorf("the local folder could not be read: %w", err)
	}
	files, cut, err := e.Remote.Files(ctx)
	if err != nil {
		return nil, err
	}
	if cut {
		return nil, errors.New("the folder in the cloud has more files than can be listed: leave the large ones out with a .gustoignore, or the files that are not listed would look deleted")
	}
	var needHash []string
	for _, file := range files {
		if e.Ignore.Skips(file.Path, false) {
			p.skipped.Ignored++
			continue
		}
		if file.Size > e.mostBytes() {
			p.skipped.Large = append(p.skipped.Large, file.Path)
			p.frozen[file.Path] = true
			continue
		}
		p.remoteFiles[file.Path] = file
		if known, ok := e.State.Remote[file.Path]; !ok || known.Size != file.Size || known.ModifiedNs != file.ModifiedNs {
			needHash = append(needHash, file.Path)
		}
	}
	if err := e.learnHashes(ctx, p, needHash); err != nil {
		return nil, err
	}
	for name, file := range p.remoteFiles {
		if known, ok := e.State.Remote[name]; ok {
			p.remote[name] = Entry{Hash: known.Hash, Size: file.Size}
		}
	}
	for name, file := range p.scanned {
		p.local[name] = file.Entry
	}
	for _, name := range p.skipped.Large {
		p.frozen[name] = true
	}
	for name, entry := range e.State.Base {
		if e.Ignore.Skips(name, false) {
			continue
		}
		p.base[name] = entry
	}
	for name := range p.frozen {
		delete(p.local, name)
		delete(p.remote, name)
		delete(p.base, name)
	}
	return p, nil
}

// learnHashes asks the cloud for the hashes of the files whose size or time is not what was hashed. A file it cannot give a
// hash for has gone, or is not readable, and is left out of this run on that side.
func (e *Engine) learnHashes(ctx context.Context, p *picture, names []string) error {
	const batch = 2000
	for start := 0; start < len(names); start += batch {
		end := min(start+batch, len(names))
		got, err := e.Remote.Hashes(ctx, names[start:end])
		if err != nil {
			return err
		}
		for _, name := range names[start:end] {
			hash, ok := got[name]
			if !ok {
				delete(p.remoteFiles, name)
				delete(e.State.Remote, name)
				continue
			}
			file := p.remoteFiles[name]
			e.State.Remote[name] = Stamp{Hash: hash, Size: file.Size, ModifiedNs: file.ModifiedNs}
		}
	}
	return nil
}

// guard stops a run that looks like a mistake. A folder that is suddenly empty, as a disk that did not mount is, would
// otherwise be carried to the cloud as every file deleted.
func (e *Engine) guard(p *picture, plan []Action) error {
	if e.Force {
		return nil
	}
	if e.State.Fresh() && len(p.scanned) > mostFirstRun {
		return fmt.Errorf("%w: %d files is a lot for a first run, and may be the wrong folder: name a smaller one, leave things out with a .gustoignore, or run again with -force", ErrGuard, len(p.scanned))
	}
	deletions := 0
	for _, act := range plan {
		if act.Kind == DeleteLocal || act.Kind == DeleteRemote {
			deletions++
		}
	}
	if deletions >= 20 && deletions*2 >= len(p.base) {
		return fmt.Errorf("%w: this would delete %d of the %d files kept in step, which looks like one side being emptied by mistake (a disk that did not mount, a folder moved): check both folders, or run again with -force", ErrGuard, deletions, len(p.base))
	}
	return nil
}

// Preview says what a run would do, and does nothing.
func (e *Engine) Preview(ctx context.Context) (*Report, error) {
	if err := refuseRoot(e.Root); err != nil {
		return nil, err
	}
	p, err := e.look(ctx)
	if err != nil {
		return nil, err
	}
	plan := Plan(p.base, p.local, p.remote, e.now(), e.taken(p))
	report := &Report{Done: map[Kind]int{}, Skipped: p.skipped, Plan: plan}
	for _, act := range plan {
		report.Done[act.Kind]++
	}
	return report, e.guard(p, plan)
}

func (e *Engine) taken(p *picture) func(string) bool {
	return func(name string) bool {
		if _, ok := p.remoteFiles[name]; ok {
			return true
		}
		if full, err := e.onDisk(name); err == nil {
			_, statErr := os.Lstat(full)
			return statErr == nil
		}
		return true
	}
}

// Once looks at both sides, decides, and does it. What could not be done is in the report and is looked at again the next
// time: a failed step is never recorded as done, so it is planned the same way again.
func (e *Engine) Once(ctx context.Context) (*Report, error) {
	if err := refuseRoot(e.Root); err != nil {
		return nil, err
	}
	p, err := e.look(ctx)
	if err != nil {
		return nil, err
	}
	plan := Plan(p.base, p.local, p.remote, e.now(), e.taken(p))
	if err := e.guard(p, plan); err != nil {
		return nil, err
	}
	report := &Report{Done: map[Kind]int{}, Skipped: p.skipped, Plan: plan}
	var done []Action
	for _, act := range plan {
		if ctx.Err() != nil {
			break
		}
		if err := e.carry(ctx, act, p); err != nil {
			e.logf("%s %s: %v", act.Kind, act.Path, err)
			report.Errors = append(report.Errors, fmt.Sprintf("%s %s: %v", act.Kind, act.Path, err))
			continue
		}
		done = append(done, act)
		report.Done[act.Kind]++
		if act.Kind == Conflict {
			report.Conflicts = append(report.Conflicts, act.Copy)
		}
	}
	// Ignored paths are not carried over: if a pattern is taken away later they are new, and new is the safe thing to be.
	e.State.Base = Settled(p.base, p.local, p.remote, done)
	for name, entry := range e.keptFrozen(p) {
		e.State.Base[name] = entry
	}
	e.forgetStale(p)
	sort.Strings(report.Conflicts)
	return report, e.State.Save()
}

// keptFrozen is the record of the files a run did not look at because they are too large, as it was, so that they are what
// they were when they are looked at again.
func (e *Engine) keptFrozen(p *picture) Snapshot {
	kept := Snapshot{}
	for name := range p.frozen {
		if entry, ok := e.State.Base[name]; ok {
			kept[name] = entry
		}
	}
	return kept
}

// forgetStale drops the remembered hashes of files that are not there any more, so the record does not grow forever.
func (e *Engine) forgetStale(p *picture) {
	for name := range e.State.Local {
		if _, ok := p.scanned[name]; !ok {
			if full, err := e.onDisk(name); err != nil || !exists(full) {
				delete(e.State.Local, name)
			}
		}
	}
	for name := range e.State.Remote {
		if _, ok := p.remoteFiles[name]; !ok {
			delete(e.State.Remote, name)
		}
	}
}

func exists(path string) bool { _, err := os.Lstat(path); return err == nil }
