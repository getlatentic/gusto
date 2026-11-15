package connector

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// NeedingSignIn is, for each connector, how many sign-ins across every person and workspace need signing in again. It opens
// every sign-in kept, so it is for a page an admin opens, and not for a request each person makes.
func (c *Connectors) NeedingSignIn() map[string]int {
	byHash := make(map[string]string, len(c.config.Apps))
	for _, entry := range c.config.Apps {
		byHash[hashOf(entry.ID)] = entry.ID
	}
	needing := map[string]int{}
	filepath.WalkDir(filepath.Join(c.vault.dir, "tokens"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		id, known := byHash[strings.TrimSuffix(entry.Name(), ".json")]
		if !known {
			return nil
		}
		relative, err := filepath.Rel(c.vault.dir, path)
		if err != nil {
			return nil
		}
		held, found, err := c.load(filepath.ToSlash(relative))
		if err == nil && found && held.standing(c.now()) == NeedsSignIn {
			needing[id]++
		}
		return nil
	})
	return needing
}
