package web

import (
	"crypto/hmac"
	"net/http"
	"net/url"
	"strings"
)

// A page says what became of what was asked, from text in its address: why it was refused, what was connected, what was turned
// on. The text is sealed by this server, and a page shows only text it sealed, so a link somebody else made cannot put words in
// the product's mouth.

// withMessage is the address back with a message under key, and the seal that lets the page show it.
func (s *Server) withMessage(back, key, text string) string {
	join := "?"
	if strings.Contains(back, "?") {
		join = "&"
	}
	return back + join + key + "=" + url.QueryEscape(text) + "&" + sealOf(key) + "=" + s.keeper.sign(sealed(key, text))
}

// message is the message under key that this server sealed, or nothing.
func (s *Server) message(r *http.Request, key string) string {
	asked := r.URL.Query()
	text := asked.Get(key)
	if text == "" || !hmac.Equal([]byte(asked.Get(sealOf(key))), []byte(s.keeper.sign(sealed(key, text)))) {
		return ""
	}
	return text
}

func sealOf(key string) string { return key + "-seal" }

// sealed is what is signed: the key with the text, so a seal for one message is not one for another.
func sealed(key, text string) string { return "message\x00" + key + "\x00" + text }

// refusal is why the last thing asked was refused, as this server said it.
func (s *Server) refusal(r *http.Request) string { return s.message(r, "refused") }
