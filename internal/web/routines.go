package web

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/instance"
)

// RunRoutines starts the turns routines are due for, every interval until ctx ends. A routine is a turn like any other:
// it goes through the proxy, counts against the cap, lands in the signed record, and is announced to the person who added it
// the way any turn of theirs is, on the events a tray listens to.
func (s *Server) RunRoutines(ctx context.Context, every time.Duration) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		s.StartDueRoutines(time.Now())
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// StartDueRoutines starts each routine whose slot has come and says nothing of the rest. A routine whose agent is still
// answering its last turn skips the slot, and the skip is kept, so the page says why nothing started.
func (s *Server) StartDueRoutines(now time.Time) {
	for _, held := range s.instances.List() {
		for _, routine := range s.instances.Routines(held.ID) {
			if routine.Due(now) {
				s.startRoutine(now, held, routine)
			}
		}
	}
}

func (s *Server) startRoutine(now time.Time, held instance.Instance, routine instance.Routine) {
	thread, found := s.instances.Thread(held.ID, routine.Thread)
	if !found {
		_ = s.instances.RoutineRan(held.ID, routine.ID, instance.Ran{At: now.Unix(), Skipped: "its conversation is gone"})
		return
	}
	if last := s.latestTurn(thread); last.ID != "" && !last.State.Done() {
		_ = s.instances.RoutineRan(held.ID, routine.ID, instance.Ran{At: now.Unix(), Skipped: "the last run was still answering"})
		return
	}
	// The slot is the key, so a restart that finds the same slot due starts one turn and not two.
	key := "routine-" + routine.ID + "-" + strconv.FormatInt(now.Unix()/60, 10)
	record, _, err := s.startTurn(routine.Who, held, thread, "[routine] "+routine.Title+": "+routine.Prompt, key)
	ran := instance.Ran{At: now.Unix(), Job: record.ID}
	if err != nil {
		ran = instance.Ran{At: now.Unix(), Skipped: err.Error()}
	}
	_ = s.instances.RoutineRan(held.ID, routine.ID, ran)
}

// addRoutine keeps a routine on an agent: a title, what to ask, and when.
func (s *Server) addRoutine(w http.ResponseWriter, r *http.Request) {
	held, found := s.mine(w, r)
	if !found {
		return
	}
	every, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("every")))
	routine := instance.Routine{
		Title:  strings.TrimSpace(r.FormValue("title")),
		Prompt: strings.TrimSpace(r.FormValue("prompt")),
		Daily:  strings.TrimSpace(r.FormValue("daily")),
		Every:  every,
		Who:    s.keeper.who(r),
	}
	where := "/instances/" + held.ID + "#routines"
	if routine.Title == "" || routine.Prompt == "" {
		s.turnAway(w, r, where, "a routine needs a title and what to ask")
		return
	}
	if _, err := s.instances.AddRoutine(held.ID, routine); err != nil {
		s.turnAway(w, r, where, err.Error())
		return
	}
	http.Redirect(w, r, where, http.StatusSeeOther)
}

// changeRoutine pauses, resumes or removes a routine, by the last word of the path.
func (s *Server) changeRoutine(w http.ResponseWriter, r *http.Request) {
	held, found := s.mine(w, r)
	if !found {
		return
	}
	id := r.PathValue("routine")
	var err error
	switch r.PathValue("action") {
	case "pause":
		err = s.instances.PauseRoutine(held.ID, id, true)
	case "resume":
		err = s.instances.PauseRoutine(held.ID, id, false)
	case "remove":
		err = s.instances.RemoveRoutine(held.ID, id)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.turnAway(w, r, "/instances/"+held.ID+"#routines", err.Error())
		return
	}
	http.Redirect(w, r, "/instances/"+held.ID+"#routines", http.StatusSeeOther)
}

// routineRow is a routine as the page shows it: what it is, when it runs, and how its last slot went.
type routineRow struct {
	instance.Routine
	When string
	Last string
}

func (s *Server) routineRows(id string) []routineRow {
	var rows []routineRow
	for _, routine := range s.instances.Routines(id) {
		row := routineRow{Routine: routine, When: routine.When()}
		switch {
		case routine.Last == nil:
			row.Last = "not yet"
		case routine.Last.Skipped != "":
			row.Last = fmt.Sprintf("skipped: %s", routine.Last.Skipped)
		default:
			row.Last = "ran"
		}
		rows = append(rows, row)
	}
	return rows
}
