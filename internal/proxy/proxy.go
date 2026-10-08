package proxy

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/getlatentic/haas/internal/redact"
	"github.com/getlatentic/haas/internal/spend"
	"github.com/getlatentic/haas/internal/wire"
)

// Where says which file in a job directory the proxy writes its record to.
type Where func(job string) (string, bool)

// Proxy adds the credential on the way out, writes down what went and what it
// cost, and refuses a call once a cap is spent.
type Proxy struct {
	tokens   *Tokens
	held     atomic.Pointer[Credentials]
	registry atomic.Int64
	where    Where
	ledger   *spend.Ledger
	allowed  Allowlist
	client   *http.Client
	public   *http.Client
	guard    sync.Mutex
	counts   counters
	resolve  Resolver
	gateways atomic.Pointer[Gateways]
	redactor atomic.Pointer[redact.Redactor]
}

// New takes the ledger every call is counted in and every cap is checked
// against, so what the proxy refuses and what a report says was spent are read
// from one place, and the hosts a job may reach besides its model.
func New(tokens *Tokens, credentials Credentials, where Where, ledger *spend.Ledger, allowed Allowlist) *Proxy {
	p := &Proxy{
		tokens:  tokens,
		where:   where,
		ledger:  ledger,
		allowed: allowed,
		// A model answers slowly and streams. There is no timeout to set here
		// that is not a guess at somebody's slowest prompt; the job's own budget
		// is what stops a run that will not end.
		// The model is where a credential goes, so an answer that sends the
		// call somewhere else is not followed: it would carry the credential's
		// headers with it.
		client: &http.Client{
			Transport:     http.DefaultTransport,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
	p.public = publicClient(p.lookup)
	p.SetCredentials(credentials, 0)
	return p
}

// SetCredentials changes what the proxy holds, and says which version of the organisation's registry
// that is. A call in flight keeps the credential it began with; the next one finds what is set here.
func (p *Proxy) SetCredentials(credentials Credentials, registry int64) {
	p.held.Store(&credentials)
	p.registry.Store(registry)
}

func (p *Proxy) credentials() Credentials { return *p.held.Load() }

// Handler answers the address a container was given. It is deliberately not
// behind the sign-in: a container has no cookie, and the token is what says who
// this is. That also means the path must never be useful without one.
func (p *Proxy) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Anything but the model is asked for the way any forward proxy is,
		// and goes through the allowlist.
		if r.Method == http.MethodConnect || r.URL.IsAbs() {
			p.outbound(w, r)
			return
		}
		token, rest := split(r.URL.Path)
		// An OpenAI-compatible client puts its key in the Authorization header
		// and its base URL in static configuration. A token in the path would
		// mean rewriting that configuration per job, so the header is accepted
		// too: the container presents the token exactly where it would have
		// presented a credential, and the proxy trades it for the real one.
		if _, known := p.tokens.Job(token); !known {
			if bearer := bearerOf(r); bearer != "" {
				if _, known := p.tokens.Job(bearer); known {
					rest = path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/model"))
					token = bearer
				}
			}
		}
		ticket, known := p.tokens.Job(token)
		if !known {
			// The same answer for a token that never existed and one that has
			// ended, so this cannot be used to learn which jobs are running.
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		if toolsPath(rest) {
			p.tools(w, r, ticket)
			return
		}
		p.pass(w, r, ticket, rest)
	})
}

// split takes the token off the front of /model/{token}/v1/chat/completions.
func split(full string) (token, rest string) {
	trimmed := strings.TrimPrefix(path.Clean(full), "/model/")
	token, rest, found := strings.Cut(trimmed, "/")
	if !found {
		return token, ""
	}
	return token, "/" + rest
}

