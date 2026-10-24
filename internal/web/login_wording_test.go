package web

import (
	"strings"
	"testing"
)

func TestAWrongPasswordIsAnsweredInPlainWords(t *testing.T) {
	site := open(t)
	page := site.ask(t, "GET", "/login?wrong=1", nil).Body.String()
	if !strings.Contains(page, "That password is not right. Check it and try again.") {
		t.Errorf("the sign-in page does not say the password is not right:\n%s", page)
	}
	if strings.Contains(page, "gusto open") {
		t.Error("a server with a shared password sends a person to a link they do not have")
	}
}
