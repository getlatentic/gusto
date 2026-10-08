package web

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/getlatentic/haas/internal/changelog"
)

// A person's own computer signs in by the device flow: it asks for a code, the person approves the code on a page they
// are signed in to, and the computer is given a token that acts as that person. The computer never sees a password, and
// the person ends the token from the same page.

const (
	// deviceLife is how long a code waits for its person. Ten minutes is long enough to find a browser and short enough
	// that a code read over a shoulder is dead by the time it could be used.
	deviceLife = 10 * time.Minute
	// deviceWait is how often the computer is told to ask again.
	deviceWait = 3
	// devicesHeld bounds the codes waiting at once, and devicesPerClient the codes one source may have waiting, so that
	// asking for codes cannot fill the server's memory.
	devicesHeld      = 200
	devicesPerClient = 5
	// userAlphabet leaves out letters that read alike and the vowels, so a code never spells a word.
	userAlphabet = "BCDFGHJKLMNPQRSTVWXZ"
)

type deviceCode struct {
	user, label, client string
	expires             time.Time
	// person is who approved it, empty until somebody has.
	person string
}

type deviceCodes struct {
	mu   sync.Mutex
	held map[string]*deviceCode
}

func (d *deviceCodes) sweep(now time.Time) {
	for code, held := range d.held {
		if now.After(held.expires) {
			delete(d.held, code)
		}
	}
}

// start gives a computer a device code and the code a person types, or refuses when too many wait.
func (d *deviceCodes) start(label, client string, now time.Time) (device, user string, ok bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.held == nil {
		d.held = map[string]*deviceCode{}
	}
	d.sweep(now)
	own := 0
	for _, held := range d.held {
		if held.client == client {
			own++
		}
	}
	if len(d.held) >= devicesHeld || own >= devicesPerClient {
		return "", "", false
	}
	device, user = randomValue(), userCode()
	d.held[device] = &deviceCode{user: user, label: label, client: client, expires: now.Add(deviceLife)}
	return device, user, true
}

// approve ties the code a person typed to them. It says nothing about a code that does not exist, so that guessing one
// learns nothing.
func (d *deviceCodes) approve(user, person string, now time.Time) (label string, ok bool) {
	user = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(user), "-", ""))
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sweep(now)
	for _, held := range d.held {
		if strings.ReplaceAll(held.user, "-", "") == user && held.person == "" {
			held.person = person
			return held.label, true
		}
	}
	return "", false
}

// take hands over what a computer asked for: the person who approved, once. Pending is true while nobody has, and a code
// that is gone or never was answers as expired.
func (d *deviceCodes) take(device string, now time.Time) (person, label string, pending bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sweep(now)
	held, found := d.held[device]
	switch {
	case !found:
		return "", "", false
	case held.person == "":
		return "", "", true
	}
	delete(d.held, device)
	return held.person, held.label, false
}

func userCode() string {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return ""
	}
	out := make([]byte, 0, 9)
	for i, b := range raw {
		if i == 4 {
			out = append(out, '-')
		}
		out = append(out, userAlphabet[int(b)%len(userAlphabet)])
	}
	return string(out)
}

func answerJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// startDevice answers a computer that wants to sign in. Nobody is signed in yet, so it asks for nothing but a label.
func (s *Server) startDevice(w http.ResponseWriter, r *http.Request) {
	if s.tokens == nil {
		http.NotFound(w, r)
		return
	}
	var asked struct {
		Label string `json:"label"`
	}
	_ = json.NewDecoder(r.Body).Decode(&asked)
	label := strings.TrimSpace(asked.Label)
	if label == "" {
		label = "computer"
	}
	device, user, ok := s.devices.start(label, clientOf(r), time.Now())
	if !ok {
		answerJSON(w, http.StatusTooManyRequests, map[string]string{"error": "slow_down"})
		return
	}
	scheme := "http"
	if overTLS(r) {
		scheme = "https"
	}
	answerJSON(w, http.StatusOK, map[string]any{
		"device_code": device, "user_code": user,
		"verification_uri": scheme + "://" + r.Host + "/device",
		"expires_in":       int(deviceLife.Seconds()), "interval": deviceWait,
	})
}

