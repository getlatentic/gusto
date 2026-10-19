package instance

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Routine is a turn an instance takes on a schedule, in a thread of its own, so the agent remembers from one run to the
// next. It runs as any other turn: through the proxy, under the team's cap, and into the signed record.
type Routine struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Prompt string `json:"prompt"`
	// Daily is a time of day, HH:MM in the server's own zone, the routine runs at. Every is instead a number of hours
	// between runs. One of the two is set.
	Daily  string `json:"daily,omitempty"`
	Every  int    `json:"everyHours,omitempty"`
	Thread string `json:"thread"`
	// Who added it, and is told how each run went.
	Who    string `json:"who,omitempty"`
	Paused bool   `json:"paused,omitempty"`
	Added  int64  `json:"addedAt"`
	// Last is the latest slot: the turn it started, or why it started none.
	Last *Ran `json:"last,omitempty"`
}

// Ran is one slot of a routine.
type Ran struct {
	At      int64  `json:"at"`
	Job     string `json:"job,omitempty"`
	Skipped string `json:"skipped,omitempty"`
	// Told is the channels that have taken the news of how the run ended, Tries how many times it was offered and not all
	// taken it, and Notified that it is settled: taken by every channel, or given up on, with why in NotifyError.
	Told        []string `json:"told,omitempty"`
	Tries       int      `json:"tries,omitempty"`
	Notified    bool     `json:"notified,omitempty"`
	NotifyError string   `json:"notifyError,omitempty"`
}

// Schedule checks the routine's schedule: a time of day as HH:MM, or a number of hours, and not both.
func (r Routine) Schedule() error {
	switch {
	case r.Daily != "" && r.Every != 0:
		return errors.New("a routine runs daily at a time or every so many hours, not both")
	case r.Every < 0:
		return errors.New("every so many hours is a number above zero")
	case r.Every > 0:
		return nil
	case r.Daily == "":
		return errors.New("a routine needs a time of day or a number of hours")
	}
	if _, err := time.Parse("15:04", r.Daily); err != nil {
		return fmt.Errorf("%q is not a time of day as HH:MM", r.Daily)
	}
	return nil
}

// When says the schedule in words.
func (r Routine) When() string {
	if r.Every == 1 {
		return "every hour"
	}
	if r.Every > 1 {
		return fmt.Sprintf("every %d hours", r.Every)
	}
	return "daily at " + r.Daily
}

// Due says whether the routine's next slot has come. A daily routine's slot is today's time of day, and it is due once
// that time has passed and its last slot, or its adding, was before it. An hourly one is due that many hours after its
// last slot, and right away when it has had none.
func (r Routine) Due(now time.Time) bool {
	if r.Paused {
		return false
	}
	last := r.Added
	if r.Last != nil {
		last = r.Last.At
	}
	if r.Every > 0 {
		return r.Last == nil || now.Unix()-last >= int64(r.Every)*3600
	}
	at, err := time.Parse("15:04", r.Daily)
	if err != nil {
		return false
	}
	slot := time.Date(now.Year(), now.Month(), now.Day(), at.Hour(), at.Minute(), 0, 0, now.Location())
	return !now.Before(slot) && last < slot.Unix()
}

func (s *Store) routinesFile(id string) string { return filepath.Join(s.Dir(id), "routines.json") }

// Routines is an instance's routines, in the order they were added.
func (s *Store) Routines(id string) []Routine {
	s.guard.Lock()
	defer s.guard.Unlock()
	routines, _ := s.readRoutines(id)
	return routines
}

// AddRoutine opens a thread for the routine, named after it, and keeps the routine. The caller has checked the schedule.
func (s *Store) AddRoutine(id string, routine Routine) (Routine, error) {
	s.guard.Lock()
	defer s.guard.Unlock()
	if _, found := s.held[id]; !found {
		return Routine{}, fmt.Errorf("no instance called %s", id)
	}
	if err := routine.Schedule(); err != nil {
		return Routine{}, err
	}
	routines, err := s.readRoutines(id)
	if err != nil {
		return Routine{}, err
	}
	thread, err := s.open(id, routine.Title)
	if err != nil {
		return Routine{}, err
	}
	routine.ID = "r" + strconv.Itoa(nextRoutine(routines))
	routine.Thread = thread.ID
	routine.Added = time.Now().Unix()
	routine.Last = nil
	return routine, s.writeRoutines(id, append(routines, routine))
}

// PauseRoutine pauses or resumes a routine.
func (s *Store) PauseRoutine(id, routine string, paused bool) error {
	return s.changeRoutine(id, routine, func(r *Routine) { r.Paused = paused })
}

// RoutineRan records a routine's latest slot.
func (s *Store) RoutineRan(id, routine string, ran Ran) error {
	return s.changeRoutine(id, routine, func(r *Routine) { r.Last = &ran })
}

// NoteRun changes the latest slot of a routine, which is how the news of how it ended is kept.
func (s *Store) NoteRun(id, routine string, change func(*Ran)) error {
	return s.changeRoutine(id, routine, func(r *Routine) {
		if r.Last != nil {
			change(r.Last)
		}
	})
}

// RemoveRoutine forgets a routine. Its thread and its turns stay.
func (s *Store) RemoveRoutine(id, routine string) error {
	s.guard.Lock()
	defer s.guard.Unlock()
	routines, err := s.readRoutines(id)
	if err != nil {
		return err
	}
	kept := routines[:0]
	for _, r := range routines {
		if r.ID != routine {
			kept = append(kept, r)
		}
	}
	if len(kept) == len(routines) {
		return fmt.Errorf("no routine %s on %s", routine, id)
	}
	return s.writeRoutines(id, kept)
}

func (s *Store) changeRoutine(id, routine string, change func(*Routine)) error {
	s.guard.Lock()
	defer s.guard.Unlock()
	routines, err := s.readRoutines(id)
	if err != nil {
		return err
	}
	for i := range routines {
		if routines[i].ID == routine {
			change(&routines[i])
			return s.writeRoutines(id, routines)
		}
	}
	return fmt.Errorf("no routine %s on %s", routine, id)
}

func (s *Store) readRoutines(id string) ([]Routine, error) {
	text, err := os.ReadFile(s.routinesFile(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var routines []Routine
	if err := json.Unmarshal(text, &routines); err != nil {
		return nil, fmt.Errorf("routines.json of %s: %w", id, err)
	}
	return routines, nil
}

func (s *Store) writeRoutines(id string, routines []Routine) error {
	if len(routines) == 0 {
		err := os.Remove(s.routinesFile(id))
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return writeJSON(s.routinesFile(id), routines)
}

// nextRoutine is one past the highest number any routine has had, so a removed routine's id is not given again while its
// thread is still named after it.
func nextRoutine(routines []Routine) int {
	next := 1
	for _, r := range routines {
		if n, err := strconv.Atoi(strings.TrimPrefix(r.ID, "r")); err == nil && n >= next {
			next = n + 1
		}
	}
	return next
}
