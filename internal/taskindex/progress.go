package taskindex

import "sync"

// Status is how far the index has caught up with the files. A page shows it while Building is true, so
// that an index still being filled is not read as a server with few tasks.
type Status struct {
	Building bool `json:"building"`
	Done     int  `json:"done"`
	Total    int  `json:"total"`
}

// Progress is how far a rebuild has got, for a page to read while it runs. The zero value is ready, and a
// nil *Progress is a rebuild nobody watches.
type Progress struct {
	// Changed, when set, is called with the new status each time it changes.
	Changed func(Status)

	mu     sync.Mutex
	status Status
}

// Status is the progress now.
func (p *Progress) Status() Status {
	if p == nil {
		return Status{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.status
}

// Set records the progress.
func (p *Progress) Set(status Status) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.status = status
	changed := p.Changed
	p.mu.Unlock()
	if changed != nil {
		changed(status)
	}
}
