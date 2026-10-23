package proxy

import (
	"context"
	"net/http"
)

// TokenSource says what a token stands for. Tokens is the one a task's runner uses: tokens it
// minted, held in memory, dropped when the task ends. A host that signs its tokens somewhere
// else gives the proxy its own with UseTokens.
type TokenSource interface {
	// Job says what the token stands for, or that it stands for nothing. The same answer for a
	// token that never existed and one that has ended, so a probe learns nothing.
	Job(token string) (Ticket, bool)
}

// CredentialSource says which credential a ticket may spend, at the moment of a call. Credentials
// is the one a deployment that holds its keys uses. A host whose keys belong to the people
// who run the tasks gives the proxy its own with UseCredentials, and fetches the key then.
type CredentialSource interface {
	// Credential is the credential this ticket may spend, or false when it may spend none.
	// A call in flight keeps the credential it began with.
	Credential(ctx context.Context, ticket Ticket) (Credential, bool)
}

var (
	_ TokenSource      = (*Tokens)(nil)
	_ CredentialSource = Credentials(nil)
)

// Credential is the credential of the ticket's name, under the rules of Get.
func (c Credentials) Credential(_ context.Context, ticket Ticket) (Credential, bool) {
	return c.Get(ticket.Credential)
}

// UseTokens has the proxy ask source what a token stands for, in place of the tokens it was made with.
func (p *Proxy) UseTokens(source TokenSource) { p.tokens.Store(&source) }

// UseCredentials has the proxy ask source for the credential of each call, in place of the
// credentials it holds. Passing nil goes back to the held credentials.
func (p *Proxy) UseCredentials(source CredentialSource) {
	if source == nil {
		p.source.Store(nil)
		return
	}
	p.source.Store(&source)
}

// UseClient has the proxy make its model calls with client, in place of the default transport.
// A host that guards where its providers may be reached from gives its own. A client that follows
// redirects would carry the credential's headers to wherever it is sent, so it must not.
func (p *Proxy) UseClient(client *http.Client) { p.client.Store(client) }

func (p *Proxy) tokenSource() TokenSource { return *p.tokens.Load() }

// credentialFor is the credential this ticket's call is made with.
func (p *Proxy) credentialFor(ctx context.Context, ticket Ticket) (Credential, bool) {
	if source := p.source.Load(); source != nil {
		return (*source).Credential(ctx, ticket)
	}
	return p.credentials().Get(ticket.Credential)
}
