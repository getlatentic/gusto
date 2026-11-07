package instance

import (
	"testing"
)

func aRoom(t *testing.T) (*Store, Instance) {
	t.Helper()
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	room, err := store.StartRoom("Launch", "ada@example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	return store, room
}

func TestAnAgentThatIsWorkingIsNotAcceptedAgain(t *testing.T) {
	store, room := aRoom(t)
	agent, err := store.AddAgent(room.ID, Agent{Name: "Scout", Harness: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AcceptAgent(room.ID, agent.ID); err == nil {
		t.Error("an agent no member proposed was accepted")
	}
}

func TestJoiningIsAllOrNothingAndNeverChangesARole(t *testing.T) {
	store, room := aRoom(t)
	if err := store.SetMember(room.ID, "bob@example.com", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	_, err := store.Join(room.ID, []string{"carol@example.com", "Bob@Example.com"}, nil)
	if _, already := err.(AlreadyInError); !already {
		t.Fatalf("adding a person in the room already was not refused: %v", err)
	}
	held, _ := store.Get(room.ID)
	if held.RoleOf("bob@example.com") != RoleAdmin || held.RoleOf("carol@example.com") != "" {
		t.Errorf("a refused add changed the room: %+v", held.Members)
	}
	_, err = store.Join(room.ID, []string{"carol@example.com"}, []Arrival{{Agent: Agent{Name: "Atlas", Harness: "fixture"}, Owner: "dave@example.com"}})
	if held, _ := store.Get(room.ID); err == nil || held.RoleOf("carol@example.com") != "" || len(held.Agents) != 0 {
		t.Errorf("an add whose agent's owner is not in the room added some of it: %v %+v", err, held)
	}
	added, err := store.Join(room.ID, []string{"carol@example.com"}, []Arrival{{Agent: Agent{Name: "Atlas", Harness: "fixture"}, Owner: "carol@example.com"}})
	if err != nil || len(added) != 1 {
		t.Fatalf("carol and her agent were not added: %v", err)
	}
}

func TestAPersonTakenOutOfARoomTakesTheirAgentsInTheSameChange(t *testing.T) {
	store, room := aRoom(t)
	store.SetMember(room.ID, "bob@example.com", RoleMember)
	store.AddAgent(room.ID, Agent{Name: "Atlas", Harness: "fixture", Member: "bobs"})
	store.AddAgent(room.ID, Agent{Name: "Scout", Harness: "fixture", Member: "adas"})
	gone, err := store.RemoveMember(room.ID, "bob@example.com", func(agent Agent) bool { return agent.Member == "bobs" })
	if err != nil || len(gone) != 1 || gone[0].Name != "Atlas" {
		t.Fatalf("taking bob out gave %+v %v", gone, err)
	}
	held, _ := store.Get(room.ID)
	if _, still := held.AgentCalled("Atlas"); still {
		t.Error("bob's agent stayed")
	}
	if _, kept := held.AgentCalled("Scout"); !kept {
		t.Error("ada's agent went with bob")
	}
}

func TestAPersonsOwnRoomWithAnAgentIsMadeOnceAndWhole(t *testing.T) {
	store, _ := aRoom(t)
	first, err := store.StartDirect("bob@example.com", "", Agent{Name: "Atlas", Harness: "fixture", Member: "m1"})
	if err != nil {
		t.Fatal(err)
	}
	again, _ := store.StartDirect("Bob@example.com", "", Agent{Name: "Atlas", Harness: "fixture", Member: "m1"})
	if again.ID != first.ID || !first.Direct || len(first.Agents) != 1 || store.Latest(first.ID).Title != "Atlas" {
		t.Errorf("the room is %+v, and asking again gave %s", first, again.ID)
	}
}
