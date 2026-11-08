// Package proxy is the only way out of a sandbox.
//
// An agent needs a model, and a model needs a credential. Putting that
// credential in the container would be the one thing this product tells a buyer
// it does not do, so the container is given an address here instead: the proxy
// holds the key, adds it on the way out, writes down what actually went, and
// refuses once a task has spent what it was allowed.
package proxy

import (
	"crypto/rand"
	"encoding/base64"
	"sync"
)

// Ticket is what a token stands for: the task the spending is attributed to, and
// the credential that task may spend. The credential is decided when the task
// starts and held here, so a container cannot reach a provider it was not given
// by naming one in a request.
type Ticket struct {
	Task       string
	Credential string
	// Bound is a task in a sandbox that holds a key of its own: its token is a secret traded once for a run token bound to that
	// key, and is never accepted as a token itself.
	Bound bool
}

// Tokens are what a container shows instead of a credential. One is minted per
// task and dropped when the task ends, so it is worth nothing to anybody who takes
// it later, and what it spends is attributed to a task rather than an account.
type Tokens struct {
	guard sync.RWMutex
	tasks map[string]Ticket
	// tokenOf is each task's token in tasks, so a task has one.
	tokenOf map[string]string
	// redeemed are the tasks whose secret was traded, by task, until the task ends.
	redeemed map[string]Ticket
}

func NewTokens() *Tokens {
	return &Tokens{tasks: map[string]Ticket{}, tokenOf: map[string]string{}, redeemed: map[string]Ticket{}}
}

// Mint gives this task a token good for one credential. A task that already has
// one keeps it, so a restarted run does not orphan the address its container
// was given, and it is good for the credential named now.
func (t *Tokens) Mint(task, credential string) (string, error) {
	return t.mint(Ticket{Task: task, Credential: credential})
}

// MintBound gives a task in a sandbox with a key of its own the secret it trades, once, for its run token.
func (t *Tokens) MintBound(task, credential string) (string, error) {
	return t.mint(Ticket{Task: task, Credential: credential, Bound: true})
}

func (t *Tokens) mint(ticket Ticket) (string, error) {
	t.guard.Lock()
	defer t.guard.Unlock()
	if token, held := t.tokenOf[ticket.Task]; held {
		t.tasks[token] = ticket
		return token, nil
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	t.tasks[token], t.tokenOf[ticket.Task] = ticket, token
	return token, nil
}

// Task says what a token stands for, or that it stands for nothing. A bound task's secret stands for nothing here: it is only
// traded.
func (t *Tokens) Task(token string) (Ticket, bool) {
	t.guard.RLock()
	defer t.guard.RUnlock()
	ticket, found := t.tasks[token]
	if ticket.Bound {
		return Ticket{}, false
	}
	return ticket, found
}

// Redeem takes a bound task's secret, once, and says whose it was.
func (t *Tokens) Redeem(secret string) (string, bool) {
	t.guard.Lock()
	defer t.guard.Unlock()
	ticket, found := t.tasks[secret]
	if !found || !ticket.Bound {
		return "", false
	}
	delete(t.tasks, secret)
	delete(t.tokenOf, ticket.Task)
	t.redeemed[ticket.Task] = ticket
	return ticket.Task, true
}

// Run is the ticket of a task whose secret was traded, while the task runs.
func (t *Tokens) Run(task string) (Ticket, bool) {
	t.guard.RLock()
	defer t.guard.RUnlock()
	ticket, found := t.redeemed[task]
	return ticket, found
}

// Drop ends a task's way out. Called when the task ends, so a container that
// outlives its server cannot keep spending.
func (t *Tokens) Drop(task string) {
	t.guard.Lock()
	defer t.guard.Unlock()
	delete(t.tasks, t.tokenOf[task])
	delete(t.tokenOf, task)
	delete(t.redeemed, task)
}
