package instance

import (
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
	room, err := store.StartRoom("Launch plan", "Ada@Example.com")
	if err != nil {
		t.Fatal(err)
	}
	if room.RoleOf("ada@example.com") != RoleAdmin || !room.Room {
		t.Fatalf("the room's maker is not its admin: %+v", room)
	}
	if err := store.SetMember(room.ID, "bob@example.com", RoleMember); err != nil {
		t.Fatal(err)
	}
	if err := store.RemoveMember(room.ID, "ada@example.com"); err == nil {
		t.Error("the room's only admin left it")
	}
	store.SetMember(room.ID, "bob@example.com", RoleAdmin)
	if err := store.RemoveMember(room.ID, "ada@example.com"); err != nil {
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
	first, _ := store.StartRoom("First", "ada@example.com")
	second, _ := store.StartRoom("Second", "bob@example.com")
	if _, err := store.Start("not-a-room", Instance{Harness: "claude", Who: "ada@example.com"}); err != nil {
		t.Fatal(err)
	}
	store.SetMember(second.ID, "Ada@example.com", RoleMember)
	if got := roomIDs(store.RoomsOf("ADA@example.com")); !slices.Equal(got, roomIDs([]Instance{first, second})) {
		t.Errorf("ada's rooms are %v", got)
	}
	store.RemoveMember(second.ID, "ada@example.com")
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

// A member's choice to let the room's agents use their connected apps is off at first, theirs alone, kept when the room is read
// again, and kept when their role changes.
func TestAMemberLetsTheRoomsAgentsUseTheirAppsOnlyForThemselves(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	room, err := store.StartRoom("Launch plan", "ada@example.com")
	if err != nil {
		t.Fatal(err)
	}
	store.SetMember(room.ID, "bob@example.com", RoleMember)
	if room.LetsConnections("ada@example.com") {
		t.Fatal("a member's apps were let before they chose")
	}
	if err := store.LetConnections(room.ID, "Ada@Example.com", true); err != nil {
		t.Fatal(err)
	}
	if err := store.LetConnections(room.ID, "carol@example.com", true); err == nil {
		t.Error("somebody outside the room let its agents use their apps")
	}
	store.SetMember(room.ID, "ada@example.com", RoleAdmin)
	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	held, _ := again.Get(room.ID)
	if !held.LetsConnections("ada@example.com") || held.LetsConnections("bob@example.com") {
		t.Errorf("the choice is not each member's own, or was not kept: %+v", held.Members)
	}
}
