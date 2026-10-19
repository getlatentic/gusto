package web

import (
	"net/http"

	"github.com/getlatentic/gusto/internal/apitoken"
)

// ProtocolVersion is the version of the protocol a client speaks to a gusto server, whether the server is on the same computer, on
// another, or an organisation's. It is the same server program and the same calls in each place: what differs is only how a client
// signs in (docs/PROTOCOL.md). A change that breaks a client raises it. A client asks GET /api/protocol and finds out.
const ProtocolVersion = 1

// who says what a call needs of the one who makes it.
const (
	// whoNobody needs nothing: the call is how somebody gets in.
	whoNobody = "open"
	// whoPerson needs a person's token, from the device flow, or on the person's own computer a signed-in session. The call is about that
	// person's own work, and somebody else's is answered as missing.
	whoPerson = "person"
)

// call is one thing a client may ask of a server. The table below is the protocol: the routes are made from it, GET /api/protocol
// reads it back, and docs/PROTOCOL.md is checked against it, so none of the three can say something the others do not.
type call struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	// Who is whoNobody, whoPerson, or the scope of an organisation's token (apitoken).
	Who  string `json:"who"`
	Does string `json:"does"`
	// Streams says the answer is a stream of server-sent events that goes on until the thing it follows ends or the client leaves.
	Streams bool `json:"streams,omitempty"`
	handler http.HandlerFunc
}

// protocol is every call of the protocol. A person's work is in four parts: sign in as a person, start or find an instance, say
// something in one of its threads (which starts a turn, a job), and follow the turn. Following is a stream, and so is the one that
// tells a client when any of its person's turns change.
func (s *Server) protocol() []call {
	return []call{
		{"POST", "/api/device", whoNobody, "a computer asks to sign in, and is given a code for its person to approve", false, s.startDevice},
		{"POST", "/api/device/token", whoNobody, "the computer asks whether its person approved, and is given a token the once they have", false, s.pollDevice},
		{"GET", "/api/me", apitoken.Work, "who this token acts as", false, s.sent(apitoken.Work, s.me)},
		{"DELETE", "/api/me", whoPerson, "sign this computer out: its token ends", false, s.endMe},

		{"GET", "/api/instances", whoPerson, "the person's instances", false, s.instancesJSON},
		{"POST", "/api/instances", whoPerson, "start an instance of an agent type, in a folder of its own", false, s.startInstanceJSON},
		{"GET", "/api/engines", whoPerson, "the agent types this server can start", false, s.enginesJSON},
		{"GET", "/api/instances/{id}", whoPerson, "one instance", false, s.instanceJSON},
		{"GET", "/api/instances/{id}/threads", whoPerson, "an instance's threads", false, s.threadsJSON},
		{"POST", "/api/instances/{id}/threads", whoPerson, "open a thread", false, s.newThreadJSON},
		{"GET", "/api/instances/{id}/threads/{thread}", whoPerson, "a thread: what was said and what came back", false, s.threadJSON},
		{"POST", "/api/instances/{id}/threads/{thread}/say", whoPerson, "say something in a thread, which starts a turn; an Idempotency-Key makes a retry the same turn", false, s.saidJSON},

		{"GET", "/api/jobs", whoPerson, "the person's tasks, newest first, narrowed by ?q, ?state and the other filters, a page at a time", false, s.tasksJSON},
		{"GET", "/api/jobs/{id}", whoPerson, "one turn: its steps, its reply and its state", false, s.jobJSON},
		{"GET", "/api/jobs/{id}/follow", whoPerson, "follow one turn: each step, the reply as it grows, and the end", true, s.followJSON},
		{"POST", "/api/jobs/{id}/stop", whoPerson, "stop a turn that is running", false, s.stopJSON},
		{"GET", "/api/events", whoPerson, "every change to the person's turns, with catch-up from ?since", true, s.eventsJSON},

		{"GET", "/api/instances/{id}/files", whoPerson, "the files in an instance's folder", false, s.filesJSON},
		{"GET", "/api/instances/{id}/file", whoPerson, "one file's bytes", false, s.fileBytes},
		{"PUT", "/api/instances/{id}/file", whoPerson, "write one file", false, s.writeFile},
		{"DELETE", "/api/instances/{id}/file", whoPerson, "remove one file", false, s.removeFile},
		{"POST", "/api/instances/{id}/hashes", whoPerson, "the hashes of the files named, so a client sends only what differs", false, s.hashesJSON},

		{"POST", "/api/jobs", apitoken.Submit, "send a task directory to be run, as an organisation's program does", false, s.sent(apitoken.Submit, s.accept)},
		{"GET", "/api/spend", apitoken.Spend, "the spend report", false, s.sent(apitoken.Spend, s.spendJSON)},
		{"GET", "/api/usage/sessions", apitoken.Spend, "the model calls added up by session, the latest first; a person's token reads that person's own", false, s.usageAccess(s.usageSessionsJSON)},
		{"GET", "/api/usage/calls", apitoken.Spend, "one session's calls, the latest first, from ?session=", false, s.usageAccess(s.usageCallsJSON)},
		{"GET", "/api/usage/totals", apitoken.Spend, "the calls that match, added up", false, s.usageAccess(s.usageTotalsJSON)},
		{"GET", "/api/usage/facets", apitoken.Spend, "the people, agents, agent types, providers and models that calls have", false, s.usageAccess(s.usageFacetsJSON)},
		{"GET", "/api/usage.csv", apitoken.Spend, "every call that matches, as CSV", false, s.usageAccess(s.usageCSV)},
		{"GET", "/api/changes", apitoken.Audit, "the record of what changed on the platform", false, s.sent(apitoken.Audit, s.changesScript)},
	}
}

// mountProtocol makes the routes of the protocol, and GET /api/protocol, which says which version this is and lists the calls.
func (s *Server) mountProtocol(mux *http.ServeMux) {
	calls := s.protocol()
	for _, c := range calls {
		handler := c.handler
		if c.Who == whoPerson {
			handler = s.asPerson(handler)
		}
		mux.HandleFunc(c.Method+" "+c.Path, handler)
	}
	mux.HandleFunc("GET /api/protocol", func(w http.ResponseWriter, _ *http.Request) {
		answerJSON(w, http.StatusOK, map[string]any{"protocol": ProtocolVersion, "calls": calls})
	})
}
