package web

import "github.com/getlatentic/gusto/internal/taskindex"

// taskIndex is the index of tasks the list page and GET /api/jobs read, and how far it has caught up with the
// files. Without one they read the tasks this server holds in memory.
type taskIndex struct {
	store  taskindex.Store
	status func() taskindex.Status
}

// SetTasks gives the task list the index of tasks.
func (s *Server) SetTasks(store taskindex.Store) { s.listing.store = store }

// SetTasksStatus says how far the index has caught up, so that a page shows an index still being filled as that
// and not as a server with few tasks.
func (s *Server) SetTasksStatus(f func() taskindex.Status) { s.listing.status = f }

func (t taskIndex) building() taskindex.Status {
	if t.status == nil {
		return taskindex.Status{}
	}
	return t.status()
}
