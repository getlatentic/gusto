package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// A shared password stands in for an identity provider. It is the smallest
// thing that stops a URL which spends money and runs code being open to
// whoever finds it, and the shape does not block SSO later: every handler
// already asks "who is this" rather than "is this on".
type keeper struct {
	password string
	key      []byte
	// ended holds the signatures of sessions somebody signed out of, until the
	// time each would have expired anyway, so a cookie that was copied before
	// signing out stops working when they do.
	mu    sync.Mutex
	ended map[string]int64
	// life is how long a sign-in lasts.
	life time.Duration
	// path is where the key and the ended sessions are kept, when they are kept, so that a sign-in outlives a restart.
	path string
}

func newKeeper(password string) (*keeper, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	// The key is made at boot, so a restart asks everybody to sign in again.
	// That is the honest trade for keeping no secret on disk.
	return &keeper{password: password, key: key, ended: map[string]int64{}, life: 12 * time.Hour}, nil
}

const cookieName = "haas"

// secureCookieName is what the session cookie is called over TLS. A browser accepts a name with
// this prefix only from the host itself, with Secure and no Domain, so a page on a sibling host
// name, a shared app's for one, cannot set or overwrite it for this one.
const secureCookieName = "__Host-haas"

// sessionCookie is the name of the session cookie for a request.
func sessionCookie(r *http.Request) string {
	if overTLS(r) {
		return secureCookieName
	}
	return cookieName
}

func (k *keeper) admits(given string) bool {
	return subtle.ConstantTimeCompare([]byte(given), []byte(k.password)) == 1
}

// issue marks the cookie Secure when the request arrived over TLS, directly or
// through a proxy that terminated it. Marking it always would mean the cookie is
// never sent back over plain http, which is how a local run works.
// issue signs somebody in as nobody in particular, which is what a shared
// password can say about them. Whoever holds it is the operator, so an admin.
func (k *keeper) issue(w http.ResponseWriter, r *http.Request) {
	k.issueFor(w, r, sharedSignIn, Admin)
}

// issueFor signs somebody in by name and role, so a job records who asked for
// it rather than that somebody did, and every request carries what they may do
// without asking the provider again.
func (k *keeper) issueFor(w http.ResponseWriter, r *http.Request, who string, role Role) {
	k.issueLasting(w, r, who, role, k.life)
}

// issueLasting signs somebody in for as long as asked, which is more than the usual when the sign-in is one a person made on purpose
// for a program they keep, such as the desktop app.
func (k *keeper) issueLasting(w http.ResponseWriter, r *http.Request, who string, role Role, life time.Duration) {
	until := time.Now().Add(life).Unix()
	value := strconv.FormatInt(until, 10) + "|" + string(role) + "|" + cookieWho(who)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie(r),
		Value:    value + "." + k.sign(value),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		Expires:  time.Unix(until, 0),
	})
}

// end makes the session in this request's cookie stop working, and forgets
// those that would have expired by now.
func (k *keeper) end(r *http.Request) {
	cookie, err := r.Cookie(sessionCookie(r))
	if err != nil {
		return
	}
	cut := strings.LastIndex(cookie.Value, ".")
	if cut < 0 {
		return
	}
	value, signature := cookie.Value[:cut], cookie.Value[cut+1:]
	if !hmac.Equal([]byte(signature), []byte(k.sign(value))) {
		return
	}
	until, err := strconv.ParseInt(strings.SplitN(value, "|", 2)[0], 10, 64)
	if err != nil {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	now := time.Now().Unix()
	for held, expires := range k.ended {
		if expires <= now {
			delete(k.ended, held)
		}
	}
	k.ended[signature] = until
	k.save()
}

func (k *keeper) hasEnded(signature string) bool {
	k.mu.Lock()
	defer k.mu.Unlock()
	_, gone := k.ended[signature]
	return gone
}

func (k *keeper) clear(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie(r), Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: overTLS(r)})
}

// session is who is in a signature this server made and has not yet expired,
// and the role they signed in with.
//
// The signature is taken from the last dot rather than the first, because the
// name is an address and an address has dots in it. A signature is base64url,
// which has none, so the last dot is always the one that separates them.
func (k *keeper) session(r *http.Request) (who string, role Role, signed bool) {
	cookie, err := r.Cookie(sessionCookie(r))
	if err != nil {
		return "", "", false
	}
	cut := strings.LastIndex(cookie.Value, ".")
	if cut < 0 {
		return "", "", false
	}
	value, signature := cookie.Value[:cut], cookie.Value[cut+1:]
	if !hmac.Equal([]byte(signature), []byte(k.sign(value))) || k.hasEnded(signature) {
		return "", "", false
	}
	parts := strings.SplitN(value, "|", 3)
	if len(parts) != 3 || parts[2] == "" {
		return "", "", false
	}
	until, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || time.Now().Unix() >= until {
		return "", "", false
	}
	return whoInCookie(parts[2]), Role(parts[1]), true
}

// cookieWho is a name as a cookie can carry it. A cookie's value cannot hold a space, a quote, a comma, a backslash or a
// letter outside ASCII, and a name can: Windows names a person COMPUTER\person.
func cookieWho(who string) string { return "~" + base64.RawURLEncoding.EncodeToString([]byte(who)) }

// whoInCookie is the name cookieWho wrote, or the name itself in a cookie made before names were written that way.
func whoInCookie(held string) string {
	if encoded, found := strings.CutPrefix(held, "~"); found {
		if who, err := base64.RawURLEncoding.DecodeString(encoded); err == nil {
			return string(who)
		}
	}
	return held
}

// knows reports whether this request carries a signature this server made and
// has not yet expired.
func (k *keeper) knows(r *http.Request) bool {
	_, _, signed := k.session(r)
	return signed
}

func (k *keeper) sign(value string) string {
	mac := hmac.New(sha256.New, k.key)
	mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// guard sends anybody this server does not know to the sign-in page, and never
// to the thing they asked for.
func (s *Server) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.keeper.knows(r) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

// who is the name in a signature this server made, or nobody when there is none.
func (k *keeper) who(r *http.Request) string {
	who, _, _ := k.session(r)
	return who
}

// role is what somebody signed in as, or nothing when nobody is signed in.
func (k *keeper) role(r *http.Request) Role {
	_, role, _ := k.session(r)
	return role
}

type keptSessions struct {
	Key   []byte           `json:"key"`
	Ended map[string]int64 `json:"ended"`
}

// keep makes sign-ins last across a restart: the key they are signed with and the sessions people ended are read from a file only
// this server's user reads, or made there when there is none. Without it a restart, which every deploy is, signs everybody out.
func (k *keeper) keep(path string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.path = path
	text, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return k.writeLocked()
	}
	if err != nil {
		return err
	}
	var held keptSessions
	if json.Unmarshal(text, &held) != nil || len(held.Key) < 32 {
		return k.writeLocked()
	}
	k.key = held.Key
	now := time.Now().Unix()
	for signature, until := range held.Ended {
		if until > now {
			k.ended[signature] = until
		}
	}
	return nil
}

func (k *keeper) save() {
	if k.path != "" {
		if err := k.writeLocked(); err != nil {
			slog.Warn("the sessions people ended could not be kept", "error", err)
		}
	}
}

func (k *keeper) writeLocked() error {
	text, err := json.Marshal(keptSessions{Key: k.key, Ended: k.ended})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(k.path), 0o700); err != nil {
		return err
	}
	scratch := k.path + ".new"
	if err := os.WriteFile(scratch, text, 0o600); err != nil {
		return err
	}
	return os.Rename(scratch, k.path)
}
