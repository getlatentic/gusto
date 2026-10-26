package instance

import (
	"fmt"
	"sync"
	"testing"
)

func eventsOf(t *testing.T, store *Store, room string) []string {
	t.Helper()
	slice, err := store.RoomEvents(room, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, event := range slice.Events {
		kinds = append(kinds, fmt.Sprintf("%d:%s:%s", event.Seq, event.Kind, event.Ref))
	}
	return kinds
}

func TestAChangeToARoomIsAnEventWithTheNextSequence(t *testing.T) {
	store := opened(t)
	room, _ := store.StartRoom("Launch plan", "ada@example.com")
	thread := store.Latest(room.ID)

	if err := store.Said(room.ID, thread.ID, "job-1", ""); err != nil {
		t.Fatal(err)
	}
	note, _ := store.Note(room.ID, thread.ID, "ada@example.com", "hello")
	if err := store.SetMember(room.ID, "bob@example.com", RoleMember); err != nil {
		t.Fatal(err)
	}
	store.Record(room.ID, thread.ID, KindTurn, "job-1")

	got := eventsOf(t, store, room.ID)
	want := []string{"1:turn:job-1", "2:note:" + note.ID, "3:people:", "4:turn:job-1"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("events %v, want %v", got, want)
	}
}

func TestAnInstanceThatIsNotARoomHasNoEvents(t *testing.T) {
	store := opened(t)
	held, _ := store.Start("20261026-1200-work", Instance{Harness: "opencode"})
	thread := store.Latest(held.ID)
	_ = store.Said(held.ID, thread.ID, "job-1", "")
	if _, err := store.RoomEvents(held.ID, 0, 0); err != ErrNoRoom {
		t.Fatalf("a private conversation has no room log, got %v", err)
	}
}

func TestSequencesSurviveAReopenedStore(t *testing.T) {
	root := t.TempDir()
	store, _ := Open(root)
	room, _ := store.StartRoom("Launch plan", "ada@example.com")
	thread := store.Latest(room.ID)
	for i := range 5 {
		_ = store.Said(room.ID, thread.ID, fmt.Sprintf("job-%d", i), "")
	}
	before, _ := store.RoomHead(room.ID)

	again, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := again.RoomHead(room.ID)
	if after != before || after.Seq != 5 {
		t.Fatalf("head across a restart: %+v then %+v", before, after)
	}
	_ = again.Said(room.ID, thread.ID, "job-5", "")
	slice, _ := again.RoomEvents(room.ID, 4, 0)
	if len(slice.Events) != 2 || slice.Events[0].Seq != 5 || slice.Events[1].Seq != 6 {
		t.Fatalf("replay across a restart: %+v", slice)
	}
}

func TestConcurrentChangesAreSequencedInTheOrderTheyWereMade(t *testing.T) {
	store := opened(t)
	room, _ := store.StartRoom("Launch plan", "ada@example.com")
	thread := store.Latest(room.ID)

	const speakers, each = 8, 40
	var wg sync.WaitGroup
	for s := range speakers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range each {
				_ = store.Said(room.ID, thread.ID, fmt.Sprintf("job-%d-%d", s, i), "")
			}
		}()
	}
	wg.Wait()

	slice, _ := store.RoomEvents(room.ID, 0, 0)
	held, _ := store.Thread(room.ID, thread.ID)
	if len(slice.Events) != speakers*each || len(held.Turns) != speakers*each {
		t.Fatalf("%d events and %d turns, want %d", len(slice.Events), len(held.Turns), speakers*each)
	}
	for i, event := range slice.Events {
		if event.Seq != uint64(i+1) || event.Ref != held.Turns[i] {
			t.Fatalf("event %d is %s #%d, but the turn in that place is %s", i, event.Ref, event.Seq, held.Turns[i])
		}
	}
}

func TestWatchersAreToldOfEachEventAndOfARemovedRoom(t *testing.T) {
	store := opened(t)
	var mu sync.Mutex
	var told []string
	store.OnRoomEvent(func(room string) {
		mu.Lock()
		told = append(told, room)
		mu.Unlock()
	})
	room, _ := store.StartRoom("Launch plan", "ada@example.com")
	thread := store.Latest(room.ID)
	_ = store.Said(room.ID, thread.ID, "job-1", "")
	if err := store.Remove(room.ID); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(told) != 2 || told[0] != room.ID || told[1] != room.ID {
		t.Fatalf("watchers were told %v", told)
	}
	if _, err := store.RoomHead(room.ID); err != ErrNoRoom {
		t.Fatalf("a removed room has no stream, got %v", err)
	}
}
