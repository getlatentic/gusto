package tasks

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/getlatentic/gusto/pkg/task"
)

// Scan reads the tasks on disk without starting anything or changing anything,
// so a second process can look at a server's tasks while it runs. Opening an
// Index would instead recover a task the server is running as interrupted.
// A task whose state.json is damaged is given as it can be rebuilt, and the file is left as it is.
func Scan(root string) ([]Record, error) {
	all, _, err := readTasks(root)
	return all, err
}

// readers is how many state files are read at once. Reading a hundred thousand of them one after another waits
// on the disk for most of its time.
const readers = 16

// reading is the outcome of reading one task directory.
type reading struct {
	record  Record
	rebuilt bool
	ok      bool
}

// readTasks reads the state of every task directory under root, in the order the directory lists them, and says which of them
// it rebuilt because their state.json was damaged. A directory without a state.json is not a task and is left out, as is one
// whose damaged state.json job.json cannot stand in for, which is logged.
func readTasks(root string) (all, rebuilt []Record, err error) {
	listing, err := os.ReadDir(root)
	if err != nil {
		return nil, nil, err
	}
	var dirs []string
	for _, entry := range listing {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			dirs = append(dirs, entry.Name())
		}
	}
	found := make([]reading, len(dirs))
	var next atomic.Int64
	var group sync.WaitGroup
	for w := 0; w < min(readers, len(dirs)); w++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for at := int(next.Add(1)) - 1; at < len(dirs); at = int(next.Add(1)) - 1 {
				found[at] = readDir(root, dirs[at])
			}
		}()
	}
	group.Wait()
	all = make([]Record, 0, len(dirs))
	for _, each := range found {
		if !each.ok {
			continue
		}
		all = append(all, each.record)
		if each.rebuilt {
			rebuilt = append(rebuilt, each.record)
		}
	}
	return all, rebuilt, nil
}

func readDir(root, name string) reading {
	record, rebuilt, err := readOrRebuild(root, task.Dir(root+"/"+name))
	if err == nil {
		return reading{record, rebuilt, true}
	}
	if !errors.Is(err, fs.ErrNotExist) {
		slog.Warn("a task's state.json cannot be read, so the task is left out", "task", name, "error", err)
	}
	return reading{}
}

// Remove deletes a finished task: its directory, and its place in the index. A
// task that is queued or running is refused, because the directory is in use.
//
// The directory is renamed out of the way first, which either happens or does not, so a task is never left in the index with
// half its files gone. What was renamed is deleted after the lock is let go, since a task's folder can be large, and a crash
// in between leaves a directory that Open deletes.
func (i *Index) Remove(id string) error {
	gone, err := i.detach(id)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(gone); err != nil {
		slog.Warn("a removed task's files could not be deleted, and are left for the next start", "dir", gone, "error", err)
	}
	return nil
}

// detach takes a finished task out of the index and its directory out of its place, and says where the directory is now.
func (i *Index) detach(id string) (string, error) {
	i.guard.Lock()
	defer i.guard.Unlock()
	record, found := i.records[id]
	if !found {
		return "", fmt.Errorf("there is no task %s", id)
	}
	if !record.State.Done() {
		return "", fmt.Errorf("the task %s is %s and cannot be removed", id, record.State)
	}
	gone := filepath.Join(i.root, removing+id)
	if err := os.Rename(record.Dir(i.root).Path(), gone); err != nil {
		return "", err
	}
	delete(i.records, id)
	if i.pump != nil {
		i.pump.push(op{id: id, delete: true})
	}
	return gone, nil
}
