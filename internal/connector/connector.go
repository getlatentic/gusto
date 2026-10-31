package connector

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/getlatentic/gusto/pkg/gateway"
)

// Client is how this server is known to an app: an id, and a secret when the app gave one. The operator registers it for an app
// that cannot register clients itself.
type Client struct {
	ID     string `json:"id"`
	Secret string `json:"secret,omitempty"`
}

// Config is what a Connectors needs: where it keeps what it holds, this server's own address, which sign-ins come back to, and the
// clients the operator registered with the apps that need one, by connector id.
type Config struct {
	Dir     string
	Base    string
	Clients map[string]Client
	// Apps is the connectors offered, the catalog when it is empty.
	Apps []Entry
	// HTTP calls the apps. Unset, the calls reach public addresses only, unless Private allows a private network, and give up
	// after callTime.
	HTTP    *http.Client
	Private bool
}

// callTime is the most one call to an app's server may take.
const callTime = 20 * time.Second

// signInTime is how long a person has to finish signing in at the app once they set out.
const signInTime = 10 * time.Minute

// mostPending is the most sign-ins under way at once; past it the oldest is forgotten.
const mostPending = 1000

// Connectors signs people in to apps and keeps their tokens, sealed, one set per person and connector.
type Connectors struct {
	config Config
	http   *http.Client
	vault  *vault

	mu      sync.Mutex
	pending map[string]pending
	// refreshing makes one refresh of a person's tokens at a time; a call that waits for it finds the new token.
	refreshing map[string]*sync.Mutex
}

// pending is a sign-in a person set out on and has not come back from.
type pending struct {
	Person, Connector string
	Verifier          string
	Client            Client
	Server            Server
	Redirect          string
	Started           time.Time
}

// tokens is what an app gave a person, kept sealed.
type tokens struct {
	Access  string    `json:"access"`
	Refresh string    `json:"refresh,omitempty"`
	Expires time.Time `json:"expires,omitzero"`
	Client  Client    `json:"client"`
	Server  Server    `json:"server"`
	Since   time.Time `json:"since"`
}

