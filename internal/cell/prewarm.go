package cell

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/getlatentic/gusto/internal/atomicfile"
)

// expected is how long after a visitor begins asking for a demo a host is started for it before it waits: the time it takes
// to send the form, and a little more.
const expected = 2 * time.Minute

// Prewarm says a visitor has begun asking for a demo, so a host is started now, while they finish, if none is up or on its
// way. It never blocks.
func (s *Scaler) Prewarm() {
	s.guard.Lock()
	s.expectUntil = s.clock().Add(expected)
	s.guard.Unlock()
	s.Wake()
}

// expecting says a host should be started for a visitor about to wait, and counts it against the day's prewarms. It is
// asked only when nothing waits, no host is up and none is starting.
func (s *Scaler) expecting() bool {
	s.guard.Lock()
	expected := s.clock().Before(s.expectUntil)
	s.guard.Unlock()
	if !expected || !s.countPrewarm() {
		return false
	}
	s.guard.Lock()
	s.expectUntil = time.Time{}
	s.guard.Unlock()
	return true
}

// prewarmsFile is the day's count of prewarms, beside the registrations, so a front door that starts again, or another that
// leads, counts on from it. It is not named as a registration is.
const prewarmsFile = "prewarms"

type prewarms struct {
	Day   string `json:"day"`
	Count int    `json:"count"`
}

// countPrewarm counts one more prewarm today, and says whether the day allows it. A count that cannot be read or kept allows
// none, since it is what bounds what visitors who never send the form can cost.
func (s *Scaler) countPrewarm() bool {
	path := filepath.Join(s.Pool.Dir, prewarmsFile)
	held := prewarms{}
	text, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.Error("the day's prewarms could not be read, so none is started", "error", err)
		return false
	}
	if err == nil && json.Unmarshal(text, &held) != nil {
		slog.Warn("the day's prewarms were not a count, so the count starts again", "file", path)
	}
	if day := s.clock().UTC().Format(time.DateOnly); held.Day != day {
		held = prewarms{Day: day}
	}
	if held.Count >= s.Prewarms {
		return false
	}
	held.Count++
	if err := writePrewarms(path, held); err != nil {
		slog.Error("the day's prewarms could not be kept, so none is started", "error", err)
		return false
	}
	return true
}

func writePrewarms(path string, held prewarms) error {
	text, err := json.Marshal(held)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return atomicfile.WriteFile(path, append(text, '\n'), 0o644)
}
