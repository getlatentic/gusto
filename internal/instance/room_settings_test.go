package instance

import (
	"os"
	"strings"
	"testing"
)

func settingsRoom(t *testing.T) (*Store, Instance) {
	t.Helper()
	store := opened(t)
	room, err := store.StartRoom("Launch plan", "ada@example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetMember(room.ID, "bob@example.com", RoleMember); err != nil {
		t.Fatal(err)
	}
	return store, room
}

func TestGuidelinesAreKeptTrimmedAndCapped(t *testing.T) {
	store, room := settingsRoom(t)
	if err := store.SetGuidelines(room.ID, "  Answer in French.\r\nBe brief.  "); err != nil {
		t.Fatal(err)
	}
	held, _ := store.Get(room.ID)
	if held.Guidelines != "Answer in French.\nBe brief." {
		t.Errorf("guidelines are %q", held.Guidelines)
	}
	if err := store.SetGuidelines(room.ID, strings.Repeat("é", MaxGuidelines)); err != nil {
		t.Errorf("guidelines at the cap were refused: %v", err)
	}
	if err := store.SetGuidelines(room.ID, strings.Repeat("é", MaxGuidelines+1)); err == nil {
		t.Error("guidelines over the cap were kept")
	}
	if err := store.SetGuidelines(room.ID, ""); err != nil {
		t.Fatal(err)
	}
	if held, _ = store.Get(room.ID); held.Guidelines != "" {
		t.Errorf("cleared guidelines are %q", held.Guidelines)
	}
}

func TestMuteIsOnePersonsAndSurvivesTheirRoleChanging(t *testing.T) {
	store, room := settingsRoom(t)
	if err := store.SetMuted(room.ID, "Bob@Example.com", true); err != nil {
		t.Fatal(err)
	}
	held, _ := store.Get(room.ID)
	if !held.MutedBy("bob@example.com") || held.MutedBy("ada@example.com") {
		t.Fatalf("mute is not bob's alone: %+v", held.Members)
	}
	if err := store.SetMember(room.ID, "bob@example.com", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if held, _ = store.Get(room.ID); !held.MutedBy("bob@example.com") {
		t.Error("promoting bob unmuted the room for him")
	}
	if err := store.SetMuted(room.ID, "eve@example.com", true); err == nil {
		t.Error("somebody outside the room muted it")
	}
	reopened, err := Open(store.Root())
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := reopened.Get(room.ID); !again.MutedBy("bob@example.com") {
		t.Error("the mute was not kept across a restart")
	}
}

func TestArchivingIsTheRoomsAndUndoes(t *testing.T) {
	store, room := settingsRoom(t)
	if err := store.SetArchived(room.ID, true); err != nil {
		t.Fatal(err)
	}
	held, _ := store.Get(room.ID)
	if held.Archived == 0 {
		t.Fatal("the room is not archived")
	}
	if err := store.SetArchived(room.ID, false); err != nil {
		t.Fatal(err)
	}
	if held, _ = store.Get(room.ID); held.Archived != 0 {
		t.Error("the room is still archived")
	}
}

func TestAPersonsOwnConversationIsNotArchived(t *testing.T) {
	store := opened(t)
	room, err := store.StartDirect("ada@example.com", "", Agent{Name: "Claude", Harness: "claude", Member: "m1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetArchived(room.ID, true); err == nil {
		t.Error("a conversation of one's own was archived")
	}
}

// A change that leaves the room as it is writes nothing and announces nothing.
func TestSettingARoomAsItIsMakesNoEvent(t *testing.T) {
	store, room := settingsRoom(t)
	before := len(eventsOf(t, store, room.ID))
	_ = store.SetArchived(room.ID, false)
	_ = store.SetGuidelines(room.ID, "")
	_ = store.SetMuted(room.ID, "bob@example.com", false)
	if after := len(eventsOf(t, store, room.ID)); after != before {
		t.Errorf("%d events were added for no change", after-before)
	}
	if err := store.SetGuidelines(room.ID, "Be brief."); err != nil {
		t.Fatal(err)
	}
	if after := len(eventsOf(t, store, room.ID)); after != before+1 {
		t.Errorf("a change added %d events", after-before)
	}
}

// A write that fails is no change: the room in memory, and the log, stay as they were.
func TestASettingThatCouldNotBeWrittenIsNoChange(t *testing.T) {
	store, room := settingsRoom(t)
	if os.Geteuid() == 0 {
		t.Skip("root writes where it is told not to")
	}
	before := len(eventsOf(t, store, room.ID))
	dir := store.Dir(room.ID)
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	if err := store.SetGuidelines(room.ID, "Be brief."); err == nil {
		t.Error("a guideline write that failed was reported as done")
	}
	if err := store.SetArchived(room.ID, true); err == nil {
		t.Error("an archive write that failed was reported as done")
	}
	if err := store.SetMuted(room.ID, "bob@example.com", true); err == nil {
		t.Error("a mute write that failed was reported as done")
	}
	held, _ := store.Get(room.ID)
	if held.Guidelines != "" || held.Archived != 0 || held.MutedBy("bob@example.com") {
		t.Errorf("the room in memory is ahead of its file: %+v", held)
	}
	if after := len(eventsOf(t, store, room.ID)); after != before {
		t.Errorf("the log holds %d events for changes that did not happen", after-before)
	}
}
