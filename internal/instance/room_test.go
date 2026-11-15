package instance

import (
	"os"
	"slices"
	"testing"
)

func roomIDs(rooms []Instance) []string {
	ids := make([]string, 0, len(rooms))
	for _, room := range rooms {
		ids = append(ids, room.ID)
	}
	slices.Sort(ids)
	return ids
}

func TestARoomKeepsItsMembersAndAlwaysAnAdmin(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	room, err := store.StartRoom("Launch plan", "Ada@Example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if room.RoleOf("ada@example.com") != RoleAdmin || !room.Room {
		t.Fatalf("the room's maker is not its admin: %+v", room)
	}
	if err := store.SetMember(room.ID, "bob@example.com", RoleMember); err != nil {
		t.Fatal(err)
	}
	if _, err := store.RemoveMember(room.ID, "ada@example.com", nil); err == nil {
		t.Error("the room's only admin left it")
	}
	store.SetMember(room.ID, "bob@example.com", RoleAdmin)
	if _, err := store.RemoveMember(room.ID, "ada@example.com", nil); err != nil {
		t.Fatal(err)
	}
	if held, _ := store.Get(room.ID); held.RoleOf("ada@example.com") != "" {
		t.Error("the room's maker kept a role after leaving it")
	}
	if err := store.SetMember(room.ID, "bob@example.com", "owner"); err == nil {
		t.Error("a role that is not admin or member was given")
	}
	if _, err := store.AddAgent(room.ID, Agent{Name: "Tosin's Claude", Harness: "claude", Member: "abc"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddAgent(room.ID, Agent{Name: "Another", Harness: "codex", Member: "abc"}); err == nil {
		t.Error("the same workspace agent joined twice")
	}
	if _, err := store.AddAgent(room.ID, Agent{Name: "tosin's claude", Harness: "codex"}); err == nil {
		t.Error("two agents took the same name")
	}
}

func TestThePeopleInARoomFindItWithoutLookingThroughEveryInstance(t *testing.T) {
	root := t.TempDir()
	store, _ := Open(root)
	first, _ := store.StartRoom("First", "ada@example.com", "")
	second, _ := store.StartRoom("Second", "bob@example.com", "")
	if _, err := store.Start("not-a-room", Instance{Harness: "claude", Who: "ada@example.com"}); err != nil {
		t.Fatal(err)
	}
	store.SetMember(second.ID, "Ada@example.com", RoleMember)
	if got := roomIDs(store.RoomsOf("ADA@example.com")); !slices.Equal(got, roomIDs([]Instance{first, second})) {
		t.Errorf("ada's rooms are %v", got)
	}
	store.RemoveMember(second.ID, "ada@example.com", nil)
	if got := roomIDs(store.RoomsOf("ada@example.com")); !slices.Equal(got, []string{first.ID}) {
		t.Errorf("after leaving, ada's rooms are %v", got)
	}
	reopened, _ := Open(root)
	if got := roomIDs(reopened.RoomsOf("bob@example.com")); !slices.Equal(got, []string{second.ID}) {
		t.Errorf("read back, bob's rooms are %v", got)
	}
	if err := reopened.Remove(first.ID); err != nil {
		t.Fatal(err)
	}
	if got := reopened.RoomsOf("ada@example.com"); len(got) != 0 {
		t.Errorf("a removed room is still ada's: %v", roomIDs(got))
	}
}

// An agent added after another was removed never gets the removed one's id: the id names a conversation each thread remembers
// and the agent's memory on disk, and a new agent given it would carry on as the one that left.
func TestANewAgentNeverTakesTheIDOfOneThatLeftSomethingBehind(t *testing.T) {
	store, _ := Open(t.TempDir())
	room, err := store.StartRoom("Launch", "ada@example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	first, _ := store.AddAgent(room.ID, Agent{Name: "Nova", Harness: "codex"})
	second, _ := store.AddAgent(room.ID, Agent{Name: "Wren", Harness: "claude"})
	thread := store.Threads(room.ID)[0].ID
	if err := store.Learned(room.ID, thread, second.ID, "wren's conversation"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.AgentState(room.ID, second.ID), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := store.RemoveAgent(room.ID, second.ID); err != nil {
		t.Fatal(err)
	}
	third, err := store.AddAgent(room.ID, Agent{Name: "Ivy", Harness: "claude"})
	if err != nil {
		t.Fatal(err)
	}
	if third.ID == second.ID || third.ID == first.ID {
		t.Fatalf("the new agent took the id %s, which %s or %s had", third.ID, first.ID, second.ID)
	}
	if held, _ := store.Get(room.ID); len(held.Agents) != 2 {
		t.Errorf("the room holds %+v", held.Agents)
	}
}
