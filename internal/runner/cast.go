package runner

import (
	"encoding/json"
	"os"
	"sync"
	"time"
	"unicode/utf8"
)

// cast records a terminal as asciicast v2: a header line, then one line an
// event, each the seconds since the start, "o" for what the screen showed or
// "i" for what somebody typed, and the bytes. Any player replays it, and it is
// text, so the manifest hashes it like the rest of the evidence.
//
// What was typed is recorded too. A person who takes over a job acts in it,
// and the record of a run says who did what.
type cast struct {
	guard   sync.Mutex
	file    *os.File
	started time.Time
	// pending is the start of a character a read cut in half, held until the
	// rest arrives, because an event's text has to be whole characters.
	pending map[string][]byte
}

func openCast(path string, cols, rows int) (*cast, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	header, err := json.Marshal(map[string]any{
		"version": 2, "width": cols, "height": rows, "timestamp": started.Unix(),
		"env": map[string]string{"TERM": "xterm-256color"},
	})
	if err != nil {
		return nil, err
	}
	if _, err := file.Write(append(header, '\n')); err != nil {
		return nil, err
	}
	return &cast{file: file, started: started, pending: map[string][]byte{}}, nil
}

func (c *cast) event(kind string, data []byte) error {
	c.guard.Lock()
	defer c.guard.Unlock()
	whole := append(c.pending[kind], data...)
	cut := len(whole)
	if last := lastStart(whole); !utf8.FullRune(whole[last:]) {
		cut = last
	}
	c.pending[kind] = append([]byte(nil), whole[cut:]...)
	if cut == 0 {
		return nil
	}
	line, err := json.Marshal([]any{time.Since(c.started).Seconds(), kind, string(whole[:cut])})
	if err != nil {
		return err
	}
	_, err = c.file.Write(append(line, '\n'))
	return err
}

// lastStart is where the last character in text begins.
func lastStart(text []byte) int {
	for i := len(text) - 1; i >= 0; i-- {
		if utf8.RuneStart(text[i]) {
			return i
		}
	}
	return 0
}

func (c *cast) Close() error { return c.file.Close() }

// recorded is one side of a terminal, as a writer that records what passes.
type recorded struct {
	cast *cast
	kind string
}

func (r recorded) Write(data []byte) (int, error) {
	return len(data), r.cast.event(r.kind, data)
}
