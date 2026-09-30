package cloud

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"time"
)

// Lease is how long a demo host has left. The host writes the file when it starts and
// removes itself when the deadline passes, so a demo that is forgotten stops costing
// money. Anybody signed in at the front door can extend it, up to a limit that no
// extension passes.
type Lease struct {
	path string
	step time.Duration
	now  func() time.Time
}

// leaseFile is what is kept: when the host goes, and the latest it may ever go.
type leaseFile struct {
	Deadline int64 `json:"deadline"`
	HardMax  int64 `json:"hardMax"`
}

// ErrLeaseAtMaximum says the host has as long as it may ever have.
var ErrLeaseAtMaximum = errors.New("this host has as long as it may have")

// OpenLease is the lease in a file, and how much each extension adds.
func OpenLease(path string, step time.Duration) *Lease {
	return &Lease{path: path, step: step, now: time.Now}
}

// Left is what a lease has left, and whether it can be extended again.
type Left struct {
	Remaining time.Duration
	Ends      time.Time
	CanExtend bool
}

func (l *Lease) read() (leaseFile, error) {
	text, err := os.ReadFile(l.path)
	if err != nil {
		return leaseFile{}, err
	}
	var held leaseFile
	if err := json.Unmarshal(text, &held); err != nil || held.Deadline == 0 {
		return leaseFile{}, fmt.Errorf("%s does not hold a lease", l.path)
	}
	return held, nil
}

// Status is what the lease has left. A host with no lease file has no lease, which is
// fs.ErrNotExist.
func (l *Lease) Status() (Left, error) {
	held, err := l.read()
	if err != nil {
		return Left{}, err
	}
	ends := time.Unix(held.Deadline, 0)
	return Left{Remaining: max(ends.Sub(l.now()), 0), Ends: ends, CanExtend: held.HardMax == 0 || held.Deadline < held.HardMax}, nil
}

// Extend adds a step to what is left, or to now when it has run out, and never past the
// latest the host may go.
func (l *Lease) Extend() (Left, error) {
	held, err := l.read()
	if err != nil {
		return Left{}, err
	}
	if held.HardMax != 0 && held.Deadline >= held.HardMax {
		return Left{}, ErrLeaseAtMaximum
	}
	from := max(held.Deadline, l.now().Unix())
	held.Deadline = from + int64(l.step/time.Second)
	if held.HardMax != 0 {
		held.Deadline = min(held.Deadline, held.HardMax)
	}
	if err := writeFile(l.path, held, 0o644); err != nil {
		return Left{}, err
	}
	return l.Status()
}

// leaseJSON is the time left, for the script on every page that shows the banner. It says
// nothing else, and answers not found when the front door has no lease.
func (s *Server) leaseJSON(w http.ResponseWriter, r *http.Request) {
	if s.Lease == nil {
		http.NotFound(w, r)
		return
	}
	left, err := s.Lease.Status()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintf(w, `{"seconds":%d}`+"\n", int(left.Remaining/time.Second))
}

// leasePage says how long the host has and lets a person sign in and extend it.
func (s *Server) leasePage(w http.ResponseWriter, r *http.Request, person Person) {
	s.drawLease(w, http.StatusOK, "")
}

func (s *Server) drawLease(w http.ResponseWriter, status int, notice string) {
	if s.Lease == nil {
		http.Error(w, "this host has no lease", http.StatusNotFound)
		return
	}
	left, err := s.Lease.Status()
	if errors.Is(err, fs.ErrNotExist) || err != nil {
		http.Error(w, "this host has no lease", http.StatusNotFound)
		return
	}
	s.render(w, status, "lease", view{Title: "This demo host", Notice: notice, Left: left, Step: s.Lease.step})
}

func (s *Server) extendLease(w http.ResponseWriter, r *http.Request, person Person) {
	if s.Lease == nil {
		http.NotFound(w, r)
		return
	}
	if _, err := s.Lease.Extend(); err != nil {
		notice := "The lease could not be extended."
		if errors.Is(err, ErrLeaseAtMaximum) {
			notice = "This host has as long as it may have."
		}
		s.drawLease(w, http.StatusConflict, notice)
		return
	}
	http.Redirect(w, r, "/lease", http.StatusSeeOther)
}
