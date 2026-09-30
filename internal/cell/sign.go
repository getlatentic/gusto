// Package cell is how the front door runs organisations' servers on other hosts. A host that runs
// them has an agent, which the front door asks over HTTP with requests signed by a secret only the
// front door and the cells hold, and which registers itself in a directory all of them see, so that
// adding a host is starting one.
package cell

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"
)

const (
	timeHeader      = "X-Cell-Time"
	signatureHeader = "X-Cell-Signature"
	// skew is how far a request's time may be from the receiver's, which is what limits how long a
	// request that was seen can be sent again.
	skew    = time.Minute
	maxBody = 1 << 20
)

// mac is the signature of a request: its method, everything after the host in its address, the
// time it was made and what it carries.
func mac(secret []byte, method, target string, at int64, body []byte) string {
	sum := sha256.Sum256(body)
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(method + "\n" + target + "\n" + strconv.FormatInt(at, 10) + "\n" + hex.EncodeToString(sum[:])))
	return hex.EncodeToString(m.Sum(nil))
}

// sign puts the time and the signature on a request that is about to be sent.
func sign(r *http.Request, secret []byte, now time.Time, body []byte) {
	r.Header.Set(timeHeader, strconv.FormatInt(now.Unix(), 10))
	r.Header.Set(signatureHeader, mac(secret, r.Method, r.URL.RequestURI(), now.Unix(), body))
}

var errUnsigned = errors.New("the request is not signed by the front door")

// verify reads a request's body and says whether it was signed with the secret, recently. The body is
// put back for the handler.
func verify(r *http.Request, secret []byte, now time.Time) ([]byte, error) {
	at, err := strconv.ParseInt(r.Header.Get(timeHeader), 10, 64)
	if err != nil || now.Sub(time.Unix(at, 0)).Abs() > skew {
		return nil, errUnsigned
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
	if err != nil || len(body) > maxBody {
		return nil, errors.New("the request is too large")
	}
	given, err := hex.DecodeString(r.Header.Get(signatureHeader))
	want, _ := hex.DecodeString(mac(secret, r.Method, r.URL.RequestURI(), at, body))
	if err != nil || len(secret) == 0 || !hmac.Equal(given, want) {
		return nil, errUnsigned
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}
