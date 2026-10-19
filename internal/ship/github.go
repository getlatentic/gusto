package ship

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// GitHub opens pull requests through GitHub's REST API. Open is its only
// method, and it sends one request, so there is no call here that could do
// anything else to a pull request.
type GitHub struct {
	API    string
	Token  string
	Client *http.Client
}

// GitHubFromEnv reads the token from GUSTO_GITHUB_TOKEN and the API from
// GUSTO_GITHUB_API, which defaults to api.github.com. It is read before anything
// is pushed, so a missing token never leaves a branch behind with no pull
// request.
func GitHubFromEnv() (GitHub, error) {
	token := os.Getenv("GUSTO_GITHUB_TOKEN")
	if token == "" {
		return GitHub{}, errors.New("set GUSTO_GITHUB_TOKEN to a token that may push and open pull requests")
	}
	return GitHub{
		API:    strings.TrimSuffix(or(os.Getenv("GUSTO_GITHUB_API"), "https://api.github.com"), "/"),
		Token:  token,
		Client: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

type opening struct {
	Title               string `json:"title"`
	Head                string `json:"head"`
	Base                string `json:"base"`
	Body                string `json:"body"`
	MaintainerCanModify bool   `json:"maintainer_can_modify"`
}

type answer struct {
	HTMLURL string `json:"html_url"`
	Message string `json:"message"`
	Errors  []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// Open asks for the pull request and returns its address.
func (g GitHub) Open(ctx context.Context, pull Pull) (string, error) {
	body, err := json.Marshal(opening{Title: pull.Title, Head: pull.Head, Base: pull.Base, Body: pull.Body, MaintainerCanModify: true})
	if err != nil {
		return "", err
	}
	address := fmt.Sprintf("%s/repos/%s/%s/pulls", g.API, pull.Owner, pull.Name)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+g.Token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	response, err := g.client().Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	return opened(response)
}

func (g GitHub) client() *http.Client {
	if g.Client == nil {
		return http.DefaultClient
	}
	return g.Client
}

// opened reads the address out of GitHub's answer, or the reason it refused.
// GitHub puts the useful part of a refusal, such as a pull request that
// already exists for the branch, in errors rather than in message.
func opened(response *http.Response) (string, error) {
	var said answer
	if err := json.NewDecoder(response.Body).Decode(&said); err != nil {
		return "", fmt.Errorf("GitHub answered %s, and not in JSON", response.Status)
	}
	if response.StatusCode == http.StatusCreated && said.HTMLURL != "" {
		return said.HTMLURL, nil
	}
	reasons := []string{or(said.Message, response.Status)}
	for _, reason := range said.Errors {
		reasons = append(reasons, reason.Message)
	}
	return "", fmt.Errorf("GitHub refused the pull request: %s", strings.Join(reasons, "; "))
}