// pollDevice is the computer asking whether its person has approved. The token is made here, when it is taken, so its
// secret goes to the computer that asked and to no page.
func (s *Server) pollDevice(w http.ResponseWriter, r *http.Request) {
	if s.tokens == nil {
		http.NotFound(w, r)
		return
	}
	var asked struct {
		DeviceCode string `json:"device_code"`
	}
	if json.NewDecoder(r.Body).Decode(&asked) != nil || asked.DeviceCode == "" {
		answerJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	person, label, pending := s.devices.take(asked.DeviceCode, time.Now())
	switch {
	case pending:
		answerJSON(w, http.StatusBadRequest, map[string]string{"error": "authorization_pending"})
		return
	case person == "":
		answerJSON(w, http.StatusBadRequest, map[string]string{"error": "expired_token"})
		return
	}
	token, secret, err := s.tokens.CreateFor(person, label)
	if err != nil {
		answerJSON(w, http.StatusInternalServerError, map[string]string{"error": "server_error"})
		return
	}
	s.record(changelog.Event{Who: person, Role: string(Runner), From: clientOf(r), Kind: changelog.TokenCreated,
		What: map[string]string{"id": token.ID, "name": token.Name, "scopes": "acts as " + person}})
	answerJSON(w, http.StatusOK, map[string]string{"access_token": secret, "token_type": "bearer", "person": person})
}

// me says who a personal token acts as, which is how a computer finds out it is signed in.
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	answerJSON(w, http.StatusOK, map[string]string{"person": personOf(r), "role": string(Runner)})
}

type personKey struct{}

type tokenIDKey struct{}

// endMe is a computer signing itself out: its own token is ended, so nothing that still works stays behind.
func (s *Server) endMe(w http.ResponseWriter, r *http.Request) {
	id, _ := r.Context().Value(tokenIDKey{}).(string)
	held, err := s.tokens.RevokeOwn(id, personOf(r))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.record(changelog.Event{Who: held.Person, Role: string(Runner), From: clientOf(r), Kind: changelog.TokenRevoked,
		What: map[string]string{"id": held.ID, "name": held.Name}})
	w.WriteHeader(http.StatusNoContent)
}

// personOf is who a personal token acts as.
func personOf(r *http.Request) string {
	who, _ := r.Context().Value(personKey{}).(string)
	return who
}

// devicePage is where a person approves a computer and ends the ones they have approved.
func (s *Server) devicePage(w http.ResponseWriter, r *http.Request) {
	s.showDevices(w, r, map[string]any{"Code": r.URL.Query().Get("code")})
}

func (s *Server) showDevices(w http.ResponseWriter, r *http.Request, page map[string]any) {
	if s.tokens == nil {
		http.NotFound(w, r)
		return
	}
	person := s.keeper.who(r)
	var rows []tokenRow
	if person != sharedSignIn {
		for _, held := range s.tokens.ListFor(person) {
			rows = append(rows, tokenRow{ID: held.ID, Name: held.Name, Made: shownTime(held.Made), Ended: endedOf(held),
				LastUsed: shownTime(held.LastUsed), Live: held.Live()})
		}
	}
	page["Title"] = productName + " — Your computers"
	page["Nav"] = "devices"
	page["Tokens"] = rows
	page["Shared"] = person == sharedSignIn
	s.render(w, "device.html", page)
}

// approveDevice ties the code the person typed to them. The shared sign-in is nobody in particular, so it is refused.
func (s *Server) approveDevice(w http.ResponseWriter, r *http.Request) {
	if s.tokens == nil {
		http.NotFound(w, r)
		return
	}
	person := s.keeper.who(r)
	if person == sharedSignIn {
		s.showDevices(w, r, map[string]any{"Refused": "The shared sign-in is not one person. Sign in with your own account to approve a computer."})
		return
	}
	if r.ParseForm() != nil {
		s.showDevices(w, r, map[string]any{"Refused": "That form could not be read."})
		return
	}
	label, ok := s.devices.approve(r.FormValue("code"), person, time.Now())
	if !ok {
		s.showDevices(w, r, map[string]any{"Refused": "That code is not waiting. Start the sign-in again on your computer and use the new code."})
		return
	}
	s.showDevices(w, r, map[string]any{"Approved": label})
}

// endDeviceToken ends one of the signed-in person's own tokens. Somebody else's is answered as not there.
func (s *Server) endDeviceToken(w http.ResponseWriter, r *http.Request) {
	if s.tokens == nil {
		http.NotFound(w, r)
		return
	}
	held, err := s.tokens.RevokeOwn(r.PathValue("id"), s.keeper.who(r))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.note(r, changelog.TokenRevoked, map[string]string{"id": held.ID, "name": held.Name})
	http.Redirect(w, r, "/device", http.StatusSeeOther)
}
