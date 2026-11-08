package web

import (
	"path"
	"regexp"

	"github.com/getlatentic/gusto/internal/apps"
)

// previewFolder is a folder in the agent's folder, as a preview names it.
var previewFolder = regexp.MustCompile(`^(?:\.|[A-Za-z0-9_][A-Za-z0-9._-]*(?:/[A-Za-z0-9_][A-Za-z0-9._-]*)*)$`)

// previewsOf is the folders among files that hold a page: where an index.html is.
func previewsOf(files []string) []string {
	var folders []string
	for _, file := range files {
		if path.Base(file) != "index.html" {
			continue
		}
		folder := path.Dir(path.Clean(file))
		if previewFolder.MatchString(folder) && !contains(folders, folder) {
			folders = append(folders, folder)
		}
	}
	return folders
}

func contains(list []string, want string) bool {
	for _, each := range list {
		if each == want {
			return true
		}
	}
	return false
}

// previewing is what the page says of the agent's apps: one that answers is opened, one that is starting is waited for.
func previewing(found []apps.App) (live []apps.App, starting bool) {
	for _, each := range found {
		switch each.State {
		case apps.Live:
			live = append(live, each)
		case apps.Starting:
			starting = true
		}
	}
	return live, starting
}