// Open makes the store over its folder.
func Open(config Config) (*Connectors, error) {
	if config.Base == "" {
		return nil, errors.New("connectors need this server's address, which sign-ins come back to")
	}
	if len(config.Apps) == 0 {
		config.Apps = Catalog
	}
	for id := range config.Clients {
		if _, known := find(config.Apps, id); !known {
			return nil, fmt.Errorf("a client is given for %s, which is no connector here", id)
		}
	}
	held, err := openVault(config.Dir)
	if err != nil {
		return nil, err
	}
	client := config.HTTP
	if client == nil {
		client = &http.Client{Timeout: callTime, Transport: &http.Transport{DialContext: gateway.PublicDialer(config.Private), ResponseHeaderTimeout: callTime},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return &Connectors{config: config, http: client, vault: held, pending: map[string]pending{}, refreshing: map[string]*sync.Mutex{}}, nil
}

// Apps is the connectors offered here.
func (c *Connectors) Apps() []Entry { return c.config.Apps }

// App is one connector offered here, by its id.
func (c *Connectors) App(id string) (Entry, bool) { return find(c.config.Apps, id) }

// Usable says whether people can sign in to a connector from this server: one that needs a client the operator registers has one.
func (c *Connectors) Usable(entry Entry) bool {
	return !entry.NeedsClient || c.config.Clients[entry.ID].ID != ""
}

// ClientDocument is the client ID metadata document this server is known by to an app that reads one: the address of the
// document is the client's id, and it names where a sign-in comes back to.
func (c *Connectors) ClientDocument() map[string]any {
	return map[string]any{
		"client_id": c.clientDocumentURL(), "client_name": "gusto", "client_uri": c.config.Base,
		"redirect_uris": []string{c.redirect()}, "grant_types": []string{"authorization_code", "refresh_token"},
		"response_types": []string{"code"}, "token_endpoint_auth_method": "none",
	}
}

func (c *Connectors) clientDocumentURL() string {
	return strings.TrimSuffix(c.config.Base, "/") + "/connectors/client.json"
}

func (c *Connectors) redirect() string {
	return strings.TrimSuffix(c.config.Base, "/") + "/connections/callback"
}

// Begin sets a person out to sign in to a connector, and gives the address at the app to send them to.
func (c *Connectors) Begin(ctx context.Context, person string, entry Entry) (string, error) {
	server, err := Discover(ctx, c.http, entry.URL)
	if err != nil {
		return "", err
	}
	if !server.ChallengeS256 {
		return "", fmt.Errorf("%s does not say it takes a PKCE challenge, which signing in here needs", entry.Name)
	}
	client, err := c.clientFor(ctx, entry, server)
	if err != nil {
		return "", err
	}
	state, verifier := randomText(), randomText()
	sum := sha256.Sum256([]byte(verifier))
	c.remember(state, pending{Person: strings.ToLower(person), Connector: entry.ID, Verifier: verifier, Client: client, Server: server,
		Redirect: c.redirect(), Started: time.Now()})
	query := url.Values{
		"response_type": {"code"}, "client_id": {client.ID}, "redirect_uri": {c.redirect()}, "state": {state},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"}, "resource": {server.Resource},
	}
	scopes := entry.Scopes
	if len(scopes) == 0 {
		scopes = server.Scopes
	}
	if len(scopes) > 0 {
		query.Set("scope", strings.Join(scopes, " "))
	}
	join := "?"
	if strings.Contains(server.Authorize, "?") {
		join = "&"
	}
	return server.Authorize + join + query.Encode(), nil
}

// Finish takes the code a sign-in came back with, for the person it was begun for, trades it for tokens and keeps them. It says
// which connector the person is now signed in to.
func (c *Connectors) Finish(ctx context.Context, person, state, code string) (string, error) {
	begun, found := c.take(state)
	if !found || begun.Person != strings.ToLower(person) || time.Since(begun.Started) > signInTime {
		return "", errors.New("that sign-in is not one under way here: start it again")
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {begun.Redirect},
		"code_verifier": {begun.Verifier}, "resource": {begun.Server.Resource}}
	got, err := c.ask(ctx, begun.Server, begun.Client, form)
	if err != nil {
		return "", err
	}
	got.Client, got.Server, got.Since = begun.Client, begun.Server, time.Now()
	return begun.Connector, c.vault.put(tokenName(begun.Person, begun.Connector), got)
}

// Token is the person's access token for a connector, refreshed first when it has run out or is about to.
func (c *Connectors) Token(ctx context.Context, person, connector string) (string, error) {
	name := tokenName(strings.ToLower(person), connector)
	lock := c.lockFor(name)
	lock.Lock()
	defer lock.Unlock()
	var held tokens
	found, err := c.vault.get(name, &held)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("%s is not signed in to %s", person, connector)
	}
	if held.Expires.IsZero() || time.Until(held.Expires) > time.Minute {
		return held.Access, nil
	}
	if held.Refresh == "" {
		return "", fmt.Errorf("%s's sign-in to %s ran out: sign in again", person, connector)
	}
	fresh, err := c.ask(ctx, held.Server, held.Client, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {held.Refresh}, "resource": {held.Server.Resource}})
	if err != nil {
		return "", fmt.Errorf("%s's sign-in to %s could not be renewed: %w", person, connector, err)
	}
	if fresh.Refresh == "" {
		fresh.Refresh = held.Refresh
	}
	fresh.Client, fresh.Server, fresh.Since = held.Client, held.Server, held.Since
	return fresh.Access, c.vault.put(name, fresh)
}

// Connected says whether a person is signed in to a connector, and since when.
func (c *Connectors) Connected(person, connector string) (time.Time, bool) {
	var held tokens
	found, err := c.vault.get(tokenName(strings.ToLower(person), connector), &held)
	if err != nil || !found {
		return time.Time{}, false
	}
	return held.Since, true
}

// Disconnect forgets a person's tokens for a connector. The app keeps its own record of the sign-in until the person ends it there.
func (c *Connectors) Disconnect(person, connector string) error {
	return c.vault.remove(tokenName(strings.ToLower(person), connector))
}

// clientFor is how this server is known to the app: the operator's client when they registered one, else this server's client
// document when the app reads one, else a client the app registered for this server, which is kept for next time.
func (c *Connectors) clientFor(ctx context.Context, entry Entry, server Server) (Client, error) {
	if client, found := c.config.Clients[entry.ID]; found && client.ID != "" {
		return client, nil
	}
	if server.ClientDocs && strings.HasPrefix(c.config.Base, "https://") {
		return Client{ID: c.clientDocumentURL()}, nil
	}
	if server.Registration == "" {
		return Client{}, errNoClient
	}
	name := "clients/" + hashOf(server.Issuer+" "+c.redirect()) + ".json"
	var kept Client
	if found, err := c.vault.get(name, &kept); err == nil && found {
		return kept, nil
	}
	registered, err := c.register(ctx, server)
	if err != nil {
		return Client{}, err
	}
	return registered, c.vault.put(name, registered)
}

