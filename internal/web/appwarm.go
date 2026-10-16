package web

import (
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/getlatentic/gusto/internal/apps"
)

// A new cell's gateway has been seen to take a request for the first app on it and never answer, while a request made
// afterwards on a connection of its own was answered at once. So for the first minutes of an app's life a request that is
// safe to ask again is given a short time to be answered, and is asked again on a new connection when it is not.
const (
	// youngApp is how long after it starts an app is treated so.
	youngApp = 5 * time.Minute
	// firstByte is how long a young app has to begin to answer, and askAgain how many times it is asked.
	firstByte = 8 * time.Second
	askAgain  = 3
)

// appTransport is how a request is sent on to an app: patiently for one that has been up a while, which may be slow for
// reasons of its own, and by asking again for a young one.
func (s *Server) appTransport(app apps.App) http.RoundTripper {
	if time.Since(time.Unix(app.Started, 0)) > youngApp {
		return http.DefaultTransport
	}
	wait := s.appFirstByte
	if wait <= 0 {
		wait = firstByte
	}
	return &askingAgain{wait: wait, base: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
		ResponseHeaderTimeout: wait,
		DisableKeepAlives:     true,
	}}
}

// askingAgain asks again, on a new connection, a request that reads and does not change anything, when it gets no
// answer in time. Anything else is sent once.
type askingAgain struct {
	wait time.Duration
	base http.RoundTripper
}

func (a *askingAgain) RoundTrip(r *http.Request) (*http.Response, error) {
	if !safe(r) {
		return a.base.RoundTrip(r)
	}
	var err error
	for range askAgain {
		var response *http.Response
		if response, err = a.base.RoundTrip(r); err == nil {
			return response, nil
		}
		var timeout net.Error
		if r.Context().Err() != nil || !(errors.As(err, &timeout) && timeout.Timeout()) {
			return nil, err
		}
	}
	return nil, err
}

func safe(r *http.Request) bool {
	reads := r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions
	return reads && (r.Body == nil || r.Body == http.NoBody)
}
