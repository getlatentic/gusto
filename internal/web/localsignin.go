package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// linkLife is how long a sign-in link for gusto on a person's own computer can be followed after it is made.
const linkLife = 5 * time.Minute

// localSignIn is how the one person gusto runs for on their own computer signs in: by a link the gusto command makes with the
// secret only this person's files hold, followed once and within minutes. Nobody else on the machine has the secret, so a
// server on the machine's loopback is not open to every user of the machine.
type localSignIn struct {
	secret []byte
	who    string
	mu     sync.Mutex
	used   map[string]int64
}

// UseLocalSignIn lets the person who holds secret sign in as who by a link (LocalLink), and an admin, since it is their own.
func (s *Server) UseLocalSignIn(secret, who string) {
	s.local = &localSignIn{secret: []byte(secret), who: who, used: map[string]int64{}}
}

// LocalLink is the sign-in link for gusto at root, made at now with the nonce given: the path and query the server answers.
func LocalLink(secret, nonce string, now time.Time) string {
	until := strconv.FormatInt(now.Add(linkLife).Unix(), 10)
	return "/local/enter?n=" + nonce + "&e=" + until + "&c=" + linkCode(secret, nonce, until)
}

func linkCode(secret, nonce, until string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(nonce + "." + until))
	return hex.EncodeToString(mac.Sum(nil))
}

// enterLocal signs in the person whose link this is, once.
func (s *Server) enterLocal(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	nonce, until, code := query.Get("n"), query.Get("e"), query.Get("c")
	if !s.local.admits(nonce, until, code, time.Now()) {
		http.Error(w, "This sign-in link has been used, has run out, or is not this computer's. Run gusto open for a new one.", http.StatusForbidden)
		return
	}
	s.keeper.issueFor(w, r, s.local.who, Admin)
	http.Redirect(w, r, "/chat", http.StatusSeeOther)
}

func (l *localSignIn) admits(nonce, until, code string, now time.Time) bool {
	ends, err := strconv.ParseInt(until, 10, 64)
	if err != nil || len(nonce) < 16 || now.Unix() > ends || ends > now.Add(linkLife).Unix() {
		return false
	}
	if !hmac.Equal([]byte(code), []byte(linkCode(string(l.secret), nonce, until))) {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for seen, expired := range l.used {
		if now.Unix() > expired {
			delete(l.used, seen)
		}
	}
	if _, again := l.used[nonce]; again {
		return false
	}
	l.used[nonce] = ends
	return true
}
