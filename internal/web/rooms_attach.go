package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/getlatentic/gusto/internal/instance"
)

// A message in a room can carry files. They are kept in the room's folder as the Files panel's uploads are, and the message names them,
// so the files and the words about them are one turn: the agents asked are told which files the person means.

// attachLimit is the most one message and its files may weigh as it is sent, a little over what the files may.
const attachLimit = mostUploads*mostFileBytes + 1<<20

// attachLimits is what the page's message box tells a person before they send: how many files a message takes and the size of the
// biggest.
var attachLimits = struct {
	Files int
	Bytes int64
}{mostUploads, mostFileBytes}

func init() { helpers["attachLimits"] = func() any { return attachLimits } }

// withAttachments keeps the files sent with a room's message and gives back the message with their names under it. It says whether
// the message can go on; when it cannot it has answered the person, who is sent back to the box with the message unsent.
func (s *Server) withAttachments(w http.ResponseWriter, r *http.Request, held instance.Instance, thread instance.Thread, said string) (string, bool) {
	if r.MultipartForm == nil {
		return said, true
	}
	defer r.MultipartForm.RemoveAll()
	sent := r.MultipartForm.File["file"]
	if len(sent) == 0 {
		return said, true
	}
	back := threadPath(thread)
	if len(sent) > mostUploads {
		s.turnAway(w, r, back, fmt.Sprintf("Attach at most %d files to one message.", mostUploads))
		return "", false
	}
	for _, each := range sent {
		if strings.Contains(each.Filename, "@") {
			s.turnAway(w, r, back, each.Filename+" has an @ in its name, which the room would read as a mention. Rename it and attach it again.")
			return "", false
		}
	}
	names, _, err := s.keepUploads(r, held, sent)
	if err != nil {
		s.turnAway(w, r, back, "The files were not attached: "+err.Error()+".")
		return "", false
	}
	return withNames(said, names), true
}

// withNames is a message with the names of the files it carries on a line of their own.
func withNames(said string, names []string) string {
	line := "Attached: " + strings.Join(names, ", ")
	if said == "" {
		return line
	}
	return said + "\n\n" + line
}
