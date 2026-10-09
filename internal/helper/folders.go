package helper

import (
	"encoding/json"
	"net/http"
	"regexp"
)

// Folders is what a person's computer is asked to keep: a folder on it, by the key the desktop app made of its path, that an agent's
// folder in the cloud is kept the same as. The key is all the server ever holds of it.
type Folders interface {
	// Register keeps a folder, or says why it will not: it must be a folder, and not the whole of a home or a disk.
	Register(key, path string) error
	// Registered is every folder kept, by key.
	Registered() map[string]string
}

// FolderKey is what a key looks like.
var FolderKey = regexp.MustCompile(`^[a-f0-9]{16}$`)

type folderRequest struct {
	Key  string `json:"key"`
	Path string `json:"path"`
}

func (d *Daemon) listFolders(w http.ResponseWriter, r *http.Request) {
	if d.Folders == nil {
		writeJSON(w, map[string]string{})
		return
	}
	writeJSON(w, d.Folders.Registered())
}

func (d *Daemon) addFolder(w http.ResponseWriter, r *http.Request) {
	var asked folderRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&asked); err != nil || !FolderKey.MatchString(asked.Key) {
		http.Error(w, "that is not a folder and a key", http.StatusBadRequest)
		return
	}
	if d.Folders == nil {
		http.Error(w, "this helper keeps no folders", http.StatusNotImplemented)
		return
	}
	if err := d.Folders.Register(asked.Key, asked.Path); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]string{"key": asked.Key})
}
