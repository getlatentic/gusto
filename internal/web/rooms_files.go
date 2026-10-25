package web

import (
	"cmp"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/getlatentic/gusto/internal/instance"
)

// A room's files are its folder, which everybody in the room shares. The room lists them in a panel beside its conversation, split
// as Kylon splits them: what its agents made, and what people added.

// roomFile is one file as the room's files panel lists it.
type roomFile struct {
	workspaceFile
	Name   string
	Folder string
}

// roomFileDay is the files changed on one day, newest first.
type roomFileDay struct {
	Day   string
	Files []roomFile
}

func (s *Server) roomFilesPanel(w http.ResponseWriter, r *http.Request) {
	held, found := s.roomOf(w, r)
	if !found {
		return
	}
	thread := s.instances.Latest(held.ID)
	if r.URL.Query().Get("panel") == "" {
		http.Redirect(w, r, "/instances/"+held.ID+"/files?from="+threadPath(thread), http.StatusSeeOther)
		return
	}
	files, cut := workspaceFiles(s.instances.Workspace(held.ID))
	made := s.madeByAgents(thread)
	var byAgents, byPeople []roomFile
	for _, file := range files {
		shown := roomFile{workspaceFile: file, Name: path.Base(file.Path), Folder: path.Dir(file.Path)}
		if made[file.Path] {
			byAgents = append(byAgents, shown)
		} else {
			byPeople = append(byPeople, shown)
		}
	}
	s.render(w, "room-files", map[string]any{
		"Room":     held,
		"From":     threadPath(thread),
		"ByAgents": filesByDay(byAgents, time.Now()),
		"ByPeople": filesByDay(byPeople, time.Now()),
		"Agents":   len(byAgents),
		"People":   len(byPeople),
		"Cut":      cut,
	})
}

// madeByAgents is every file a turn in the conversation changed.
func (s *Server) madeByAgents(thread instance.Thread) map[string]bool {
	made := map[string]bool{}
	for _, id := range thread.Turns {
		if record, found := s.index.Get(id); found {
			for _, name := range changed(record.Dir(s.index.Root())) {
				made[name] = true
			}
		}
	}
	return made
}

func filesByDay(files []roomFile, now time.Time) []roomFileDay {
	slices.SortFunc(files, func(a, b roomFile) int { return cmp.Compare(b.ModifiedNs, a.ModifiedNs) })
	var days []roomFileDay
	for _, file := range files {
		day := dayName(time.Unix(file.Modified, 0), now)
		if len(days) == 0 || days[len(days)-1].Day != day {
			days = append(days, roomFileDay{Day: day})
		}
		days[len(days)-1].Files = append(days[len(days)-1].Files, file)
	}
	return days
}

// mostUploads is how many files one upload may carry.
const mostUploads = 20

// uploadToRoom puts files somebody added into the room's folder, beside what is there, never over it.
func (s *Server) uploadToRoom(w http.ResponseWriter, r *http.Request) {
	held, found := s.roomOf(w, r)
	if !found {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, mostUploads*mostFileBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "the files could not be read: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.MultipartForm.RemoveAll()
	sent := r.MultipartForm.File["file"]
	if len(sent) == 0 || len(sent) > mostUploads {
		http.Error(w, fmt.Sprintf("send between 1 and %d files", mostUploads), http.StatusBadRequest)
		return
	}
	root := s.instances.Workspace(held.ID)
	s.writing(held.ID).Lock()
	defer s.writing(held.ID).Unlock()
	for _, each := range sent {
		if each.Size > mostFileBytes {
			http.Error(w, each.Filename+" is too big", http.StatusRequestEntityTooLarge)
			return
		}
		if err := keepUpload(root, each); err != nil {
			http.Error(w, each.Filename+" could not be kept: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// keepUpload writes one uploaded file at the top of the folder under its own name, numbering the name when it is taken.
func keepUpload(root string, sent *multipart.FileHeader) error {
	name := filepath.Base(strings.ReplaceAll(sent.Filename, "\\", "/"))
	if name == "." || name == "/" || strings.HasPrefix(name, ".") {
		return fmt.Errorf("that name cannot be used")
	}
	full, err := freePath(root, name)
	if err != nil {
		return err
	}
	from, err := sent.Open()
	if err != nil {
		return err
	}
	defer from.Close()
	to, err := os.OpenFile(full, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o664)
	if err != nil {
		return err
	}
	if _, err := io.Copy(to, from); err != nil {
		to.Close()
		os.Remove(full)
		return err
	}
	matchOwner(full, root)
	return to.Close()
}

// freePath is where a file of that name can go in the folder without replacing one: the name, or the name numbered.
func freePath(root, name string) (string, error) {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for n := 1; n < 1000; n++ {
		candidate := name
		if n > 1 {
			candidate = fmt.Sprintf("%s (%d)%s", stem, n, ext)
		}
		full, err := folderPath(root, candidate)
		if err != nil {
			return "", err
		}
		if _, err := os.Lstat(full); os.IsNotExist(err) {
			return full, nil
		}
	}
	return "", fmt.Errorf("too many files are called %s", name)
}
