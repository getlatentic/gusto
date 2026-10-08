package helper

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client speaks to one server as the person a token acts as.
type Client struct {
	Server string
	Token  string
	HTTP   *http.Client
}

// Who is what the server says a token acts as.
type Who struct {
	Person string `json:"person"`
	Role   string `json:"role"`
}

func (c Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// do sends one request and gives its body when the server answers with one of want.
func (c Client) do(ctx context.Context, method, path string, want ...int) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.Server, "/")+path, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+c.Token)
	response, err := c.http().Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	for _, status := range want {
		if response.StatusCode == status {
			return body, nil
		}
	}
	if response.StatusCode == http.StatusUnauthorized {
		return nil, fmt.Errorf("%w: run `haas login` again", ErrSignedOut)
	}
	return nil, fmt.Errorf("%s %s answered %d", method, path, response.StatusCode)
}

// Me asks the server who the token acts as, which is how a computer learns it is still signed in.
func (c Client) Me(ctx context.Context) (Who, error) {
	body, err := c.do(ctx, http.MethodGet, "/api/me", http.StatusOK)
	if err != nil {
		return Who{}, err
	}
	var who Who
	return who, json.Unmarshal(body, &who)
}

// End asks the server to end the token, so that signing out leaves nothing that still works.
func (c Client) End(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodDelete, "/api/me", http.StatusNoContent, http.StatusOK)
	return err
}
