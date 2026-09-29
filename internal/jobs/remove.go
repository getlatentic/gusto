package jobs

import (
	"fmt"
	"os"

	"github.com/getlatentic/haas/internal/job"
)

// Scan reads the jobs on disk without starting anything or changing anything,
// so a second process can look at a server's jobs while it runs. Opening an
// Index would instead recover a job the server is running as interrupted.
func Scan(root string) ([]Record, error) {
	listing, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var all []Record
	for _, entry := range listing {
		if !entry.IsDir() {
			continue
		}
		if record, err := read(job.Dir(root + "/" + entry.Name())); err == nil {
			all = append(all, record)
		}
	}
	return all, nil
}

// Remove deletes a finished job: its directory, and its place in the index. A
// job that is queued or running is refused, because the directory is in use.
func (i *Index) Remove(id string) error {
	i.guard.Lock()
	defer i.guard.Unlock()
	record, found := i.records[id]
	if !found {
		return fmt.Errorf("there is no job %s", id)
	}
	if !record.State.Done() {
		return fmt.Errorf("the job %s is %s and cannot be removed", id, record.State)
	}
	if err := os.RemoveAll(record.Dir(i.root).Path()); err != nil {
		return err
	}
	delete(i.records, id)
	return nil
}
