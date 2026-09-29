// Package repoaccess is how a job reaches a private repository: with a token
// for that repository alone, which expires, and which never enters the box.
//
// A GitHub App holds the access. When somebody asks for a repository, the App
// mints an installation token scoped to that one repository and to what the work
// needs, and the token is revoked when the work is done. Nothing long-lived sits
// on the machine but the App's key.
package repoaccess

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// Access is what a token may do to its repository.
type Access int

const (
	// Read is cloning.
	Read Access = iota
	// Write is pushing a branch and opening a pull request.
	Write
)

func (a Access) permissions() map[string]string {
	if a == Write {
		return map[string]string{"contents": "write", "pull_requests": "write"}
	}
	return map[string]string{"contents": "read"}
}

// NotInstalledError is a repository the App cannot reach. It names the
// repository, so whoever asked for it knows what to ask an owner for.
type NotInstalledError struct{ Repo string }

func (e NotInstalledError) Error() string {
	return fmt.Sprintf("the GitHub App is not installed on %s: ask an owner to add that repository to the App's installation", e.Repo)
}

// App is a GitHub App this deployment holds.
type App struct {
	id     string
	key    *rsa.PrivateKey
	api    string
	client *http.Client
	now    func() time.Time
}

// Token is a credential for one repository.
type Token struct {
	Value   string
	Expires time.Time
	app     *App
}

// OpenApp reads the App's private key, a PEM file as GitHub gives it.
func OpenApp(id, keyPath, api string) (*App, error) {
	text, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("the GitHub App's key could not be read: %w", err)
	}
	key, err := parseKey(text)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", keyPath, err)
	}
	return &App{id: id, key: key, api: api, client: &http.Client{Timeout: 30 * time.Second}, now: time.Now}, nil
}

func parseKey(text []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(text)
	if block == nil {
		return nil, errors.New("holds no PEM private key")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("is not an RSA private key")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("is not an RSA private key")
	}
	return key, nil
}

// jwt is how the App proves who it is to GitHub, for nine minutes, dated a
// minute back so a clock a little behind GitHub's is still believed.
func (a *App) jwt() (string, error) {
	now := a.now()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, err := json.Marshal(map[string]any{"iat": now.Add(-time.Minute).Unix(), "exp": now.Add(9 * time.Minute).Unix(), "iss": a.id})
	if err != nil {
		return "", err
	}
	signing := header + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signing))
	signature, err := rsa.SignPKCS1v15(rand.Reader, a.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

// installation is the App's installation on a repository, or NotInstalledError.
func (a *App) installation(ctx context.Context, owner, name string) (int64, error) {
	token, err := a.jwt()
	if err != nil {
		return 0, err
	}
	var found struct {
		ID int64 `json:"id"`
	}
	status, err := a.call(ctx, http.MethodGet, "/repos/"+owner+"/"+name+"/installation", token, nil, &found)
	switch {
	case status == http.StatusNotFound:
		return 0, NotInstalledError{Repo: owner + "/" + name}
	case err != nil:
		return 0, err
	}
	return found.ID, nil
}

// Check says whether the App can reach the repository, without minting a token.
func (a *App) Check(ctx context.Context, owner, name string) error {
	_, err := a.installation(ctx, owner, name)
	return err
}

// Token mints a token for this one repository and what the work needs of it.
func (a *App) Token(ctx context.Context, owner, name string, access Access) (Token, error) {
	id, err := a.installation(ctx, owner, name)
	if err != nil {
		return Token{}, err
	}
	signed, err := a.jwt()
	if err != nil {
		return Token{}, err
	}
	wanted, err := json.Marshal(map[string]any{"repositories": []string{name}, "permissions": access.permissions()})
	if err != nil {
		return Token{}, err
	}
	var minted struct {
		Token   string    `json:"token"`
		Expires time.Time `json:"expires_at"`
	}
	if _, err := a.call(ctx, http.MethodPost, fmt.Sprintf("/app/installations/%d/access_tokens", id), signed, wanted, &minted); err != nil {
		return Token{}, err
	}
	if minted.Token == "" {
		return Token{}, errors.New("GitHub minted no token")
	}
	return Token{Value: minted.Token, Expires: minted.Expires, app: a}, nil
}

// Revoke ends the token now, rather than when it would have expired.
func (t Token) Revoke(ctx context.Context) error {
	_, err := t.app.call(ctx, http.MethodDelete, "/installation/token", t.Value, nil, nil)
	return err
}

// call sends one request as whoever the bearer is. It returns the status even
// when it fails, so a caller can tell a repository that is not there from a
// service that is not answering.
func (a *App) call(ctx context.Context, method, path, bearer string, body []byte, into any) (int, error) {
	request, err := http.NewRequestWithContext(ctx, method, a.api+path, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Authorization", "Bearer "+bearer)
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := a.client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("GitHub could not be reached: %w", err)
	}
	defer response.Body.Close()
	text, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return response.StatusCode, err
	}
	if response.StatusCode >= 300 {
		var said struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(text, &said)
		return response.StatusCode, fmt.Errorf("GitHub answered %d for %s %s: %s", response.StatusCode, method, path, said.Message)
	}
	if into != nil {
		if err := json.Unmarshal(text, into); err != nil {
			return response.StatusCode, fmt.Errorf("GitHub's answer for %s could not be read: %w", path, err)
		}
	}
	return response.StatusCode, nil
}
