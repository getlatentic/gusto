package jobs

import (
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/getlatentic/haas/internal/job"
)

// Index is every job under one directory. One process owns it, so a mutex is
// the whole of the concurrency story: a second runner is what would earn a
// database, and there is not one yet.
type Index struct {
	root    string
	guard   sync.Mutex
	records map[string]Record
}

// Open reads what is on disk. A job left running by a server that stopped is
// marked interrupted, because nothing is watching its container any more.
func Open(root string) (*Index, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	index := &Index{root: root, records: map[string]Record{}}
	listing, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range listing {
		if !entry.IsDir() {
			continue
		}
		record, err := read(job.Dir(root + "/" + entry.Name()))
		if err != nil {
			continue // a directory without state is not a job this server made
		}
		if record.State == Running {
			record.State = Interrupted
			record.Ended = Now()
			if err := write(record.Dir(root), record); err != nil {
				return nil, err
			}
		}
		index.records[record.ID] = record
	}
	return index, nil
}

func (i *Index) Root() string { return i.root }

// Add writes a new job's state and takes it into the index.
func (i *Index) Add(record Record) error {
	i.guard.Lock()
	defer i.guard.Unlock()
	if _, taken := i.records[record.ID]; taken {
		return fmt.Errorf("a job called %s is already here", record.ID)
	}
	if err := write(record.Dir(i.root), record); err != nil {
		return err
	}
	i.records[record.ID] = record
	return nil
}

func (i *Index) Get(id string) (Record, bool) {
	i.guard.Lock()
	defer i.guard.Unlock()
	record, found := i.records[id]
	return record, found
}

// List is newest first, which is the order a person reads.
func (i *Index) List() []Record {
	i.guard.Lock()
	defer i.guard.Unlock()
	all := make([]Record, 0, len(i.records))
	for _, record := range i.records {
		all = append(all, record)
	}
	sort.Slice(all, func(a, b int) bool { return all[a].Queued > all[b].Queued })
	return all
}

// Claim takes the oldest queued job and marks it running, so two workers cannot
// take the same one.
func (i *Index) Claim() (Record, bool, error) {
	i.guard.Lock()
	defer i.guard.Unlock()
	var oldest *Record
	for id := range i.records {
		record := i.records[id]
		if record.State != Queued {
			continue
		}
		if oldest == nil || record.Queued < oldest.Queued {
			taken := record
			oldest = &taken
		}
	}
	if oldest == nil {
		return Record{}, false, nil
	}
	oldest.State = Running
	oldest.Started = Now()
	if err := write(oldest.Dir(i.root), *oldest); err != nil {
		return Record{}, false, err
	}
	i.records[oldest.ID] = *oldest
	return *oldest, true, nil
}

// Finish records how a job ended.
func (i *Index) Finish(id string, state State, exit int) error {
	i.guard.Lock()
	defer i.guard.Unlock()
	record, found := i.records[id]
	if !found {
		return fmt.Errorf("no job called %s", id)
	}
	record.State = state
	record.Exit = exit
	record.Ended = Now()
	if err := write(record.Dir(i.root), record); err != nil {
		return err
	}
	i.records[id] = record
	return nil
}
