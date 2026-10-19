package cloud

import (
	"strings"
	"time"
)

// noticeOfDemo mails the address that follows demos up, when there is one, that a visitor has started one. What the visitor typed
// is in the subject too, so it is put on one line: a line break in a subject would add a header of the visitor's choosing.
func (s *Server) noticeOfDemo(slug string) {
	if s.Notify == "" {
		return
	}
	lead, found := s.Store.LeadOf(slug)
	if !found {
		return
	}
	subject := "New demo: " + oneLine(lead.Name) + ", " + oneLine(lead.Company)
	body := strings.Join([]string{
		"Someone started a demo of gusto.",
		"",
		"Name: " + oneLine(lead.Name),
		"Email: " + oneLine(lead.Email),
		"Company: " + oneLine(lead.Company),
		"Asked at: " + lead.Requested.UTC().Format(time.RFC1123),
	}, "\n")
	s.send(s.Notify, subject, body)
}

// oneLine is text with each run of line breaks and tabs made a space, and no more than two hundred characters.
func oneLine(text string) string {
	line := strings.Join(strings.Fields(text), " ")
	if len(line) > 200 {
		line = line[:200]
	}
	return line
}
