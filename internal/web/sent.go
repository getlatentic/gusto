package web

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/apitoken"
)

// sentOverHTTP is who asked for a task that arrived at POST /api/jobs with the
// shared password. The server knows how such a task arrived and not who sent it.
const sentOverHTTP = "sent over HTTP"

// tokenAsker is the start of who asked for a task a token sent.
const tokenAsker = "token:"

type callerKey struct{}

// callerOf is who a request to the API came from: the token it carried, or the
// shared password when the server takes no tokens.
func callerOf(r *http.Request) string {
	if who, ok := r.Context().Value(callerKey{}).(string); ok {
		return who
	}
	return sentOverHTTP
}

// sent guards an endpoint a program calls. It reads the Authorization header
// rather than the cookie: no script can fill in a sign-in page, and the
// cookie's signing key is made at boot, so a saved cookie dies with every
// restart while a token does not. Accepting nothing a browser sends by itself
// also leaves no way to post from another origin on a signed-in person's
// behalf.
//
// A server with API tokens takes those and not the shared password, so that
// every call names what made it and can be ended without changing what
// everything else uses. A server with none takes the shared password.
func (s *Server) sent(scope string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.wrong.blocked(clientOf(r), time.Now()) {
			s.noteRefused(r, "", s.presented(), "too many wrong attempts from this source")
			tooMany(w)
			return
		}
		given, bearing := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !bearing {
			s.turnBack(w, r, "no Authorization: Bearer header", "the "+s.credential()+" goes in an Authorization: Bearer header")
			return
		}
		if s.tokens == nil {
			if !s.keeper.admits(given) {
				s.turnBack(w, r, "a wrong shared password", "that is not the shared password")
				return
			}
			next(w, r)
			return
		}
		token, err := s.tokens.Check(given)
		if err != nil {
			s.turnBack(w, r, refusalOf(err), "that is not a live API token")
			return
		}
		who := tokenAsker + token.Name
		if !token.May(scope) {
			s.turnBackForbidden(w, r, who, scope)
			return
		}
		if token.Person != "" {
			if _, in, checked := s.memberNow(token.Person); checked && !in {
				s.turnBack(w, r, "a token for a person no longer in the organization", "that is not a live API token")
				return
			}
		}
		next(w, r.WithContext(carrying(r.Context(), token, who)))
	}
}

// carrying is a request's context with what its token says: who it came from, and the person it acts as or, for an
// organization's token, the workspace it works in.
func carrying(ctx context.Context, token apitoken.Token, who string) context.Context {
	ctx = context.WithValue(ctx, callerKey{}, who)
	if token.Person == "" {
		return withTokenSpace(ctx, token.Workspace)
	}
	ctx = context.WithValue(ctx, personKey{}, token.Person)
	return context.WithValue(ctx, tokenIDKey{}, token.ID)
}

// credential is what a program presents to this server, in a sentence.
func (s *Server) credential() string {
	if s.tokens != nil {
		return "API token"
	}
	return "shared password"
}

// refusalOf is why a token was turned away, as a short phrase.
func refusalOf(err error) string {
	switch {
	case errors.Is(err, apitoken.ErrRevoked):
		return "a revoked token"
	case errors.Is(err, apitoken.ErrIdle):
		return "a computer's token unused for too long"
	case errors.Is(err, apitoken.ErrUnread):
		return "the tokens could not be read"
	}
	return "an unknown token"
}

// turnBack answers 401 and records it. Saying which of the two things went
// wrong tells whoever sent this nothing they did not already send, and saves
// them looking for a header they wrote correctly. A wrong token and a revoked
// one get one answer, so a guess cannot learn that a token once existed; the
// record says which it was.
func (s *Server) turnBack(w http.ResponseWriter, r *http.Request, why, said string) {
	s.wrong.failed(clientOf(r), time.Now())
	s.noteRefused(r, "", s.presented(), why)
	w.Header().Set("WWW-Authenticate", `Bearer realm="gusto"`)
	reply(w, http.StatusUnauthorized, Answer{Refused: said})
}

// turnBackForbidden answers 403 to a token that is live and may not do this.
func (s *Server) turnBackForbidden(w http.ResponseWriter, r *http.Request, who, scope string) {
	s.noteRefused(r, who, s.presented(), "the token may not "+scope)
	reply(w, http.StatusForbidden, Answer{Refused: "this token may not " + scope})
}

// presented is what a refused caller was offering, for the record.
func (s *Server) presented() string {
	if s.tokens != nil {
		return "an API token"
	}
	return "the shared password"
}
