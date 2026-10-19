package jobs

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"

	"github.com/getlatentic/gusto/internal/job"
)

// Scan reads the tasks on disk without starting anything or changing anything,
// so a second process can look at a server's tasks while it runs. Opening an
// Index would instead recover a task the server is running as interrupted.
func Scan(root string) ([]Record, error) { return readAll(root) }

// readers is how many state files are read at once. Reading a hundred thousand of them one after another waits
// on the disk for most of its time.
const readers = 16

// readAll reads the state of every task directory under root, in the order the directory lists them. A directory
// without a readable state.json is not a task and is left out.
func readAll(root string) ([]Record, error) {
	listing, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, entry := range listing {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		}
	}
	found := make([]*Record, len(dirs))
	var next atomic.Int64
	var group sync.WaitGroup
	for w := 0; w < min(readers, len(dirs)); w++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for at := int(next.Add(1)) - 1; at < len(dirs); at = int(next.Add(1)) - 1 {
				if record, err := read(job.Dir(root + "/" + dirs[at])); err == nil {
					found[at] = &record
				}
			}
		}()
	}
	group.Wait()
	all := make([]Record, 0, len(dirs))
	for _, record := range found {
		if record != nil {
			all = append(all, *record)
		}
	}
	return all, nil
}

// Remove deletes a finished task: its directory, and its place in the index. A
// task that is queued or running is refused, because the directory is in use.
func (i *Index) Remove(id string) error {
	i.guard.Lock()
	defer i.guard.Unlock()
	record, found := i.records[id]
	if !found {
		return fmt.Errorf("there is no task %s", id)
	}
	if !record.State.Done() {
		return fmt.Errorf("the task %s is %s and cannot be removed", id, record.State)
	}
	if err := os.RemoveAll(record.Dir(i.root).Path()); err != nil {
		return err
	}
	delete(i.records, id)
	if i.pump != nil {
		i.pump.push(op{id: id, delete: true})
	}
	return nil
}
