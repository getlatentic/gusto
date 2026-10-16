package jobs

import (
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/job"
	"github.com/getlatentic/gusto/internal/quota"
)

// Index is every job under one directory. One process owns it, so a mutex is
// the whole of the concurrency story: a second runner is what would earn a
// database, and there is not one yet.
type Index struct {
	root    string
	guard   sync.Mutex
	records map[string]Record
	// seq is the arrival order the next job gets, so two jobs submitted in one
	// second still run in the order they were sent.
	seq    int64
	limits Limits
	// quota is what the whole server may run, when it is under one.
	quota *quota.File
	// finished and the two times below are counted since this server started,
	// for the metrics: what is on disk says how a job ended, and not how many
	// ended today.
	finished                  map[State]int64
	lastStarted, lastFinished int64
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
	index := &Index{root: root, records: map[string]Record{}, finished: map[State]int64{}}
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
	index.number()
	return index, nil
}

// number gives every job an arrival order, and says what the next one is.
// A job written before this field existed has none, so they are numbered by
// the clock and then by name: arbitrary between two of the same second, but
// the same on every read, and always before anything queued since.
func (i *Index) number() {
	var missing []Record
	for id := range i.records {
		if record := i.records[id]; record.Seq == 0 {
			missing = append(missing, record)
		} else if record.Seq >= i.seq {
			i.seq = record.Seq + 1
		}
	}
	sort.Slice(missing, func(a, b int) bool {
		if missing[a].Queued != missing[b].Queued {
			return missing[a].Queued < missing[b].Queued
		}
		return missing[a].ID < missing[b].ID
	})
	for at, record := range missing {
		record.Seq = int64(at) - int64(len(missing))
		i.records[record.ID] = record
	}
	if i.seq < 1 {
		i.seq = 1
	}
}

func (i *Index) Root() string { return i.root }

// Add writes a new job's state and takes it into the index.
func (i *Index) Add(record Record) error {
	i.guard.Lock()
	defer i.guard.Unlock()
	if _, taken := i.records[record.ID]; taken {
		return fmt.Errorf("a job called %s is already here", record.ID)
	}
	// The check that counts: two jobs asked for together both passed the one in
	// Reserve, and only one of them may pass this.
	if err := i.overQuota(time.Now()); err != nil {
		return err
	}
	if record.Seq == 0 {
		record.Seq = i.seq
		i.seq++
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

// FindKey is the job the asker already made with this request id, so that a send made twice is one job. An id is the
// asker's own: the same words from somebody else are another request.
func (i *Index) FindKey(asked, key string) (Record, bool) {
	if key == "" {
		return Record{}, false
	}
	i.guard.Lock()
	defer i.guard.Unlock()
	for _, record := range i.records {
		if record.Key == key && record.Asked == asked {
			return record, true
		}
	}
	return Record{}, false
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
	now := i.running()
	var oldest *Record
	for id := range i.records {
		record := i.records[id]
		if record.State != Queued {
			continue
		}
		if _, held := i.hold(record, now); held {
			continue
		}
		if oldest == nil || record.Seq < oldest.Seq {
			taken := record
			oldest = &taken
		}
	}
	if oldest == nil {
		return Record{}, false, nil
	}
	oldest.State = Running
	oldest.Started = Now()
	i.lastStarted = oldest.Started
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

// ErrNotStoppable is the answer to stopping a job that is already over.
var ErrNotStoppable = fmt.Errorf("that job is already over")

// Stop marks a job as stopped by somebody. A job that is still queued is over at once, interrupted, and never runs. A job
// that is running is marked, and ends interrupted when its work returns, which the queue arranges after it has told the
// work to stop. It gives the job as it stands, and whether it was running.
func (i *Index) Stop(id, by string) (record Record, wasRunning bool, err error) {
	i.guard.Lock()
	defer i.guard.Unlock()
	record, found := i.records[id]
	if !found {
		return Record{}, false, fmt.Errorf("no job called %s", id)
	}
	if record.State.Done() {
		return record, false, ErrNotStoppable
	}
	wasRunning = record.State == Running
	record.StoppedBy = by
	if !wasRunning {
		record.State, record.Exit, record.Ended = Interrupted, -1, Now()
	}
	if err := write(record.Dir(i.root), record); err != nil {
		return Record{}, false, err
	}
	i.records[id] = record
	if !wasRunning {
		i.finished[Interrupted]++
		i.lastFinished = record.Ended
	}
	return record, wasRunning, nil
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
	i.finished[state]++
	i.lastFinished = record.Ended
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
	if manifest.Gate == nil {
		// A turn in a conversation has no verdict. It ran, and that is all the
		// evidence claims.
		record.State = Passed
		record.Exit = 0
		return record
	}
	record.Exit = manifest.Gate.Code
	record.State = Failed
	if manifest.Gate.Passed {
		record.State = Passed
	}
	return record
}
