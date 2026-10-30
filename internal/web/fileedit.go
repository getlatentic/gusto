package web

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// editable marks a text file that is shown whole as one a person may change here, with all of it and what it was, so that a
// save says which file it changed and is refused when somebody, or an agent, changed it since.
func editable(file *viewedFile, resolved string, size int64) {
	switch file.Kind {
	case kindText, kindMarkdown, kindTable:
	default:
		return
	}
	if size > mostShown {
		return
	}
	text, err := os.ReadFile(resolved)
	if err != nil || len(text) > mostShown || !utf8.Valid(text) {
		return
	}
	sum := sha256.Sum256(text)
	file.Editable, file.Source, file.Hash = true, string(text), hex.EncodeToString(sum[:])
}

// saveFile puts what a person wrote in the page over a text file of the folder, when the file is still what they opened. In a
// room, the room says who changed it, so everybody in it, and its agents, know.
func (s *Server) saveFile(w http.ResponseWriter, r *http.Request) {
	held, found := s.sharedOr404(w, r)
	if !found {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2*mostShown)
	if err := r.ParseForm(); err != nil {
		answerJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "that is more than a file here can hold"})
		return
	}
	text, seen := r.PostFormValue("text"), r.PostFormValue("seen")
	switch {
	case seen == "":
		answerJSON(w, http.StatusBadRequest, map[string]string{"error": "say what the file was when you opened it"})
		return
	case len(text) > mostShown || !utf8.ValidString(text):
		answerJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "that is more than a file here can hold"})
		return
	}
	name := strings.TrimPrefix(filepath.ToSlash(filepath.Clean("/"+r.URL.Query().Get("path"))), "/")
	resolved, ok := resolveInFolder(s.instances.Workspace(held.ID), "/"+name)
	info, err := os.Stat(resolved)
	if !ok || err != nil || !info.Mode().IsRegular() {
		answerJSON(w, http.StatusNotFound, map[string]string{"error": "there is no such file to change"})
		return
	}
	put := s.putFile(r, held.ID, name, strings.NewReader(text), seen, info.Mode().Perm()&0o111 != 0)
	if put.status == http.StatusPreconditionFailed {
		answerJSON(w, http.StatusConflict, map[string]string{"error": "somebody changed this file after you opened it. Copy what you wrote, open the file again, and add it there.", "hash": put.hash})
		return
	}
	if put.status != http.StatusOK {
		answerJSON(w, put.status, map[string]string{"error": put.err})
		return
	}
	if held.Room {
		if thread := s.instances.Latest(held.ID); thread.ID != "" {
			_, _ = s.instances.FileNotice(held.ID, thread.ID, s.personName(s.keeper.who(r))+" changed "+name+".", name)
		}
	}
	answerJSON(w, http.StatusOK, map[string]string{"hash": put.hash})
}
