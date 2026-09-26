package jobs

import (
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/getlatentic/haas/internal/evidence"
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

// Open reads what is on disk, and lets the evidence decide.
//
// A container is a child of the container runtime, not of this server, so a job
// whose server was killed often runs on, finishes the gate and writes its
// manifest. Calling that interrupted because the state file still says running
// would throw away a verdict that is on disk. A job is only interrupted when it
// left no manifest behind.
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
			record = ended(record, root)
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
	// A manifest left by an earlier run of this directory would be read as this
	// run's verdict if this run never reaches the end. The run about to start
	// overwrites the rest of out/ anyway, so the old verdict goes with it.
	if err := os.Remove(oldest.Dir(i.root).OutFile("manifest.json")); err != nil && !os.IsNotExist(err) {
		return Record{}, false, err
	}
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

// ended asks the job directory how a run that nobody was watching turned out.
func ended(record Record, root string) Record {
	manifest, err := evidence.Read(record.Dir(root))
	// A manifest is this run's only if it was written after this run started.
	// Anything older belongs to a previous run of the same directory, and
	// treating it as a verdict would report work that this run never did.
	if err != nil || manifest.AtUnix < record.Started {
		// Nothing was left behind, so nothing is known: an exit code of zero
		// would read as success.
		record.State = Interrupted
		record.Exit = -1
		record.Ended = Now()
		return record
	}
	// The manifest knows when the run ended. The clock at recovery only knows
	// when somebody noticed, which would count the outage as part of the job.
	record.Ended = manifest.AtUnix
	record.Exit = manifest.Gate.Code
	record.State = Failed
	if manifest.Gate.Passed {
		record.State = Passed
	}
	return record
}
