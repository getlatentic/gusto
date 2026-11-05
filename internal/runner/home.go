package runner

import (
	"io/fs"
	"os"
	"path/filepath"
)

// harnessSettings is where the image keeps each harness's own settings, which a kept home is given afresh at the start of
// each task, so a home made under an older image runs its harnesses as this image configures them.
var harnessSettings = "/opt/gusto/harness"

// homeToGive is what of the home is handed to the agent at the start of a task: the whole home, or of a person's kept home only
// what this task brings to it, which is the harnesses' settings, the task's skills and the conversation's own state, mounted
// there from the server. The rest of a kept home is what the agent made, and is the agent's already.
func (a agent) homeToGive() []string {
	home := os.Getenv("HOME")
	if !a.keptHome || home == "" {
		return []string{home}
	}
	return existing(filepath.Join(home, ".config"), filepath.Join(home, ".local", "share"), a.skills)
}

// homeToTakeBack is what of the home is given back to the server at the end of a task: the whole home, or of a kept home only
// the conversation's state, which is the server's to keep. The rest stays the agent's for the person's next task.
func (a agent) homeToTakeBack() []string {
	home := os.Getenv("HOME")
	if !a.keptHome || home == "" {
		return []string{home}
	}
	return existing(filepath.Join(home, ".local", "share"))
}

// existing is the paths given that are there.
func existing(paths ...string) []string {
	var found []string
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, err := os.Lstat(path); err == nil {
			found = append(found, path)
		}
	}
	return found
}

// freshenHome gives a kept home the image's harness settings and no skills but this task's: what an earlier task was given is
// not this one's.
func (a agent) freshenHome() error {
	home := os.Getenv("HOME")
	if !a.keptHome || home == "" {
		return nil
	}
	if a.skills != "" {
		if err := os.RemoveAll(a.skills); err != nil {
			return err
		}
	}
	return copyOver(harnessSettings, filepath.Join(home, ".config"))
}

// copyOver copies the files under from to the same places under to, replacing what is there, and leaves what only to has. A
// from that is not there copies nothing.
func copyOver(from, to string) error {
	if _, err := os.Stat(from); os.IsNotExist(err) {
		return nil
	}
	return filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		text, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.RemoveAll(target); err != nil {
			return err
		}
		return os.WriteFile(target, text, 0o644)
	})
}
