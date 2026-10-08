// Package spend is what a job's calls to a model cost, and the caps that stop
// a job or a team spending more.
//
// Everything here is counted at the proxy, from the provider's own answer. A
// harness reporting its usage is a claim; the answer the provider sent back is
// the count. The ledger, the caps and the report all read the same lines of
// out/proxy.jsonl, so they cannot disagree.
package spend

import "strings"

// Call is one line of out/proxy.jsonl: what went out, what came back, and what
// it cost.
//
// The token counts and the cost are pointers written without omitempty, so an
// answer that carried no usage, or a model with no price, is null in the file.
// A zero would read as free, and nobody would notice the number was missing.
type Call struct {
	Path   string `json:"path"`
	Status int    `json:"status,omitempty"`
	Bytes  int64  `json:"bytes,omitempty"`
	Took   int64  `json:"tookMs"`
	// Credential names the key the proxy added. The key itself is never here.
	Credential string `json:"credential,omitempty"`
	// Registry is the version of the organisation's model registry the call was sent under.
	Registry int64  `json:"registry,omitempty"`
	Error    string `json:"error,omitempty"`
	AtUnix   int64  `json:"atUnix"`
	// Translated is the shape of API the harness called in when the call was translated into Chat Completions for
	// a provider that answers only that: "messages" or "responses". It is empty for a call that went as it came.
	Translated string `json:"translated,omitempty"`
	// Model is what the request asked for, and the name the price is found by.
	Model        string `json:"model,omitempty"`
	InputTokens  *int64 `json:"inputTokens"`
	OutputTokens *int64 `json:"outputTokens"`
	// CachedTokens is the part of InputTokens the provider read from its cache
	// and billed at its cache rate. A line written before it was counted has
	// none, and is priced as if nothing was cached.
	CachedTokens *int64   `json:"cachedTokens,omitempty"`
	USD          *float64 `json:"usd"`
	// Redacted is how many of each kind of data were masked in the request before it left, and RedactedAnswer in what came back,
	// by the name of the kind. What was masked is not kept.
	Redacted       map[string]int `json:"redacted,omitempty"`
	RedactedAnswer map[string]int `json:"redactedAnswer,omitempty"`
	// Host is where a request other than a model call was going, as
	// host:port. Such a request costs no model money and is not counted.
	Host string `json:"host,omitempty"`
	// Denied says why a request to Host was refused rather than sent.
	Denied string `json:"denied,omitempty"`
	// Refused is the cap that kept this call from leaving. A refused call sent
	// nothing, so its tokens and cost are a measured zero rather than null.
	Refused *Refusal `json:"refused,omitempty"`
}

// ToModel says whether this line is a call to the model, the only kind that
// spends.
func (c Call) ToModel() bool { return c.Host == "" }

// Measured says whether the cost of this call is known.
func (c Call) Measured() bool { return c.USD != nil }

// Counted says whether the provider's answer said what the call used.
func (c Call) Counted() bool { return c.InputTokens != nil && c.OutputTokens != nil }

// Named is what a report calls the model this call went to: the credential
// and the model, as a job names it. A call that named no model, such as a
// harness listing the models on offer, is named by its path instead.
func (c Call) Named() string {
	if c.Model == "" {
		return strings.TrimSpace(c.Credential + " " + c.Path)
	}
	if c.Credential == "" {
		return c.Model
	}
	return c.Credential + "/" + c.Model
}

// answered says whether the provider accepted the call. A successful answer
// with no usage in it is the one a cap cannot live with, because the provider
// billed something the proxy could not count.
func (c Call) answered() bool { return c.Status >= 200 && c.Status < 300 }

// billed says whether the provider charges for this call. An answer it turned
// away is not billed, and neither is a request that asked no model anything,
// such as a harness listing the models on offer. Only an accepted call to a
// model that carried no count is money nobody can see.
func (c Call) billed() bool { return c.answered() && c.Model != "" }

// Refusal says which cap stopped a call and why, in words the person who asked
// for the job can act on.
type Refusal struct {
	// Cap is "job", "team" or "organisation".
	Cap      string  `json:"cap"`
	Team     string  `json:"team,omitempty"`
	LimitUSD float64 `json:"limitUsd"`
	SpentUSD float64 `json:"spentUsd"`
	Reason   string  `json:"reason"`
}

// RefusedCall is the line a refused call leaves: a measured nothing.
func RefusedCall(path, model string, at int64, why *Refusal) Call {
	var none int64
	var free float64
	return Call{
		Path: path, Status: 402, AtUnix: at, Model: model,
		InputTokens: &none, OutputTokens: &none, USD: &free, Refused: why,
	}
}
