package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"
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

// Proxy adds the credential on the way out and writes down what went.
type Proxy struct {
	tokens   *Tokens
	upstream Upstream
	where    Where
	client   *http.Client
	guard    sync.Mutex
}

func New(tokens *Tokens, upstream Upstream, where Where) *Proxy {
	return &Proxy{
		tokens:   tokens,
		upstream: upstream,
		where:    where,
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
	sent := time.Now()
	out, err := http.NewRequestWithContext(r.Context(), r.Method, to, r.Body)
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
	answer, err := p.client.Do(out)
	if err != nil {
		p.note(job, record{Path: rest, Error: err.Error(), Took: since(sent)})
		http.Error(w, "the model did not answer", http.StatusBadGateway)
		return
	}
	defer answer.Body.Close()
	copyHeaders(w.Header(), answer.Header)
	w.WriteHeader(answer.StatusCode)
	written, _ := io.Copy(w, answer.Body)
	p.note(job, record{
		Path:   rest,
		Status: answer.StatusCode,
		Bytes:  written,
		Took:   since(sent),
		Model:  p.upstream.Name,
	})
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

// record is what the proxy saw. It is deliberately independent of what the
// harness says it spent: a harness reporting its own usage is a claim, and this
// is the count of what actually left the box.
type record struct {
	Path   string `json:"path"`
	Status int    `json:"status,omitempty"`
	Bytes  int64  `json:"bytes,omitempty"`
	Took   int64  `json:"tookMs"`
	Model  string `json:"credential,omitempty"`
	Error  string `json:"error,omitempty"`
}

func (p *Proxy) note(job string, seen record) {
	where, ok := p.where(job)
	if !ok {
		return
	}
	line, err := json.Marshal(seen)
	if err != nil {
		return
	}
	p.guard.Lock()
	defer p.guard.Unlock()
	file, err := os.OpenFile(where, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		slog.Error("the proxy could not write down a call", "job", job, "error", err)
		return
	}
	defer file.Close()
	if _, err := fmt.Fprintf(file, "%s\n", line); err != nil {
		slog.Error("the proxy could not write down a call", "job", job, "error", err)
	}
}

// bearerOf is the token a client presented where a key would go.
func bearerOf(r *http.Request) string {
	given := r.Header.Get("Authorization")
	if bearer, found := strings.CutPrefix(given, "Bearer "); found {
		return strings.TrimSpace(bearer)
	}
	return ""
}
