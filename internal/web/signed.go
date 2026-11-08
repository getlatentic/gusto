package web

import "net/http"

// signedAs is who a page is drawn for, as the foot of the rail shows them: their initials, and their name on hover.
type signedAs struct {
	Name, Initials string
	// Who is the sign-in address, since a name drawn from an address can belong to two people.
	Who string
}

func (s *Server) signedAs(r *http.Request) signedAs {
	who := s.keeper.who(r)
	name := nameFromAddress(who)
	return signedAs{Name: name, Initials: initialsOf(name), Who: who}
}
