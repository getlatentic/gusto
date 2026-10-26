package roomwork

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/getlatentic/gusto/internal/atomicfile"
)

// A merge changes the folder all at once or not at all. Every file to put in place is staged beside the folder first, each file
// it replaces or removes is kept in backup, and a journal says what is about to happen. If anything fails, or the server dies
// part way, the journal is enough to put every file back as it was.

// step is one file the merge puts in place, or removes when Stage is empty. Backup is where the file it replaces is kept.
type step struct {
	Dest   string `json:"dest"`
	Stage  string `json:"stage,omitempty"`
	Backup string `json:"backup"`
}

// journal is a merge that has begun and not finished: its steps, and the folders it made to hold them.
type journal struct {
	Steps []step   `json:"steps"`
	Made  []string `json:"made,omitempty"`
}

func stageDir(in Room) string  { return filepath.Join(in.Meta, "stage") }
func backupDir(in Room) string { return filepath.Join(in.Meta, "backup") }
func journalOf(in Room) string { return filepath.Join(in.Meta, "journal.json") }
func forget(in Room) {
	_ = os.RemoveAll(stageDir(in))
	_ = os.RemoveAll(backupDir(in))
	_ = os.Remove(journalOf(in))
}
func exists(path string) bool { _, err := os.Lstat(path); return err == nil }

// commit carries out the actions and returns its steps, whose backups hold the files they replaced. The caller holds the room's
// guard, and calls forget when it has done with the backups.
func commit(in Room, actions []action) ([]step, error) {
	if len(actions) == 0 {
		return nil, nil
	}
	forget(in)
	for _, dir := range []string{stageDir(in), backupDir(in)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	steps, err := stage(in, actions)
	if err != nil {
		forget(in)
		return nil, err
	}
	made, err := makeParents(in.Folder, steps)
	plan := journal{Steps: steps, Made: made}
	if err == nil {
		err = atomicfile.Write(journalOf(in), plan, 0o600)
	}
	if err == nil {
		err = run(plan)
	}
	if err == nil {
		// Taking the journal away is what makes the merge final.
		err = os.Remove(journalOf(in))
	}
	if err != nil {
		// The backups and the journal stay when the folder could not be put back, for recoverMerge to try again.
		if undo(plan) == nil {
			forget(in)
		}
		return nil, err
	}
	return steps, nil
}

// stage copies each file to be put in place into the staging folder, and works out where everything goes.
func stage(in Room, actions []action) ([]step, error) {
	steps := make([]step, len(actions))
	for i, each := range actions {
		steps[i] = step{Dest: filepath.Join(in.Folder, filepath.FromSlash(each.dest)), Backup: filepath.Join(backupDir(in), strconv.Itoa(i))}
		if each.source == "" {
			continue
		}
		info, err := os.Lstat(each.source)
		if err != nil {
			return nil, err
		}
		steps[i].Stage = filepath.Join(stageDir(in), strconv.Itoa(i))
		if err := place(each.source, steps[i].Stage, info); err != nil {
			return nil, err
		}
	}
	return steps, nil
}

// makeParents makes the folders the files go in, and says which it made. A folder on the way that is not a folder is an error.
func makeParents(folder string, steps []step) ([]string, error) {
	var made []string
	for _, each := range steps {
		if each.Stage == "" {
			continue
		}
		var missing []string
		for at := filepath.Dir(each.Dest); at != folder; at = filepath.Dir(at) {
			info, err := os.Lstat(at)
			if err == nil && !info.IsDir() {
				return made, &fs.PathError{Op: "merge", Path: at, Err: errors.New("is not a folder")}
			}
			if err != nil {
				missing = append(missing, at)
			}
		}
		for i := len(missing) - 1; i >= 0; i-- {
			if err := os.Mkdir(missing[i], 0o755); err != nil {
				return made, err
			}
			made = append(made, missing[i])
		}
	}
	return made, nil
}

func run(plan journal) error {
	for _, each := range plan.Steps {
		if err := each.do(); err != nil {
			return err
		}
	}
	return nil
}

func (s step) do() error {
	existing, err := os.Lstat(s.Dest)
	if s.Stage == "" {
		if err != nil {
			return nil
		}
		return os.Rename(s.Dest, s.Backup)
	}
	if err == nil {
		if err := keep(s.Dest, s.Backup, existing); err != nil {
			return err
		}
	}
	return os.Rename(s.Stage, s.Dest)
}

// keep saves the file a step is about to replace. A hard link costs nothing and leaves the file in place until the rename
// replaces it in one move, so a reader of the folder never finds it missing.
func keep(from, to string, info fs.FileInfo) error {
	if info.Mode()&fs.ModeSymlink != 0 {
		target, err := os.Readlink(from)
		if err != nil {
			return err
		}
		return os.Symlink(target, to)
	}
	if err := os.Link(from, to); err != nil {
		return copyBytes(from, to, info.Mode().Perm())
	}
	return nil
}

// undo puts the folder back as it was before the steps that ran, last first. A step ran when its staged file is gone, or for a
// removal when its backup exists.
func undo(plan journal) error {
	var failed error
	for i := len(plan.Steps) - 1; i >= 0; i-- {
		each := plan.Steps[i]
		var err error
		switch {
		case each.Stage != "" && exists(each.Stage):
		case each.Stage != "" && exists(each.Backup):
			err = os.Rename(each.Backup, each.Dest)
		case each.Stage != "":
			err = os.Remove(each.Dest)
		case exists(each.Backup) && !exists(each.Dest):
			err = os.Rename(each.Backup, each.Dest)
		}
		failed = errors.Join(failed, err)
	}
	for i := len(plan.Made) - 1; i >= 0; i-- {
		_ = os.Remove(plan.Made[i])
	}
	return failed
}

// recoverMerge puts back what a merge that was cut off had changed. A room with no journal has nothing to put back.
func recoverMerge(in Room) error {
	text, err := os.ReadFile(journalOf(in))
	if errors.Is(err, fs.ErrNotExist) {
		forget(in)
		return nil
	}
	if err != nil {
		return err
	}
	var plan journal
	if err := json.Unmarshal(text, &plan); err != nil {
		return err
	}
	err = undo(plan)
	forget(in)
	return err
}

// tidy removes the folders a merge emptied by removing files, up to but not including the room's folder.
func tidy(folder string, actions []action) {
	for _, each := range actions {
		if each.source != "" {
			continue
		}
		for at := filepath.Dir(filepath.Join(folder, filepath.FromSlash(each.dest))); at != folder; at = filepath.Dir(at) {
			if os.Remove(at) != nil {
				break
			}
		}
	}
}
