package proxy

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/getlatentic/haas/internal/spend"
)

// Upstream is where the model actually answers, and the credential to reach it.
// An empty Key is normal: a model in the building often needs none.
type Upstream struct {
	// Address is the model's root, with no version on it: the container is given
	// `.../model/<token>/v1` and the harness appends the rest, so the whole path
	// after the token comes from the container and this must not repeat it.
	Address string
	Key     string
	// Name is what the evidence records. The key itself never appears anywhere.
	Name string
}

// Where says which file in a job directory the proxy writes its record to.
type Where func(job string) (string, bool)

// Proxy adds the credential on the way out, writes down what went and what it
// cost, and refuses a call once a cap is spent.
type Proxy struct {
	tokens   *Tokens
	upstream Upstream
	where    Where
	ledger   *spend.Ledger
	client   *http.Client
	guard    sync.Mutex
}

// New takes the ledger every call is counted in and every cap is checked
// against, so what the proxy refuses and what a report says was spent are read
// from one place.
func New(tokens *Tokens, upstream Upstream, where Where, ledger *spend.Ledger) *Proxy {
	return &Proxy{
		tokens:   tokens,
		upstream: upstream,
		where:    where,
		ledger:   ledger,
		// A model answers slowly and streams. There is no timeout to set here
		// that is not a guess at somebody's slowest prompt; the job's own budget
		// is what stops a run that will not end.
		client: &http.Client{},
	}
}

// Handler answers the address a container was given. It is deliberately not
// behind the sign-in: a container has no cookie, and the token is what says who
// this is. That also means the path must never be useful without one.
func (p *Proxy) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		job, known := p.tokens.Job(token)
		if !known {
			// The same answer for a token that never existed and one that has
			// ended, so this cannot be used to learn which jobs are running.
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		p.pass(w, r, job, rest)
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

func (p *Proxy) pass(w http.ResponseWriter, r *http.Request, job, rest string) {
	to, err := url.JoinPath(p.upstream.Address, rest)
	if err != nil {
		http.Error(w, "that is not an address", http.StatusBadRequest)
		return
	}
	asked, err := readQuestion(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
		return
	}
	sent := time.Now()
	if refusal := p.ledger.Allow(job, asked.model); refusal != nil {
		p.refuse(w, job, spend.RefusedCall(rest, asked.model, sent.Unix(), refusal))
		return
	}
	out, err := http.NewRequestWithContext(r.Context(), r.Method, to, bytes.NewReader(asked.body))
	if err != nil {
		http.Error(w, "that request could not be made", http.StatusBadGateway)
		return
	}
	copyHeaders(out.Header, r.Header)
	// The credential is added here and nowhere else. Whatever the container sent
	// as authorization is discarded rather than forwarded.
	out.Header.Del("Authorization")
	if p.upstream.Key != "" {
		out.Header.Set("Authorization", "Bearer "+p.upstream.Key)
	}
	// Left to itself the client asks for gzip and unpacks it, so the usage in
	// the answer is readable here. A compression the container chose would pass
	// through packed, and the call would go uncounted.
	out.Header.Del("Accept-Encoding")
	call := spend.Call{Path: rest, AtUnix: sent.Unix(), Model: asked.model, Credential: p.upstream.Name}
	answer, err := p.client.Do(out)
	if err != nil {
		call.Error, call.Took = err.Error(), since(sent)
		p.note(job, call)
		http.Error(w, "the model did not answer", http.StatusBadGateway)
		return
	}
	defer answer.Body.Close()
	copyHeaders(w.Header(), answer.Header)
	w.WriteHeader(answer.StatusCode)
	meter := spend.NewMeter(answer.Header.Get("Content-Type"))
	call.Bytes, _ = io.Copy(io.MultiWriter(relay{to: w}, meter), answer.Body)
	call.Status, call.Took = answer.StatusCode, since(sent)
	p.note(job, p.priced(call, meter.Usage()))
}

// priced puts the provider's count and its cost on a call. Either stays null
// when it is not known: an answer without usage, or a model without a price.
func (p *Proxy) priced(call spend.Call, usage *spend.Usage) spend.Call {
	if usage != nil {
		call.InputTokens, call.OutputTokens = &usage.Input, &usage.Output
	}
	call.USD = p.ledger.Policy().Cost(call.Model, usage)
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
func bearerOf(r *http.Request) string {
	given := r.Header.Get("Authorization")
	if bearer, found := strings.CutPrefix(given, "Bearer "); found {
		return strings.TrimSpace(bearer)
	}
	return ""
}
