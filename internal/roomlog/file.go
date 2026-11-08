package roomlog

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"
	"time"
)

// A log's file is one JSON object to a line. The first is the header, which names the epoch; each after it is an event. The
// file is appended to, and rewritten with only the window in it once it holds twice that many events.

const formatVersion = 1

type header struct {
	Epoch   string `json:"epoch"`
	Version int    `json:"v"`
}

// roomLog is one room's window of events and the file behind it. mu orders appends with each other and with reads.
type roomLog struct {
	path   string
	window int

	mu     sync.Mutex
	epoch  string
	head   uint64
	events []Event
	// lines is how many events the file holds, which is more than events does until the next rewrite.
	lines int
	// broken says the file could not be written, so the next append rewrites it whole.
	broken bool
}

// openLog reads a room's log from its file. A file that is missing or cannot be read starts a new epoch, and a torn last
// line, the mark of a write a crash cut short, is dropped.
func openLog(path string, window int) (*roomLog, error) {
	l := &roomLog{path: path, window: window}
	events, epoch, clean, err := readFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist) || epoch == "":
		l.epoch = newEpoch()
		l.broken = true
	case err != nil:
		return nil, err
	default:
		l.epoch, l.events, l.lines = epoch, events, len(events)
		l.broken = !clean
	}
	if n := len(l.events); n > 0 {
		l.head = l.events[n-1].Seq
	}
	l.trim()
	if l.broken {
		l.broken = l.rewrite() != nil
	}
	return l, nil
}

func newEpoch() string {
	var word [8]byte
	if _, err := rand.Read(word[:]); err != nil {
		return fmt.Sprintf("t%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(word[:])
}

// readFile gives the epoch and events in a log's file, and whether the file ended cleanly.
func readFile(path string) (events []Event, epoch string, clean bool, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, "", false, err
	}
	clean = len(raw) == 0 || raw[len(raw)-1] == '\n'
	lines := bufio.NewScanner(bytes.NewReader(raw))
	lines.Buffer(make([]byte, 0, 4096), 64<<10)
	first := true
	for lines.Scan() {
		if first {
			first = false
			var head header
			if json.Unmarshal(lines.Bytes(), &head) != nil || head.Epoch == "" {
				return nil, "", false, nil
			}
			epoch = head.Epoch
			continue
		}
		var event Event
		if json.Unmarshal(lines.Bytes(), &event) != nil || event.Seq == 0 {
			clean = false
			break
		}
		events = append(events, event)
	}
	return events, epoch, clean && lines.Err() == nil, nil
}

// trim keeps the newest window events.
func (l *roomLog) trim() {
	if extra := len(l.events) - l.window; extra > 0 {
		l.events = append(l.events[:0:0], l.events[extra:]...)
	}
}

func (l *roomLog) append(event Event) (Event, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.head++
	event.Seq = l.head
	if event.At == 0 {
		event.At = time.Now().Unix()
	}
	l.events = append(l.events, event)
	if len(l.events) > l.window*2 {
		l.trim()
	}
	return event, l.persist(event)
}

// persist puts an event in the file. When the file cannot be written the epoch changes, because the numbers handed out from
// here on are not on disk and a restart would hand them out again.
func (l *roomLog) persist(event Event) error {
	if !l.broken && l.lines >= l.window*2 {
		l.broken = true
	}
	if l.broken {
		return l.recover()
	}
	line, err := json.Marshal(event)
	if err == nil {
		err = appendLine(l.path, append(line, '\n'))
	}
	if err != nil {
		l.broken, l.epoch = true, newEpoch()
		return fmt.Errorf("a room's event could not be written: %w", err)
	}
	l.lines++
	return nil
}

// recover rewrites the file from the window, which is also how the file is cut down to it.
func (l *roomLog) recover() error {
	l.trim()
	if err := l.rewrite(); err != nil {
		l.epoch = newEpoch()
		return fmt.Errorf("a room's event log could not be written: %w", err)
	}
	return nil
}

func (l *roomLog) rewrite() error {
	var out bytes.Buffer
	head, _ := json.Marshal(header{Epoch: l.epoch, Version: formatVersion})
	out.Write(head)
	out.WriteByte('\n')
	for _, event := range l.events {
		line, err := json.Marshal(event)
		if err != nil {
			return err
		}
		out.Write(line)
		out.WriteByte('\n')
	}
	if err := replaceFile(l.path, out.Bytes()); err != nil {
		return err
	}
	l.broken, l.lines = false, len(l.events)
	return nil
}

func (l *roomLog) position() Position {
	l.mu.Lock()
	defer l.mu.Unlock()
	return Position{Epoch: l.epoch, Seq: l.head}
}

func (l *roomLog) after(seq uint64, limit int) Slice {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := Slice{Head: Position{Epoch: l.epoch, Seq: l.head}}
	if seq >= l.head {
		out.Gap = seq > l.head
		return out
	}
	// The newest events are at the end, so the first one above seq is found from there.
	first := len(l.events)
	for first > 0 && l.events[first-1].Seq > seq {
		first--
	}
	if first == len(l.events) || l.events[first].Seq != seq+1 {
		out.Gap = true
		return out
	}
	rest := l.events[first:]
	if limit > 0 && len(rest) > limit {
		rest = rest[:limit]
	}
	out.Events = append([]Event(nil), rest...)
	return out
}