// register asks the app to register this server as a client (RFC 7591), as a public client that proves itself with PKCE.
func (c *Connectors) register(ctx context.Context, server Server) (Client, error) {
	body, _ := json.Marshal(map[string]any{
		"client_name": "gusto", "client_uri": c.config.Base, "redirect_uris": []string{c.redirect()},
		"grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}, "token_endpoint_auth_method": "none",
	})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.Registration, strings.NewReader(string(body)))
	if err != nil {
		return Client{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	answer, err := c.http.Do(request)
	if err != nil {
		return Client{}, err
	}
	defer answer.Body.Close()
	var got struct {
		ID     string `json:"client_id"`
		Secret string `json:"client_secret"`
	}
	if answer.StatusCode != http.StatusCreated && answer.StatusCode != http.StatusOK {
		return Client{}, fmt.Errorf("the app would not register gusto as a client: %s", answer.Status)
	}
	if err := json.NewDecoder(io.LimitReader(answer.Body, mostMetadata)).Decode(&got); err != nil || got.ID == "" {
		return Client{}, errors.New("the app registered gusto and gave no client id")
	}
	return Client{ID: got.ID, Secret: got.Secret}, nil
}

// ask calls the app's token endpoint, as the client, and reads the tokens it answers with.
func (c *Connectors) ask(ctx context.Context, server Server, client Client, form url.Values) (tokens, error) {
	if !secureAddress(server.Token) {
		return tokens{}, fmt.Errorf("%s is not an https address", server.Token)
	}
	if client.Secret == "" || server.SecretInBody {
		form.Set("client_id", client.ID)
	}
	if client.Secret != "" && server.SecretInBody {
		form.Set("client_secret", client.Secret)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.Token, strings.NewReader(form.Encode()))
	if err != nil {
		return tokens{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	if client.Secret != "" && !server.SecretInBody {
		request.SetBasicAuth(url.QueryEscape(client.ID), url.QueryEscape(client.Secret))
	}
	answer, err := c.http.Do(request)
	if err != nil {
		return tokens{}, err
	}
	defer answer.Body.Close()
	var got struct {
		Access    string `json:"access_token"`
		Refresh   string `json:"refresh_token"`
		ExpiresIn int64  `json:"expires_in"`
		Error     string `json:"error"`
		Said      string `json:"error_description"`
	}
	if err := json.NewDecoder(io.LimitReader(answer.Body, mostMetadata)).Decode(&got); err != nil {
		return tokens{}, fmt.Errorf("the app answered %s with no tokens", answer.Status)
	}
	if answer.StatusCode != http.StatusOK || got.Access == "" {
		return tokens{}, fmt.Errorf("the app refused: %s", firstOf(got.Said, got.Error, answer.Status))
	}
	held := tokens{Access: got.Access, Refresh: got.Refresh}
	if got.ExpiresIn > 0 {
		held.Expires = time.Now().Add(time.Duration(got.ExpiresIn) * time.Second)
	}
	return held, nil
}

func (c *Connectors) remember(state string, begun pending) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.pending) >= mostPending {
		oldest, at := "", time.Now()
		for key, each := range c.pending {
			if each.Started.Before(at) {
				oldest, at = key, each.Started
			}
		}
		delete(c.pending, oldest)
	}
	c.pending[state] = begun
}

func (c *Connectors) take(state string) (pending, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	begun, found := c.pending[state]
	delete(c.pending, state)
	return begun, found
}

func (c *Connectors) lockFor(name string) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.refreshing[name] == nil {
		c.refreshing[name] = &sync.Mutex{}
	}
	return c.refreshing[name]
}

// tokenName is where a person's tokens for a connector are kept; the address is hashed, so the folder names nobody, and the
// connector's id is hashed too, so no id can name a place outside the folder.
func tokenName(person, connector string) string {
	return "tokens/" + hashOf(person) + "/" + hashOf(connector) + ".json"
}

func hashOf(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:16])
}

func randomText() string {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}
