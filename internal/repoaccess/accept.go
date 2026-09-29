package repoaccess

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// Acceptable says whether a person may name remote as where an agent's
// repository comes from. What they type is handed to git, so it is held to the
// forms that reach a repository over the network by an encrypted transport, and
// to a local path only under a directory the operator named.
//
// An address that starts with a dash is refused first: git would read it as an
// option, and one of them (--upload-pack) runs a command. A local path is
// refused by default because the server can read directories that belong to
// other people and to the server itself, and cloning one would hand them over.
func Acceptable(remote string, localRoots []string) error {
	switch {
	case strings.TrimSpace(remote) == "":
		return errors.New("name a repository")
	case strings.HasPrefix(remote, "-"):
		return errors.New("a repository address cannot start with a dash")
	case strings.ContainsAny(remote, "\x00\r\n"):
		return errors.New("that is not a repository address")
	}
	if scp.MatchString(remote) {
		return nil
	}
	if parsed, err := url.Parse(remote); err == nil && parsed.Host != "" {
		switch parsed.Scheme {
		case "https", "ssh":
			return nil
		}
		return fmt.Errorf("a repository is reached over https or ssh, not %s", parsed.Scheme)
	}
	if beginsLikeATransport.MatchString(remote) {
		return errors.New("a repository is reached over https or ssh")
	}
	if inside(remote, localRoots) {
		return nil
	}
	return errors.New("a repository is an https or ssh address, such as https://github.com/owner/name: this server does not clone from its own disk")
}

// beginsLikeATransport is git's helper syntax, such as ext::command, which runs
// a program to reach a repository.
var beginsLikeATransport = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*::`)

// inside says whether path is a local path under one of the roots.
func inside(path string, roots []string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	clean := filepath.Clean(path)
	for _, root := range roots {
		root = filepath.Clean(root)
		if clean == root || strings.HasPrefix(clean, root+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// AcceptableRef refuses a git ref that would be read as an option.
func AcceptableRef(ref string) error {
	if strings.HasPrefix(ref, "-") || strings.ContainsAny(ref, "\x00\r\n") {
		return errors.New("that is not a git ref")
	}
	return nil
}
