// Package apitoken is the tokens a script or a CI job presents to the API in
// place of the shared password. Each has a name, a list of what it may do, and
// a maker, and can be revoked on its own, so a token that leaks costs one
// integration and not everybody's sign-in.
//
// Only a hash of the secret is kept. The secret is shown once, when the token is
// made, and cannot be read back.
package apitoken

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// Scopes are what a token may do. A token has at least one.
const (
	// Submit sends a job to the API.
	Submit = "submit"
	// Spend reads the spend report.
	Spend = "spend"
	// Audit reads the record of what changed on the platform.
	Audit = "audit"
	// Metrics reads GET /metrics.
	Metrics = "metrics"
)

// Work is what a token a person's own computer holds may do: act as that person, on their own work and nobody else's.
// It is not one of Scopes, since an admin cannot grant it from a form: it is made only for a person who approved it.
const Work = "work"

// Scopes is every scope an admin may grant, in the order a person reads them.
var Scopes = []string{Submit, Spend, Audit, Metrics}

// Prefix starts every secret, so one found in a repository or a log is known for
// what it is.
const Prefix = "haas_"

var (
	// ErrUnknown is a token nobody made, or that is not a token at all.
	ErrUnknown = errors.New("that is not a token this server made")
	// ErrRevoked is a token that was made and has been ended.
	ErrRevoked = errors.New("that token has been revoked")
	// ErrName is a name that will not do.
	ErrName = errors.New("name a token in lowercase letters, digits and dashes, so a record can say which one it was")
)

