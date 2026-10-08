package mirror

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// State is what a pair of folders remembers between runs: what both sides agreed on, which is the picture a change is told
// from, and the hashes of files by their size and time, so a folder that did not change is not read again.
type State struct {
	// Base is what both sides held when they last agreed.
	Base Snapshot `json:"base"`
	// Local and Remote are hashes of files by what the file was when it was hashed.
	Local  map[string]Stamp `json:"local"`
	Remote map[string]Stamp `json:"remote"`

	path string
}

// Stamp is a hash and the size and time of the file it was taken from. Two times of the same file at the same size are the
// same file, which is how a check that is cheap tells it from one that has to be read again.
type Stamp struct {
	Hash       string `json:"hash"`
	Size       int64  `json:"size"`
	ModifiedNs int64  `json:"modifiedNs"`
}

// OpenState reads the record kept at path, or starts an empty one. A record that cannot be read is an error and not an empty
// record: starting again from nothing would take every file on both sides for new, and a deleted one for never having been.
func OpenState(path string) (*State, error) {
	state := &State{Base: Snapshot{}, Local: map[string]Stamp{}, Remote: map[string]Stamp{}, path: path}
	text, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(text, state); err != nil {
		return nil, errors.New(path + " is not a record this program wrote: move it away to start these folders again from nothing")
	}
	if state.Base == nil {
		state.Base = Snapshot{}
	}
	if state.Local == nil {
		state.Local = map[string]Stamp{}
	}
	if state.Remote == nil {
		state.Remote = map[string]Stamp{}
	}
	state.path = path
	return state, nil
}

// Fresh says whether the record has never held an agreement, which is a first run.
func (s *State) Fresh() bool { return len(s.Base) == 0 }

// Save writes the record whole, and moves it into place.
func (s *State) Save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	text, err := json.Marshal(s)
	if err != nil {
		return err
	}
	scratch := s.path + ".new"
	if err := os.WriteFile(scratch, text, 0o600); err != nil {
		return err
	}
	return os.Rename(scratch, s.path)
}
