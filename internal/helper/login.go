package helper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Login is the sign-in of a person's computer by the device flow: it asks the server for a code, shows it, and waits
// until the person has approved it on the server's own page, signed in as they always are. The computer never sees a
// password.
type Login struct {
	Server string
	// Label names this computer in the person's list of approved ones.
	Label string
	// Show is told where to go and what to type, once.
	Show func(address, code string)
	HTTP *http.Client
	// Wait sleeps between asks, and is replaced in a test.
	Wait func(context.Context, time.Duration) error
}

func (l Login) http() *http.Client {
	if l.HTTP != nil {
		return l.HTTP
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func sleep(ctx context.Context, wait time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
		return nil
	}
}

type answer struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	AccessToken     string `json:"access_token"`
	Person          string `json:"person"`
	Error           string `json:"error"`
}

func (l Login) post(ctx context.Context, path string, body any) (answer, int, error) {
	text, _ := json.Marshal(body)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(l.Server, "/")+path, bytes.NewReader(text))
	if err != nil {
		return answer{}, 0, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := l.http().Do(request)
	if err != nil {
		return answer{}, 0, err
	}
	defer response.Body.Close()
	var got answer
	_ = json.NewDecoder(response.Body).Decode(&got)
	return got, response.StatusCode, nil
}

// Run signs in and gives the token and the person it acts as.
func (l Login) Run(ctx context.Context) (token, person string, err error) {
	if l.Wait == nil {
		l.Wait = sleep
	}
	started, status, err := l.post(ctx, "/api/device", map[string]string{"label": l.Label})
	switch {
	case err != nil:
		return "", "", err
	case status == http.StatusNotFound:
		return "", "", errors.New("this server takes no tokens for a person: its administrator has to turn on API tokens (HAAS_API_TOKENS)")
	case status == http.StatusTooManyRequests:
		return "", "", errors.New("the server is asked for too many sign-ins from here: wait a few minutes and try again")
	case status != http.StatusOK || started.DeviceCode == "":
		return "", "", fmt.Errorf("the server did not start a sign-in: it answered %d", status)
	}
	address := started.VerificationURI
	if parsed, err := url.Parse(address); err == nil {
		query := parsed.Query()
		query.Set("code", started.UserCode)
		parsed.RawQuery = query.Encode()
		address = parsed.String()
	}
	if l.Show != nil {
		l.Show(address, started.UserCode)
	}
	every := time.Duration(max(started.Interval, 1)) * time.Second
	deadline := time.Now().Add(time.Duration(started.ExpiresIn) * time.Second)
	for time.Now().Before(deadline) {
		if err := l.Wait(ctx, every); err != nil {
			return "", "", err
		}
		got, status, err := l.post(ctx, "/api/device/token", map[string]string{"device_code": started.DeviceCode})
		switch {
		case err != nil:
			return "", "", err
		case status == http.StatusOK && got.AccessToken != "":
			return got.AccessToken, got.Person, nil
		case got.Error == "authorization_pending":
		case got.Error == "slow_down":
			every += 5 * time.Second
		case got.Error == "expired_token":
			return "", "", errors.New("the code expired before it was approved: run `haas login` again")
		default:
			return "", "", fmt.Errorf("the server refused the sign-in: %d %s", status, got.Error)
		}
	}
	return "", "", errors.New("the code expired before it was approved: run `haas login` again")
}
