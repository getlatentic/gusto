// Package inbox is how gusto cloud tells an organisation's server what an admin
// did, so that the server can put it in its change record.
//
// The change record has one writer, the organisation's server, because two
// processes appending to one chained file would fork it. Gusto cloud drops a
// file for each event in a directory, and the server takes them into its record
// in the order they were dropped and removes them.
package inbox

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/atomicfile"
)

// Item is one thing that happened: who did it, from where, what kind of change it
// was, and the details.
type Item struct {
	At   time.Time         `json:"at"`
	Who  string            `json:"who"`
	Role string            `json:"role,omitempty"`
	From string            `json:"from,omitempty"`
	Kind string            `json:"kind"`
	What map[string]string `json:"what,omitempty"`
}

// Waiting is an item that has not been taken yet, and the name it is known by. A
// file that is not an item has Err set, so the server can set it aside and not
// meet it on every pass.
type Waiting struct {
	ID   string
	Item Item
	Err  error
}

const suffix = ".json"

// Drop leaves an item for the server. Its name sorts by when it was dropped, and
// carries a random part so two dropped in the same instant are two files.
func Drop(dir string, item Item) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		return err
	}
	id := fmt.Sprintf("%020d-%s", time.Now().UnixNano(), hex.EncodeToString(random))
	return atomicfile.Write(filepath.Join(dir, id+suffix), item, 0o600)
}

// Pending is what is waiting, oldest first: a directory is read in the order of its
// names, and a name begins with when the item was dropped.
func Pending(dir string) ([]Waiting, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var waiting []Waiting
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, suffix) {
			continue
		}
		item, err := read(filepath.Join(dir, name))
		waiting = append(waiting, Waiting{ID: strings.TrimSuffix(name, suffix), Item: item, Err: err})
	}
	return waiting, nil
}

func read(path string) (Item, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return Item{}, err
	}
	var item Item
	if err := json.Unmarshal(text, &item); err != nil {
		return Item{}, err
	}
	if item.Kind == "" || item.Who == "" {
		return Item{}, errors.New("it names no kind of change, or nobody who made it")
	}
	return item, nil
}

// Done removes an item once the server has it.
func Done(dir, id string) error {
	if err := os.Remove(filepath.Join(dir, id+suffix)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// SetAside keeps a file that is not an item where a person can read it, and out of
// the way of the next pass.
func SetAside(dir, id string) error {
	return os.Rename(filepath.Join(dir, id+suffix), filepath.Join(dir, id+".rejected"))
}
