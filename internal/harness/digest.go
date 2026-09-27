package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Digest names a configured harness by what it holds, so two people who both
// say "opencode" can tell whether they mean the same thing. Anybody holding the
// files computes the same string without this code; docs/FORMATS.md gives the
// steps.
//
// It is the SHA-256 of a listing, one line a file in path order:
//
//	<mode> <sha256 of the content, hex> <slash path>\n
//
// where the mode is 100755 for an executable file and 100644 for any other.
// Timestamps, owners and the rest of the permission bits are left out, because
// a checkout, an unpacked tarball and a copy into a job each set them afresh
// and none of them changes what the harness does.
func (t Tree) Digest() string {
	listing := sha256.New()
	for _, file := range t {
		fmt.Fprintf(listing, "%s %x %s\n", file.gitMode(), sha256.Sum256(file.Content), file.Path)
	}
	return "sha256:" + hex.EncodeToString(listing.Sum(nil))
}

// gitMode writes the executable bit the way git records it, so a reader who
// knows git's modes reads these without a table.
func (f File) gitMode() string {
	if f.Executable {
		return "100755"
	}
	return "100644"
}