func (p *Proxy) pass(w http.ResponseWriter, r *http.Request, ticket Ticket, rest string) {
	job := ticket.Job
	credential, held := p.credentials().Get(ticket.Credential)
	if !held {
		// The job was given this credential when it started, so losing it here
		// means the deployment changed underneath a running job.
		http.Error(w, "this job's credential is no longer held here", http.StatusBadGateway)
		return
	}
	asked, err := readQuestion(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		return
	}
	sent := time.Now()
	if refusal := p.ledger.Allow(job, credential.Name, asked.model); refusal != nil {
		p.refuse(w, job, spend.RefusedCall(rest, asked.model, sent.Unix(), refusal))
		return
	}
	// What leaves is masked before it is translated, so that the provider sees the mask whatever shape the harness called in.
	redactor := p.redactor.Load()
	masked, maskedRequest := maskRequest(redactor, asked.body)
	asked.body = masked
	body, path, translated, err := translation(credential, rest, asked.body)
	if err != nil {
		wire.Fail(w, translated.From, http.StatusBadRequest, err.Error())
		return
	}
	to, err := url.JoinPath(credential.Address, path)
	if err != nil {
		http.Error(w, "that is not an address", http.StatusBadRequest)
		return
	}
	out, err := http.NewRequestWithContext(r.Context(), r.Method, to, bytes.NewReader(body))
	if err != nil {
		http.Error(w, "that request could not be made", http.StatusBadGateway)
		return
	}
	copyHeaders(out.Header, r.Header)
	if translated.From != "" {
		asChat(out.Header)
	}
	// The credential is added here and nowhere else. Whatever the container sent
	// in its place is discarded rather than forwarded.
	credential.apply(out.Header)
	// Left to itself the client asks for gzip and unpacks it, so the usage in
	// the answer is readable here. A compression the container chose would pass
	// through packed, and the call would go uncounted.
	out.Header.Del("Accept-Encoding")
	call := spend.Call{Path: rest, AtUnix: sent.Unix(), Model: asked.model, Credential: credential.Name, Registry: p.registry.Load(), Translated: string(translated.From)}
	answer, err := p.clientFor(credential).Do(out)
	if err != nil {
		call.Error, call.Took = err.Error(), since(sent)
		p.note(job, call)
		http.Error(w, "the model did not answer", http.StatusBadGateway)
		return
	}
	defer answer.Body.Close()
	meter := spend.NewMeter(answer.Header.Get("Content-Type"))
	var masking *maskedAnswer
	var toHarness http.ResponseWriter = w
	if redactor.On() {
		masking = newMaskedAnswer(w, redactor)
		toHarness = masking
	}
	if translated.From != "" {
		read := &countedReader{from: io.TeeReader(answer.Body, meter)}
		if err := translated.Answer(toHarness, answer.StatusCode, answer.Header.Get("Content-Type"), read); err != nil {
			call.Error = err.Error()
		}
		call.Bytes = read.n
	} else {
		copyHeaders(toHarness.Header(), answer.Header)
		toHarness.WriteHeader(answer.StatusCode)
		call.Bytes, _ = io.Copy(io.MultiWriter(relay{to: toHarness}, meter), answer.Body)
	}
	call.Redacted = maskedRequest
	if masking != nil {
		if err := masking.Close(); err != nil && call.Error == "" {
			call.Error = err.Error()
		}
		call.RedactedAnswer = masking.counts
	}
	call.Status, call.Took = answer.StatusCode, since(sent)
	p.note(job, p.priced(call, meter.Usage()))
}

// priced puts the provider's count and its cost on a call. Either stays null
// when it is not known: an answer without usage, or a model without a price.
func (p *Proxy) priced(call spend.Call, usage *spend.Usage) spend.Call {
	if usage != nil {
		call.InputTokens, call.OutputTokens = &usage.Input, &usage.Output
		call.CachedTokens = &usage.Cached
	}
	call.USD = p.ledger.Policy().Cost(call.Credential, call.Model, usage, time.Unix(call.AtUnix, 0))
	return call
}

func copyHeaders(to, from http.Header) {
	for name, values := range from {
		if strings.EqualFold(name, "Connection") || strings.EqualFold(name, "Host") {
			continue
		}
		for _, value := range values {
			to.Add(name, value)
		}
	}
}

func since(start time.Time) int64 { return time.Since(start).Milliseconds() }

// bearerOf is the token a client presented where a key would go.
//
// Where that is depends on the shape the client speaks: an OpenAI-compatible
// one sends Authorization: Bearer, and an Anthropic-shaped one sends x-api-key.
// Both are read, because the container is told one address and puts its token
// wherever its own provider's key belongs.
func bearerOf(r *http.Request) string {
	given := r.Header.Get("Authorization")
	if bearer, found := strings.CutPrefix(given, "Bearer "); found {
		return strings.TrimSpace(bearer)
	}
	return strings.TrimSpace(r.Header.Get("x-api-key"))
}
