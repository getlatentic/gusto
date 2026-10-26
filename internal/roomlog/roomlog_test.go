package roomlog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func newSet(t *testing.T, opts Options) (*Set, string) {
	t.Helper()
	dir := t.TempDir()
	return NewSet(func(room string) string { return filepath.Join(dir, room+".log") }, opts), dir
}

func mustAppend(t *testing.T, s *Set, room, kind, ref string) Event {
	t.Helper()
	event, err := s.Append(room, Event{Kind: kind, Thread: "1", Ref: ref})
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func TestSequencesAreOrderedFromOne(t *testing.T) {
	s, _ := newSet(t, Options{})
	for want := uint64(1); want <= 5; want++ {
		if got := mustAppend(t, s, "r", "turn", "a").Seq; got != want {
			t.Fatalf("event %d got sequence %d", want, got)
		}
	}
}

func TestSequencesAndEpochSurviveARestart(t *testing.T) {
	s, dir := newSet(t, Options{})
	for range 7 {
		mustAppend(t, s, "r", "turn", "a")
	}
	before, _ := s.Head("r")

	again := NewSet(func(room string) string { return filepath.Join(dir, room+".log") }, Options{})
	after, err := again.Head("r")
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("head after a restart is %+v, was %+v", after, before)
	}
	if next := mustAppend(t, again, "r", "note", "n1"); next.Seq != 8 {
		t.Fatalf("the event after a restart got sequence %d, want 8", next.Seq)
	}
	slice, _ := again.After("r", 5, 0)
	if len(slice.Events) != 3 || slice.Events[0].Seq != 6 || slice.Gap {
		t.Fatalf("replay after a restart: %+v", slice)
	}
}

func TestAfterGivesExactlyWhatWasMissed(t *testing.T) {
	s, _ := newSet(t, Options{})
	for i := range 10 {
		mustAppend(t, s, "r", "turn", fmt.Sprint(i))
	}
	slice, err := s.After("r", 4, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(slice.Events) != 6 || slice.Events[0].Seq != 5 || slice.Events[5].Seq != 10 || slice.Gap {
		t.Fatalf("after 4: %+v", slice)
	}
	if caught, _ := s.After("r", 10, 0); len(caught.Events) != 0 || caught.Gap {
		t.Fatalf("a reader at the head is not behind: %+v", caught)
	}
	if limited, _ := s.After("r", 0, 3); len(limited.Events) != 3 || limited.Events[2].Seq != 3 {
		t.Fatalf("a limit cuts the replay short: %+v", limited)
	}
}

func TestAReaderBeyondTheWindowIsToldItMustStartAgain(t *testing.T) {
	s, _ := newSet(t, Options{Window: 4})
	for range 20 {
		mustAppend(t, s, "r", "turn", "a")
	}
	slice, _ := s.After("r", 3, 0)
	if !slice.Gap || len(slice.Events) != 0 {
		t.Fatalf("a cursor from before the window should be a gap: %+v", slice)
	}
	if slice.Head.Seq != 20 {
		t.Fatalf("head is %d, want 20", slice.Head.Seq)
	}
	if inside, _ := s.After("r", 17, 0); inside.Gap || len(inside.Events) != 3 {
		t.Fatalf("a cursor inside the window replays: %+v", inside)
	}
	if ahead, _ := s.After("r", 99, 0); !ahead.Gap {
		t.Fatalf("a cursor beyond the head is from another stream: %+v", ahead)
	}
}

func TestTheFileIsCutDownToTheWindowAndSurvivesIt(t *testing.T) {
	s, dir := newSet(t, Options{Window: 5})
	for range 40 {
		mustAppend(t, s, "r", "turn", "a")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "r.log"))
	if err != nil {
		t.Fatal(err)
	}
	lines := 0
	for _, b := range raw {
		if b == '\n' {
			lines++
		}
	}
	if lines > 1+2*5 {
		t.Fatalf("the file holds %d lines, more than the header and twice the window", lines)
	}
	again := NewSet(func(room string) string { return filepath.Join(dir, room+".log") }, Options{Window: 5})
	if head, _ := again.Head("r"); head.Seq != 40 {
		t.Fatalf("head after compaction and restart is %d, want 40", head.Seq)
	}
}

