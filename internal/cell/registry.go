package cell

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Registration is a cell saying it is there: where its agent is, and the address on the private
// network its organisations' servers bind to. It is written again every few seconds, and a
// cell that has not been heard from lately is not one that can be placed on.
type Registration struct {
	Name    string `json:"name"`
	Agent   string `json:"agent"`
	Private string `json:"private"`
	Beat    int64  `json:"beat"`
}

var nameShape = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,30}$`)

// ValidName is what a cell may be called, which is also what names its file.
func ValidName(name string) bool { return nameShape.MatchString(name) && name != "orchestrator" }

func file(dir, name string) string { return filepath.Join(dir, name+".json") }

// Register writes a cell's registration, whole or not at all.
func Register(dir string, r Registration) error {
	if !ValidName(r.Name) {
		return fmt.Errorf("%q is not a name a cell may have", r.Name)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	text, err := json.Marshal(r)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".register-")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(append(text, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), file(dir, r.Name))
}

// Registered is the cell with that name, and how long ago it was last heard from.
func Registered(dir, name string, now time.Time) (Registration, time.Duration, error) {
	if !ValidName(name) {
		return Registration{}, 0, fs.ErrNotExist
	}
	text, err := os.ReadFile(file(dir, name))
	if err != nil {
		return Registration{}, 0, err
	}
	var r Registration
	if err := json.Unmarshal(text, &r); err != nil || r.Name != name || r.Agent == "" || r.Private == "" {
		return Registration{}, 0, fmt.Errorf("%s is not a registration", file(dir, name))
	}
	return r, max(now.Sub(time.Unix(r.Beat, 0)), 0), nil
}

// Fresh is every cell heard from within maxAge, in name order.
func Fresh(dir string, now time.Time, maxAge time.Duration) ([]Registration, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var found []Registration
	for _, entry := range entries {
		name, isJSON := strings.CutSuffix(entry.Name(), ".json")
		if !isJSON || !ValidName(name) {
			continue
		}
		if r, age, err := Registered(dir, name, now); err == nil && age <= maxAge {
			found = append(found, r)
		}
	}
	return found, nil
}

const orchestratorBeat = "orchestrator.beat"

// Beat says the front door is there. A cell whose front door has not said so for a long
// while has nobody to work for, which the host's own timer acts on.
func Beat(dir string, now time.Time) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, orchestratorBeat)
	if err := os.WriteFile(path, []byte(now.UTC().Format(time.RFC3339)+"\n"), 0o644); err != nil {
		return err
	}
	return os.Chtimes(path, now, now)
}
