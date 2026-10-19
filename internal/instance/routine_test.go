package instance

import (
	"testing"
	"time"
)

func at(day int, hour, minute int) time.Time {
	return time.Date(2026, 10, day, hour, minute, 0, 0, time.UTC)
}

func TestARoutineIsDueOnceTheTimeOfDayPassesAndNotAgainThatDay(t *testing.T) {
	routine := Routine{Daily: "08:00", Added: at(20, 7, 0).Unix()}
	for _, tc := range []struct {
		now  time.Time
		last *Ran
		want bool
	}{
		{at(20, 7, 59), nil, false},
		{at(20, 8, 0), nil, true},
		{at(20, 12, 0), nil, true},
		{at(20, 12, 0), &Ran{At: at(20, 8, 1).Unix()}, false},
		{at(21, 7, 59), &Ran{At: at(20, 8, 1).Unix()}, false},
		{at(21, 8, 0), &Ran{At: at(20, 8, 1).Unix()}, true},
	} {
		routine.Last = tc.last
		if got := routine.Due(tc.now); got != tc.want {
			t.Errorf("at %s with last %+v: due %v, want %v", tc.now.Format("Jan 2 15:04"), tc.last, got, tc.want)
		}
	}
	if (Routine{Daily: "08:00", Added: at(20, 9, 0).Unix()}).Due(at(20, 9, 30)) {
		t.Error("a routine added after today's time waits for tomorrow's")
	}
}

func TestAnHourlyRoutineRunsRightAwayThenEveryNHours(t *testing.T) {
	routine := Routine{Every: 6, Added: at(20, 9, 0).Unix()}
	if !routine.Due(at(20, 9, 0)) {
		t.Error("a routine that has never run is due")
	}
	routine.Last = &Ran{At: at(20, 9, 0).Unix()}
	if routine.Due(at(20, 14, 59)) || !routine.Due(at(20, 15, 0)) {
		t.Error("six hours after the last slot, and not before")
	}
}

func TestAPausedRoutineIsNeverDue(t *testing.T) {
	if (Routine{Every: 1, Paused: true}).Due(at(20, 9, 0)) {
		t.Error("paused")
	}
}

func TestAScheduleIsATimeOfDayOrAnHourCountAndNotBoth(t *testing.T) {
	for schedule, ok := range map[Routine]bool{
		{Daily: "08:00"}:           true,
		{Every: 6}:                 true,
		{}:                         false,
		{Daily: "8am"}:             false,
		{Daily: "25:00"}:           false,
		{Daily: "08:00", Every: 6}: false,
		{Every: -1}:                false,
	} {
		if err := schedule.Schedule(); (err == nil) != ok {
			t.Errorf("%+v: %v", schedule, err)
		}
	}
}

func TestARoutineKeepsItsThreadAndSurvivesAReload(t *testing.T) {
	store := opened(t)
	held, err := store.Start("20261020-0900-work", Instance{Harness: "opencode"})
	if err != nil {
		t.Fatal(err)
	}
	added, err := store.AddRoutine(held.ID, Routine{Title: "Morning", Prompt: "what changed", Daily: "08:00", Who: "tosin"})
	if err != nil {
		t.Fatal(err)
	}
	if added.ID != "r1" || added.Thread == "" || added.Thread == store.Threads(held.ID)[0].ID {
		t.Fatalf("a routine is given an id and a conversation of its own: %+v", added)
	}
	if err := store.RoutineRan(held.ID, added.ID, Ran{At: 1, Job: "job-1"}); err != nil {
		t.Fatal(err)
	}
	if err := store.PauseRoutine(held.ID, added.ID, true); err != nil {
		t.Fatal(err)
	}
	again, err := Open(store.Root())
	if err != nil {
		t.Fatal(err)
	}
	read := again.Routines(held.ID)
	if len(read) != 1 || !read[0].Paused || read[0].Last == nil || read[0].Last.Job != "job-1" || read[0].Who != "tosin" {
		t.Fatalf("read back as %+v", read)
	}
	second, _ := again.AddRoutine(held.ID, Routine{Title: "Evening", Prompt: "x", Every: 12})
	if err := again.RemoveRoutine(held.ID, "r1"); err != nil {
		t.Fatal(err)
	}
	third, _ := again.AddRoutine(held.ID, Routine{Title: "Night", Prompt: "x", Every: 24})
	if second.ID != "r2" || third.ID != "r3" {
		t.Errorf("a removed routine's id is not given again: %s, %s", second.ID, third.ID)
	}
	if err := again.RemoveRoutine(held.ID, "r9"); err == nil {
		t.Error("removing a routine that is not there is an error")
	}
	if _, err := again.AddRoutine(held.ID, Routine{Title: "Bad", Prompt: "x"}); err == nil {
		t.Error("a routine with no schedule is refused")
	}
}