func TestATornLastLineIsDropped(t *testing.T) {
	s, dir := newSet(t, Options{})
	for range 3 {
		mustAppend(t, s, "r", "turn", "a")
	}
	file, err := os.OpenFile(filepath.Join(dir, "r.log"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.WriteString(`{"s":4,"t":1,"k":"tu`)
	file.Close()

	again := NewSet(func(room string) string { return filepath.Join(dir, room+".log") }, Options{})
	if head, _ := again.Head("r"); head.Seq != 3 {
		t.Fatalf("head with a torn line is %d, want 3", head.Seq)
	}
	if event := mustAppend(t, again, "r", "turn", "b"); event.Seq != 4 {
		t.Fatalf("the next event got %d, want 4", event.Seq)
	}
	third := NewSet(func(room string) string { return filepath.Join(dir, room+".log") }, Options{})
	if head, _ := third.Head("r"); head.Seq != 4 {
		t.Fatalf("the repaired file reads back to %d, want 4", head.Seq)
	}
}

func TestALostLogStartsANewEpoch(t *testing.T) {
	s, dir := newSet(t, Options{})
	mustAppend(t, s, "r", "turn", "a")
	first, _ := s.Head("r")
	if err := os.Remove(filepath.Join(dir, "r.log")); err != nil {
		t.Fatal(err)
	}
	again := NewSet(func(room string) string { return filepath.Join(dir, room+".log") }, Options{})
	second, _ := again.Head("r")
	if second.Epoch == first.Epoch || second.Seq != 0 {
		t.Fatalf("a lost log should start again under a new epoch: %+v then %+v", first, second)
	}
}

func TestAnUnwritableLogKeepsServingUnderANewEpoch(t *testing.T) {
	dir := t.TempDir()
	s := NewSet(func(room string) string { return filepath.Join(dir, "gone", room+".log") }, Options{})
	event, err := s.Append("r", Event{Kind: "turn"})
	if err == nil {
		t.Fatal("an unwritable log should say so")
	}
	if event.Seq != 1 {
		t.Fatalf("the event is still sequenced in memory, got %d", event.Seq)
	}
	if slice, _ := s.After("r", 0, 0); len(slice.Events) != 1 {
		t.Fatal("the event should still be served from memory")
	}
}

func TestRoomsAreEvictedAndReadBackWithoutLosingTheirSequence(t *testing.T) {
	s, _ := newSet(t, Options{MaxLoaded: 2})
	for _, room := range []string{"a", "b", "c", "d"} {
		for range 3 {
			mustAppend(t, s, room, "turn", "x")
		}
	}
	for _, room := range []string{"a", "b", "c", "d"} {
		if head, _ := s.Head(room); head.Seq != 3 {
			t.Fatalf("room %s head is %d after eviction, want 3", room, head.Seq)
		}
	}
	s.mu.Lock()
	loaded := len(s.loaded)
	s.mu.Unlock()
	if loaded > 2 {
		t.Fatalf("%d logs are held, the bound is 2", loaded)
	}
}

func TestConcurrentAppendsGetDistinctOrderedSequences(t *testing.T) {
	s, dir := newSet(t, Options{Window: 2000})
	const writers, each = 16, 100
	var wg sync.WaitGroup
	seen := make(chan uint64, writers*each)
	for range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range each {
				event, err := s.Append("r", Event{Kind: "turn"})
				if err != nil {
					t.Error(err)
					return
				}
				seen <- event.Seq
			}
		}()
	}
	wg.Wait()
	close(seen)
	unique := map[uint64]bool{}
	for seq := range seen {
		if unique[seq] {
			t.Fatalf("sequence %d handed out twice", seq)
		}
		unique[seq] = true
	}
	if len(unique) != writers*each {
		t.Fatalf("%d sequences, want %d", len(unique), writers*each)
	}
	again := NewSet(func(room string) string { return filepath.Join(dir, room+".log") }, Options{Window: 2000})
	slice, _ := again.After("r", 0, 0)
	for i, event := range slice.Events {
		if event.Seq != uint64(i+1) {
			t.Fatalf("the file has event %d at place %d", event.Seq, i)
		}
	}
}

func TestDropRemovesTheLog(t *testing.T) {
	s, dir := newSet(t, Options{})
	mustAppend(t, s, "r", "turn", "a")
	if err := s.Drop("r"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "r.log")); !os.IsNotExist(err) {
		t.Fatalf("the file should be gone: %v", err)
	}
}
