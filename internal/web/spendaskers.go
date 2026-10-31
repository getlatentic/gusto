package web

import (
	"strings"

	"github.com/getlatentic/gusto/pkg/spend"
)

// byAsker is the month's jobs added up under who each counts against, the person who asked or a room agent's owner, in the
// words a person reads: a name for a person, "Token ci" for a token, and one line for everything the shared password or the
// API sent before tokens existed.
func (s *Server) byAsker(jobs []spend.Line) []spend.Line {
	return plainly(jobs, func(id string) string {
		record, found := s.index.Get(id)
		if !found {
			return askerNotRecorded
		}
		return askerOf(record.Payer())
	})
}

const askerNotRecorded = "Not recorded"

// peopleShares is what each person spent, each called by their name with their address under it.
func (s *Server) peopleShares(lines []spend.Line) []share {
	out := shares(lines)
	for i := range out {
		if strings.Contains(out[i].Name, "@") {
			out[i].Label, out[i].Sub = s.personName(out[i].Name), out[i].Name
		}
	}
	return out
}

func askerOf(asked string) string {
	switch {
	case asked == "":
		return askerNotRecorded
	case asked == sentOverHTTP:
		return "Sent over the API"
	case asked == sharedSignIn:
		return "The shared sign-in"
	case strings.HasPrefix(asked, tokenAsker):
		return "Token " + strings.TrimPrefix(asked, tokenAsker)
	}
	return asked
}

// askers is the breakdown as the JSON report carries it.
type askers struct {
	spend.Report
	Askers []spend.Line `json:"askers"`
}
