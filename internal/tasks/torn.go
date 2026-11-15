package tasks

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/getlatentic/gusto/pkg/task"
)

// errTorn is the answer to reading a state.json that is there and cannot be parsed. Writes go through atomicfile, so a file
// like that is a disk fault, or a file written by something else.
var errTorn = errors.New("the task's state.json is damaged")

// removing marks a task directory that is being deleted. Its name starts with a dot so that no scan reads it as a task.
const removing = ".removing-"

// tornSuffix names the damaged state.json that is kept beside the one written in its place.
const tornSuffix = ".damaged-"

// readOrRebuild reads a task's state, and when state.json is damaged, builds the record again from what else the directory
// holds. rebuilt says it did. A directory with no state.json, or one whose job.json cannot say what the task was, is not a
// task here.
func readOrRebuild(root string, dir task.Dir) (record Record, rebuilt bool, err error) {
	record, err = read(dir)
	if err == nil || errors.Is(err, fs.ErrNotExist) {
		return record, false, err
	}
	record, err = rebuild(root, dir)
	return record, err == nil, err
}

// rebuild is the record of a task whose state.json is damaged, made from job.json, which says what was asked, and the
// manifest, which says how a run that finished ended. What only state.json held is lost: who stopped the task, who paid, the
// request id and the handing on of a turn. A task is never made queued again, since a guess could run it twice: it is
// interrupted when it left no verdict.
func rebuild(root string, dir task.Dir) (Record, error) {
	spec, err := task.Read(dir)
	if err != nil {
		return Record{}, fmt.Errorf("%w and job.json cannot stand in for it: %v", errTorn, err)
	}
	info, err := os.Stat(dir.Spec())
	if err != nil {
		return Record{}, err
	}
	asked := Record{
		ID: filepath.Base(dir.Path()), Brief: spec.Brief, Repo: spec.Repo, Harness: spec.Harness, Model: spec.Model,
		Instance: spec.Session, Thread: spec.Thread, Agent: spec.Agent, Asked: spec.Person, Team: spec.Team,
		Workspace: spec.Workspace, Queued: info.ModTime().Unix(), Started: info.ModTime().Unix(),
	}
	return ended(asked, root), nil
}

// setAsideState keeps a damaged state.json beside the file that replaces it, so nothing is lost by the repair.
func setAsideState(dir task.Dir) (string, error) {
	kept := dir.Path() + "/state.json" + tornSuffix + fmt.Sprint(Now())
	return kept, os.Rename(dir.Path()+"/state.json", kept)
}

// repairTorn writes the rebuilt records of damaged tasks over the damaged files, which are kept, and says what it did.
func repairTorn(root string, torn []Record) error {
	for _, record := range torn {
		dir := record.Dir(root)
		kept, err := setAsideState(dir)
		if err != nil {
			return err
		}
		if err := write(dir, record); err != nil {
			return err
		}
		slog.Warn("a task's state.json was damaged: it is rebuilt from job.json and the damaged file is kept",
			"task", record.ID, "state", record.State, "kept", filepath.Base(kept))
	}
	return nil
}

// sweepRemoving finishes the deletion of task directories a crash interrupted.
func sweepRemoving(root string) {
	listing, _ := os.ReadDir(root)
	for _, entry := range listing {
		if !strings.HasPrefix(entry.Name(), removing) {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			slog.Warn("a task directory that was being removed could not be deleted", "dir", entry.Name(), "error", err)
		}
	}
}
