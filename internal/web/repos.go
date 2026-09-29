package web

// UseLocalRepos names the directories on this server a job's repository may be
// cloned from. Without any, a repository is an https or ssh address: a path on
// the server's own disk would let anybody who can start work read what the
// server can.
func (s *Server) UseLocalRepos(dirs []string) { s.localRepos = dirs }
