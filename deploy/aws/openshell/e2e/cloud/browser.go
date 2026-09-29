package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// visitor is one person's browser: its own cookies, and every address it is given reaches
// the front door on this machine whatever name it carries, as a name in DNS would.
type visitor struct {
	email  string
	client *http.Client
}

type page struct {
	status int
	url    string
	body   string
}

func newVisitor(email, door string) *visitor {
	jar, _ := cookiejar.New(nil)
	dial := &net.Dialer{Timeout: 5 * time.Second}
	return &visitor{email: email, client: &http.Client{
		Jar:     jar,
		Timeout: 60 * time.Second,
		Transport: &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return dial.DialContext(ctx, network, door)
		}},
	}}
}

func (v *visitor) get(address string) (page, error) {
	response, err := v.client.Get(address)
	return read(response, err)
}

// post sends a form the way a page in a browser does, from the address's own origin.
func (v *visitor) post(address string, form url.Values) (page, error) {
	request, err := http.NewRequest("POST", address, strings.NewReader(form.Encode()))
	if err != nil {
		return page{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Origin", originOf(address))
	response, err := v.client.Do(request)
	return read(response, err)
}

// where is the address a request to this one is redirected to, without following it.
func (v *visitor) where(address string) (string, error) {
	once := *v.client
	once.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := once.Get(address)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	return response.Header.Get("Location"), nil
}

func read(response *http.Response, err error) (page, error) {
	if err != nil {
		return page{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	return page{status: response.StatusCode, url: response.Request.URL.String(), body: string(body)}, err
}

func originOf(address string) string {
	parsed, _ := url.Parse(address)
	return parsed.Scheme + "://" + parsed.Host
}
