package web

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/getlatentic/gusto/internal/taskindex"
)

type tasksAnswer struct {
	Tasks  []taskindex.Row  `json:"tasks"`
	Counts map[string]int   `json:"counts"`
	Next   taskindex.Cursor `json:"next,omitempty"`
}

// tasksJSON is GET /api/jobs: the person's tasks that match the filter, newest first, a page at a time.
func (s *Server) tasksJSON(w http.ResponseWriter, r *http.Request) {
	if s.listing.store == nil {
		http.Error(w, "this server keeps no task index", http.StatusNotFound)
		return
	}
	query, err := s.readTaskQuery(r, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if building := s.listing.building(); building.Building {
		w.Header().Set("Task-Index-Building", fmt.Sprintf("%d of %d tasks", building.Done, building.Total))
	}
	answer := tasksAnswer{Tasks: []taskindex.Row{}, Counts: map[string]int{}}
	if !query.None {
		if answer, err = s.readTasks(r, query); err != nil {
			s.tasksFailed(w, err)
			return
		}
	}
	s.asJSON(w, answer)
}

func (s *Server) readTasks(r *http.Request, query taskQuery) (tasksAnswer, error) {
	rows, next, err := s.listing.store.List(r.Context(), query.Filter, query.Page)
	if err != nil {
		return tasksAnswer{}, err
	}
	counts, err := s.listing.store.Count(r.Context(), query.Filter)
	if err != nil {
		return tasksAnswer{}, err
	}
	return tasksAnswer{Tasks: append([]taskindex.Row{}, rows...), Counts: counts, Next: next}, nil
}

func (s *Server) tasksFailed(w http.ResponseWriter, err error) {
	if errors.Is(err, taskindex.ErrBadCursor) || errors.Is(err, taskindex.ErrBadSort) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Error(w, "the tasks could not be read: "+taskindex.Scrub(err), http.StatusInternalServerError)
}
