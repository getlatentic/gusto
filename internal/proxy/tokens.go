// Package proxy is the only way out of a sandbox.
//
// A harness needs a model, and a model needs a credential. Putting that
// credential in the container would be the one thing this product tells a buyer
// it does not do, so the container is given an address here instead: the proxy
// holds the key, adds it on the way out, writes down what actually went, and
// refuses once a job has spent what it was allowed.
package proxy

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
)

// Ticket is what a token stands for: the job the spending is attributed to, and
// the credential that job may spend. The credential is decided when the job
// starts and held here, so a container cannot reach a provider it was not given
// by naming one in a request.
type Ticket struct {
	Job        string
	Credential string
}

// Tokens are what a container shows instead of a credential. One is minted per
// job and dropped when the job ends, so it is worth nothing to anybody who takes
// it later, and what it spends is attributed to a job rather than an account.
type Tokens struct {
	guard sync.RWMutex
	jobs  map[string]Ticket
}

func NewTokens() *Tokens {
	return &Tokens{jobs: map[string]Ticket{}}
}

// Mint gives this job a token good for one credential. A job that already has
// one keeps it, so a restarted run does not orphan the address its container
// was given.
func (t *Tokens) Mint(job, credential string) (string, error) {
	t.guard.Lock()
	defer t.guard.Unlock()
	for token, held := range t.jobs {
		if held.Job == job {
			return token, nil
		}
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	t.jobs[token] = Ticket{Job: job, Credential: credential}
	return token, nil
}

// Job says what a token stands for, or that it stands for nothing.
func (t *Tokens) Job(token string) (Ticket, bool) {
	t.guard.RLock()
	defer t.guard.RUnlock()
	ticket, found := t.jobs[token]
	return ticket, found
}

// Drop ends a job's way out. Called when the job ends, so a container that
// outlives its server cannot keep spending.
func (t *Tokens) Drop(job string) {
	t.guard.Lock()
	defer t.guard.Unlock()
	for token, held := range t.jobs {
		if held.Job == job {
			delete(t.jobs, token)
		}
	}
}