var plain = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,59}$`)

// Token is one token as the file keeps it.
type Token struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
	// Hash is the SHA-256 of the secret's random part.
	Hash string `json:"hash"`
	// Person is who a token with the Work scope acts as. Empty for the organisation's tokens.
	Person    string `json:"person,omitempty"`
	Made      int64  `json:"made"`
	By        string `json:"by"`
	Revoked   int64  `json:"revoked,omitempty"`
	RevokedBy string `json:"revokedBy,omitempty"`
	LastUsed  int64  `json:"lastUsed,omitempty"`
}

// Live is whether the token still works.
func (t Token) Live() bool { return t.Revoked == 0 }

// May says whether the token has the scope.
func (t Token) May(scope string) bool { return slices.Contains(t.Scopes, scope) }

// Store is the tokens, kept in a file. It reads the file again when another
// process changes it, so a token made by one is known to the other at once.
type Store struct {
	mu     sync.Mutex
	path   string
	tokens []Token
	seen   time.Time
	size   int64
	// written is when last-used times were last saved. They are saved at most
	// once a minute, because a token in constant use would write the file on
	// every call.
	written time.Time
	now     func() time.Time
}

// Open reads the tokens at path, or starts with none when there is no file.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	s := &Store{path: path, now: time.Now}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	info, err := os.Stat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		s.tokens, s.seen, s.size = nil, time.Time{}, 0
		return nil
	}
	if err != nil {
		return err
	}
	text, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	var tokens []Token
	if err := json.Unmarshal(text, &tokens); err != nil {
		return fmt.Errorf("%s is not a list of tokens: %w", s.path, err)
	}
	s.tokens, s.seen, s.size = tokens, info.ModTime(), info.Size()
	return nil
}

// fresh reads the file again when something else changed it.
func (s *Store) fresh() {
	if info, err := os.Stat(s.path); err == nil && (info.ModTime().After(s.seen) || info.Size() != s.size) {
		_ = s.load()
	}
}

// save writes the file whole and moves it into place, so a reader never sees
// half of it.
func (s *Store) save() error {
	text, err := json.MarshalIndent(s.tokens, "", "  ")
	if err != nil {
		return err
	}
	scratch := s.path + ".new"
	if err := os.WriteFile(scratch, append(text, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(scratch, s.path); err != nil {
		return err
	}
	if info, err := os.Stat(s.path); err == nil {
		s.seen, s.size = info.ModTime(), info.Size()
	}
	return nil
}

func digest(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func random(n int) ([]byte, error) {
	raw := make([]byte, n)
	_, err := rand.Read(raw)
	return raw, err
}

// Create makes a token and returns it with its secret, which nothing keeps and
// nothing can show again. The name is what records call the token by, so two
// live tokens cannot share one.
func (s *Store) Create(name string, scopes []string, by string) (Token, string, error) {
	name = strings.TrimSpace(name)
	if !plain.MatchString(name) {
		return Token{}, "", ErrName
	}
	if len(scopes) == 0 {
		return Token{}, "", errors.New("give the token at least one thing it may do")
	}
	for _, scope := range scopes {
		if !slices.Contains(Scopes, scope) {
			return Token{}, "", fmt.Errorf("%q is not something a token may do: %s", scope, strings.Join(Scopes, ", "))
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fresh()
	for _, held := range s.tokens {
		if held.Live() && held.Name == name {
			return Token{}, "", fmt.Errorf("a token called %s is already in use: revoke it first, or choose another name", name)
		}
	}
	id, err := random(4)
	if err != nil {
		return Token{}, "", err
	}
	part, err := random(32)
	if err != nil {
		return Token{}, "", err
	}
	random := base64.RawURLEncoding.EncodeToString(part)
	token := Token{
		ID: hex.EncodeToString(id), Name: name, Scopes: sorted(scopes), Hash: digest(random),
		Made: s.now().Unix(), By: by,
	}
	s.tokens = append(s.tokens, token)
	if err := s.save(); err != nil {
		s.tokens = s.tokens[:len(s.tokens)-1]
		return Token{}, "", err
	}
	return token, Prefix + token.ID + "_" + random, nil
}

// sorted is the scopes in the order Scopes gives them, each once.
func sorted(scopes []string) []string {
	var out []string
	for _, scope := range Scopes {
		if slices.Contains(scopes, scope) {
			out = append(out, scope)
		}
	}
	return out
}

// Revoke ends a token now. It is not an error to revoke one already ended.
func (s *Store) Revoke(id, by string) (Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fresh()
	for i := range s.tokens {
		if s.tokens[i].ID != id {
			continue
		}
		if s.tokens[i].Live() {
			s.tokens[i].Revoked, s.tokens[i].RevokedBy = s.now().Unix(), by
			if err := s.save(); err != nil {
				s.tokens[i].Revoked, s.tokens[i].RevokedBy = 0, ""
				return Token{}, err
			}
		}
		return s.tokens[i], nil
	}
	return Token{}, ErrUnknown
}

// List is every token, the newest first, revoked ones included, because who had
// a token and when it ended is part of the record.
func (s *Store) List() []Token {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fresh()
	out := slices.Clone(s.tokens)
	slices.Reverse(out)
	return out
}

// Check is the token a secret stands for. The secret's id finds the token, and
// its random part is compared to the stored hash in constant time, so how long
// a refusal takes says nothing about how close a guess was.
func (s *Store) Check(secret string) (Token, error) {
	rest, ok := strings.CutPrefix(secret, Prefix)
	id, random, found := strings.Cut(rest, "_")
	if !ok || !found || id == "" || random == "" {
		return Token{}, ErrUnknown
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fresh()
	for i := range s.tokens {
		if s.tokens[i].ID != id {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(digest(random)), []byte(s.tokens[i].Hash)) != 1 {
			return Token{}, ErrUnknown
		}
		if !s.tokens[i].Live() {
			return s.tokens[i], ErrRevoked
		}
		now := s.now()
		s.tokens[i].LastUsed = now.Unix()
		if now.Sub(s.written) > time.Minute {
			if s.save() == nil {
				s.written = now
			}
		}
		return s.tokens[i], nil
	}
	return Token{}, ErrUnknown
}

// CreateFor makes a token that acts as one person, for the computer they approved. It can do nothing else, and the person
// may end it. Its name says whose it is and which computer, and is made unique so that two computers of one person can
// both hold one.
func (s *Store) CreateFor(person, label string) (Token, string, error) {
	person = strings.TrimSpace(person)
	if person == "" {
		return Token{}, "", errors.New("a token for a person needs the person")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fresh()
	id, err := random(4)
	if err != nil {
		return Token{}, "", err
	}
	part, err := random(32)
	if err != nil {
		return Token{}, "", err
	}
	random := base64.RawURLEncoding.EncodeToString(part)
	token := Token{
		ID: hex.EncodeToString(id), Name: nameFor(person, label, hex.EncodeToString(id)), Scopes: []string{Work}, Hash: digest(random),
		Person: person, Made: s.now().Unix(), By: person,
	}
	s.tokens = append(s.tokens, token)
	if err := s.save(); err != nil {
		s.tokens = s.tokens[:len(s.tokens)-1]
		return Token{}, "", err
	}
	return token, Prefix + token.ID + "_" + random, nil
}

var notPlain = regexp.MustCompile(`[^a-z0-9]+`)

// nameFor is a token's name from its person and the label of their computer, ending in the token's own id so that it is
// unique, and short enough for the name rule.
func nameFor(person, label, id string) string {
	slug := func(text string, most int) string {
		text = strings.Trim(notPlain.ReplaceAllString(strings.ToLower(text), "-"), "-")
		if len(text) > most {
			text = strings.Trim(text[:most], "-")
		}
		return text
	}
	return strings.Trim(slug(label, 16)+"-"+slug(person, 30)+"-"+id, "-")
}

// ListFor is a person's own tokens, the newest first, revoked ones included.
func (s *Store) ListFor(person string) []Token {
	var own []Token
	for _, held := range s.List() {
		if held.Person != "" && strings.EqualFold(held.Person, person) {
			own = append(own, held)
		}
	}
	return own
}

// RevokeOwn ends a token of this person's. A token that is not theirs is answered as unknown, so a person cannot learn
// which ids exist.
func (s *Store) RevokeOwn(id, person string) (Token, error) {
	for _, held := range s.ListFor(person) {
		if held.ID == id {
			return s.Revoke(id, person)
		}
	}
	return Token{}, ErrUnknown
}
