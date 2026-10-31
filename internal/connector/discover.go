package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Server is what an app's MCP server says of how to sign in to it: the resource the token is for, and its authorization
// server's endpoints and what that server supports.
type Server struct {
	Resource      string   `json:"resource"`
	Issuer        string   `json:"issuer"`
	Authorize     string   `json:"authorize"`
	Token         string   `json:"token"`
	Registration  string   `json:"registration,omitempty"`
	ClientDocs    bool     `json:"clientDocs,omitempty"`
	Scopes        []string `json:"scopes,omitempty"`
	ChallengeS256 bool     `json:"challengeS256"`
}

// mostMetadata is the most of a metadata document read; a real one is a few kilobytes.
const mostMetadata = 256 << 10

// Discover finds how to sign in to the MCP server at address. It asks the server, and reads the resource metadata its 401 names,
// or the well-known address when it names none (RFC 9728); then the authorization server's metadata (RFC 8414, or OpenID
// Connect discovery).
func Discover(ctx context.Context, client *http.Client, address string) (Server, error) {
	metadataAt, scope, err := challenge(ctx, client, address)
	if err != nil {
		return Server{}, err
	}
	candidates := []string{}
	if metadataAt != "" {
		candidates = append(candidates, metadataAt)
	}
	candidates = append(candidates, wellKnown(address, "oauth-protected-resource", true), wellKnown(address, "oauth-protected-resource", false))
	var resource struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
		ScopesSupported      []string `json:"scopes_supported"`
	}
	for _, each := range candidates {
		if getJSON(ctx, client, each, &resource) == nil && len(resource.AuthorizationServers) > 0 {
			break
		}
	}
	if len(resource.AuthorizationServers) == 0 {
		return Server{}, fmt.Errorf("%s names no authorization server", address)
	}
	found := Server{Resource: firstOf(resource.Resource, address)}
	if scope != "" {
		found.Scopes = strings.Fields(scope)
	} else {
		found.Scopes = resource.ScopesSupported
	}
	issuer := resource.AuthorizationServers[0]
	var meta struct {
		Issuer           string   `json:"issuer"`
		Authorize        string   `json:"authorization_endpoint"`
		Token            string   `json:"token_endpoint"`
		Registration     string   `json:"registration_endpoint"`
		ClientDocs       bool     `json:"client_id_metadata_document_supported"`
		ChallengeMethods []string `json:"code_challenge_methods_supported"`
	}
	for _, each := range []string{
		wellKnown(issuer, "oauth-authorization-server", true), wellKnown(issuer, "openid-configuration", true),
		strings.TrimSuffix(issuer, "/") + "/.well-known/openid-configuration",
	} {
		if getJSON(ctx, client, each, &meta) == nil && meta.Authorize != "" && meta.Token != "" {
			break
		}
	}
	if meta.Authorize == "" || meta.Token == "" {
		return Server{}, fmt.Errorf("the authorization server %s says nothing of how to sign in", issuer)
	}
	found.Issuer, found.Authorize, found.Token, found.Registration, found.ClientDocs = firstOf(meta.Issuer, issuer), meta.Authorize, meta.Token, meta.Registration, meta.ClientDocs
	for _, method := range meta.ChallengeMethods {
		found.ChallengeS256 = found.ChallengeS256 || method == "S256"
	}
	return found, nil
}

// challenge asks the MCP server without a token and reads the 401 it answers: where its resource metadata is, and the scope it
// asks for. A server that does not answer 401 cannot be signed in to.
func challenge(ctx context.Context, client *http.Client, address string) (metadata, scope string, err error) {
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"gusto","version":"1"}}}`
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, address, strings.NewReader(body))
	if err != nil {
		return "", "", err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	answer, err := client.Do(request)
	if err != nil {
		return "", "", err
	}
	io.Copy(io.Discard, io.LimitReader(answer.Body, mostMetadata))
	answer.Body.Close()
	if answer.StatusCode != http.StatusUnauthorized {
		return "", "", fmt.Errorf("%s answered %s, not a request to sign in", address, answer.Status)
	}
	for _, header := range answer.Header.Values("WWW-Authenticate") {
		if value, found := authParam(header, "resource_metadata"); found {
			metadata = value
		}
		if value, found := authParam(header, "scope"); found {
			scope = value
		}
	}
	return metadata, scope, nil
}

// authParam is one parameter of a WWW-Authenticate challenge, quoted or not.
func authParam(header, name string) (string, bool) {
	at := strings.Index(header, name+"=")
	if at < 0 {
		return "", false
	}
	rest := header[at+len(name)+1:]
	if strings.HasPrefix(rest, `"`) {
		end := strings.Index(rest[1:], `"`)
		if end < 0 {
			return "", false
		}
		return rest[1 : end+1], true
	}
	end := strings.IndexAny(rest, ", ")
	if end < 0 {
		return rest, true
	}
	return rest[:end], true
}

// wellKnown is a well-known address for a URL: with the URL's path after the well-known name (RFC 8414 and 9728), or at the root.
func wellKnown(address, name string, withPath bool) string {
	parsed, err := url.Parse(address)
	if err != nil {
		return ""
	}
	path := strings.TrimSuffix(parsed.Path, "/")
	if !withPath {
		path = ""
	}
	return parsed.Scheme + "://" + parsed.Host + "/.well-known/" + name + path
}

// getJSON reads a JSON document over https, or over http for this computer only, which is how a test and a local provider are reached.
func getJSON(ctx context.Context, client *http.Client, address string, into any) error {
	if !secureAddress(address) {
		return fmt.Errorf("%s is not an https address", address)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	answer, err := client.Do(request)
	if err != nil {
		return err
	}
	defer answer.Body.Close()
	if answer.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %s", address, answer.Status)
	}
	return json.NewDecoder(io.LimitReader(answer.Body, mostMetadata)).Decode(into)
}

// secureAddress is an https address, or an http one on this computer.
func secureAddress(address string) bool {
	parsed, err := url.Parse(address)
	if err != nil {
		return false
	}
	host := parsed.Hostname()
	return parsed.Scheme == "https" || (parsed.Scheme == "http" && (host == "localhost" || host == "127.0.0.1" || host == "::1"))
}

func firstOf(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

var errNoClient = errors.New("this app has no way for gusto to register as a client, so the operator registers one and gives its id")
