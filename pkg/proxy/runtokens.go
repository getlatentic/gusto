package proxy

import (
	"encoding/json"
	"net/http"
	"path"
	"strings"
)

// RunTokens lets in a task whose sandbox holds a key of its own. The task's secret is traded once for a run token bound to that
// key, and every call shows the token with a fresh proof signed by the key (DPoP, RFC 9449). A token or a secret taken out of the
// sandbox is worth nothing: the secret is spent, and the token needs the key.
type RunTokens interface {
	Exchange(subject, proof, method, url string, redeem func(secret string) (job string, ok bool)) (string, error)
	Check(token, proof, method, url string) (job string, err error)
}

// Runs is a token source whose bound tasks' secrets are traded for run tokens. Tokens is one.
type Runs interface {
	Redeem(secret string) (job string, ok bool)
	Run(job string) (Ticket, bool)
}

// UseRunTokens has the proxy trade bound tasks' secrets for run tokens and check them, with source keeping the tasks.
func (p *Proxy) UseRunTokens(tokens RunTokens) { p.runTokens.Store(&tokens) }

// exchangePath is where a sandbox trades its secret, or a run token about to expire, for a run token.
const exchangePath = "/model/token"

// tokenExchange is the grant RFC 8693 names, and the only one taken.
const tokenExchange = "urn:ietf:params:oauth:grant-type:token-exchange"

func (p *Proxy) runTokenSource() (RunTokens, Runs, bool) {
	tokens := p.runTokens.Load()
	runs, keeps := p.tokenSource().(Runs)
	if tokens == nil || !keeps {
		return nil, nil, false
	}
	return *tokens, runs, true
}

// exchange answers a sandbox trading its secret, or its run token, for a run token, as an OAuth token endpoint answers.
func (p *Proxy) exchange(w http.ResponseWriter, r *http.Request) {
	tokens, runs, on := p.runTokenSource()
	if !on || r.Method != http.MethodPost {
		http.Error(w, "no", http.StatusNotFound)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if r.ParseForm() != nil || r.PostForm.Get("grant_type") != tokenExchange {
		oauthError(w, "unsupported_grant_type")
		return
	}
	token, err := tokens.Exchange(r.PostForm.Get("subject_token"), r.Header.Get("DPoP"), r.Method, requestURL(r), runs.Redeem)
	if err != nil {
		oauthError(w, "invalid_grant")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(map[string]string{
		"access_token": token, "token_type": "DPoP", "issued_token_type": "urn:ietf:params:oauth:token-type:access_token",
	})
}

func oauthError(w http.ResponseWriter, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{"error": code})
}

// runTicket is the task a call with a run token stands for, and the rest of its path. A call with no run token is not one.
func (p *Proxy) runTicket(r *http.Request) (Ticket, string, bool) {
	token, shown := strings.CutPrefix(r.Header.Get("Authorization"), "DPoP ")
	tokens, runs, on := p.runTokenSource()
	if !shown || !on {
		return Ticket{}, "", false
	}
	job, err := tokens.Check(strings.TrimSpace(token), r.Header.Get("DPoP"), r.Method, requestURL(r))
	if err != nil {
		return Ticket{}, "", false
	}
	ticket, live := runs.Run(job)
	return ticket, path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/model")), live
}

// requestURL is the address a proof names for a request: as the sandbox asked for it, without its query.
func requestURL(r *http.Request) string {
	return "http://" + r.Host + r.URL.Path
}
