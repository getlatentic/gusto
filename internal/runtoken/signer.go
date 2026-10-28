package runtoken

import (
	"bufio"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// A Prover makes the DPoP proof for one request. The forwarder asks one for every request it sends, and never holds the key.
type Prover func(method, url, token string, now time.Time) (string, error)

// KeyProver proves with a key in this process.
func KeyProver(key ed25519.PrivateKey) Prover {
	return func(method, url, token string, now time.Time) (string, error) {
		return Prove(key, method, url, token, now)
	}
}

// proofAsked and proofGiven are one line each between a forwarder and its signer.
type proofAsked struct {
	Method string `json:"htm"`
	URL    string `json:"htu"`
	Token  string `json:"token,omitempty"`
	At     int64  `json:"iat"`
}

type proofGiven struct {
	Proof string `json:"proof,omitempty"`
	Error string `json:"error,omitempty"`
}

// mostAskedBytes bounds one line a signer reads: a request names a method, an address and a token of about a kilobyte.
const mostAskedBytes = 64 << 10

// ServeSigner makes a key that never leaves this process and answers each proof asked for on in, one line each, until in
// ends. The signer's process makes no network calls: a sandbox that reads a process's memory to decide its connections
// would otherwise have to read the process that holds the key.
func ServeSigner(in io.Reader, out io.Writer) error {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	lines := bufio.NewScanner(in)
	lines.Buffer(make([]byte, 0, 4<<10), mostAskedBytes)
	answers := json.NewEncoder(out)
	for lines.Scan() {
		if err := answers.Encode(proofFor(key, lines.Bytes())); err != nil {
			return err
		}
	}
	return lines.Err()
}

func proofFor(key ed25519.PrivateKey, line []byte) proofGiven {
	var asked proofAsked
	if err := json.Unmarshal(line, &asked); err != nil || asked.Method == "" || asked.URL == "" {
		return proofGiven{Error: "the request for a proof cannot be read"}
	}
	proof, err := Prove(key, asked.Method, asked.URL, asked.Token, time.Unix(asked.At, 0))
	if err != nil {
		return proofGiven{Error: err.Error()}
	}
	return proofGiven{Proof: proof}
}

// SignerProver asks the signer at the other end of to and from for each proof, one at a time.
func SignerProver(to io.Writer, from io.Reader) Prover {
	var mu sync.Mutex
	asks := json.NewEncoder(to)
	answers := bufio.NewScanner(from)
	answers.Buffer(make([]byte, 0, 4<<10), mostAskedBytes)
	return func(method, url, token string, now time.Time) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		if err := asks.Encode(proofAsked{Method: method, URL: url, Token: token, At: now.Unix()}); err != nil {
			return "", fmt.Errorf("the signer cannot be asked: %w", err)
		}
		if !answers.Scan() {
			return "", errors.Join(errors.New("the signer did not answer"), answers.Err())
		}
		var given proofGiven
		if err := json.Unmarshal(answers.Bytes(), &given); err != nil {
			return "", fmt.Errorf("the signer's answer cannot be read: %w", err)
		}
		if given.Error != "" {
			return "", errors.New(given.Error)
		}
		return given.Proof, nil
	}
}
