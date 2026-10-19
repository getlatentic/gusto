package mirror

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/getlatentic/gusto/internal/helper"
)

// errMoved is a local file that is not what the look found: the person is editing it, or it came while the run was going. It is
// left alone, and the next run looks again, which is when a change on both sides is told from one.
var errMoved = errors.New("the local file changed while the run was going, so it was left alone")

// carry does one action of a plan.
func (e *Engine) carry(ctx context.Context, act Action, p *picture) error {
	switch act.Kind {
	case Settle, Forget:
		return nil
	case Pull:
		return e.pull(ctx, act.Path, p.remote[act.Path], p)
	case Push:
		return e.push(ctx, act.Path, p)
	case DeleteLocal:
		return e.deleteLocal(act.Path, p)
	case DeleteRemote:
		return e.deleteRemote(ctx, act.Path, p)
	case Conflict:
		return e.conflict(ctx, act, p)
	}
	return fmt.Errorf("an action nobody knows: %s", act.Kind)
}

// unchanged says whether the local file at full is as the look found it. A file that was not there at the look must still not
// be there.
func unchanged(full string, was *scanned) error {
	info, err := os.Lstat(full)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if was != nil {
			return errMoved
		}
		return nil
	case err != nil:
		return err
	case was == nil || info.Size() != was.Size || info.ModTime().UnixNano() != was.modifiedNs:
		return errMoved
	}
	return nil
}

func (e *Engine) scannedAt(p *picture, name string) *scanned {
	if file, ok := p.scanned[name]; ok {
		return &file
	}
	return nil
}

// fetch downloads a file of the cloud to the local place full, whole or not at all, and checks it is what was meant: a file
// that changed in the cloud while it was being read is not kept, and the next run tries again. stillFree is asked at the last
// moment, after the download, whether the place is still as it was.
func (e *Engine) fetch(ctx context.Context, name, full, hash string, executable bool, stillFree func() error) error {
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	scratch, err := os.CreateTemp(filepath.Dir(full), ".gusto-pull-*")
	if err != nil {
		return err
	}
	defer os.Remove(scratch.Name())
	digest := sha256.New()
	err = e.Remote.Download(ctx, name, io.MultiWriter(scratch, digest))
	if err == nil {
		err = scratch.Sync()
	}
	if closeErr := scratch.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if got := hex.EncodeToString(digest.Sum(nil)); got != hash {
		return errors.New("the file changed in the cloud while it was being read")
	}
	mode := os.FileMode(0o644)
	if executable {
		mode = 0o755
	}
	if err := os.Chmod(scratch.Name(), mode); err != nil {
		return err
	}
	// The download took time, in which the person may have saved the file. It is looked at again at the last moment.
	if err := stillFree(); err != nil {
		return err
	}
	return os.Rename(scratch.Name(), full)
}

// remember records a local file just written, so the next look does not read it again.
func (e *Engine) remember(name, full, hash string) {
	if info, err := os.Lstat(full); err == nil {
		e.State.Local[name] = Stamp{Hash: hash, Size: info.Size(), ModifiedNs: info.ModTime().UnixNano()}
	}
}

func (e *Engine) pull(ctx context.Context, name string, want Entry, p *picture) error {
	full, err := e.onDisk(name)
	if err != nil {
		return err
	}
	was := e.scannedAt(p, name)
	if err := unchanged(full, was); err != nil {
		return err
	}
	executable := was != nil && was.executable
	if err := e.fetch(ctx, name, full, want.Hash, executable, func() error { return unchanged(full, was) }); err != nil {
		return err
	}
	e.remember(name, full, want.Hash)
	return nil
}

// sendFile uploads the local file at full over what the cloud has, which is `saw`, and checks it is what was meant.
func (e *Engine) sendFile(ctx context.Context, name, full, hash, saw string, executable bool) error {
	file, err := os.Open(full)
	if err != nil {
		return err
	}
	defer file.Close()
	got, err := e.Remote.Upload(ctx, name, file, saw, executable)
	if err != nil {
		return err
	}
	if got != hash {
		return errors.New("the local file changed while it was being sent, so it is sent again next time")
	}
	return nil
}

func (e *Engine) push(ctx context.Context, name string, p *picture) error {
	full, err := e.onDisk(name)
	if err != nil {
		return err
	}
	was := e.scannedAt(p, name)
	if was == nil {
		return errMoved
	}
	if err := unchanged(full, was); err != nil {
		return err
	}
	saw := absent
	if there, ok := p.remote[name]; ok {
		saw = there.Hash
	}
	return e.mapChanged(e.sendFile(ctx, name, full, was.Hash, saw, was.executable))
}

// absent is what a write says it saw when the file was not in the cloud.
const absent = "absent"

// mapChanged says in words what the server refusing a write means here.
func (e *Engine) mapChanged(err error) error {
	var changed *helper.Changed
	if errors.As(err, &changed) {
		return errors.New("the file changed in the cloud since it was looked at, so it is looked at again next time")
	}
	return err
}

func (e *Engine) deleteLocal(name string, p *picture) error {
	full, err := e.onDisk(name)
	if err != nil {
		return err
	}
	if err := unchanged(full, e.scannedAt(p, name)); err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	delete(e.State.Local, name)
	return nil
}

func (e *Engine) deleteRemote(ctx context.Context, name string, p *picture) error {
	return e.mapChanged(e.Remote.Remove(ctx, name, p.remote[name].Hash))
}

// conflict keeps both versions. The cloud's is brought down and written beside the local file first, and only when it is
// safe on this computer is the local file sent over the cloud's, and the copy sent beside it. If any step fails, what came
// before it is still there and nothing is lost: a failed run leaves the cloud's version in the cloud until it is safe here.
func (e *Engine) conflict(ctx context.Context, act Action, p *picture) error {
	copyPath, err := e.onDisk(act.Copy)
	if err != nil {
		return err
	}
	if exists(copyPath) {
		return errMoved
	}
	local, there := p.local[act.Path], p.remote[act.Path]
	full, err := e.onDisk(act.Path)
	if err != nil {
		return err
	}
	was := e.scannedAt(p, act.Path)
	if err := unchanged(full, was); err != nil {
		return err
	}
	if err := e.fetch(ctx, act.Path, copyPath, there.Hash, was != nil && was.executable, func() error { return unchanged(full, was) }); err != nil {
		return err
	}
	e.remember(act.Copy, copyPath, there.Hash)
	if err := e.mapChanged(e.sendFile(ctx, act.Path, full, local.Hash, there.Hash, was != nil && was.executable)); err != nil {
		// The cloud still has its own version, so the copy is not needed yet, and one left behind would be made again, and again,
		// at every try.
		_ = os.Remove(copyPath)
		delete(e.State.Local, act.Copy)
		return err
	}
	return e.mapChanged(e.sendFile(ctx, act.Copy, copyPath, there.Hash, absent, was != nil && was.executable))
}
