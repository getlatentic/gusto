package web

import (
	"net/http"
	"unicode/utf8"
)

// signedAs is who a page is drawn for, as the foot of the side list shows them: a name and its first letter.
type signedAs struct {
	Name, Initial string
	// Who is the sign-in address, shown on hover, since a name drawn from an address can belong to two people.
	Who string
}

func (s *Server) signedAs(r *http.Request) signedAs {
	who := s.keeper.who(r)
	name := nameFromAddress(who)
	initial, _ := utf8.DecodeRuneInString(name)
	return signedAs{Name: name, Initial: string(initial), Who: who}
}
