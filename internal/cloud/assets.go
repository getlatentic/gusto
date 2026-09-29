package cloud

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
)

// prints is a short digest of each of the front door's own static files, made once.
// A page names a file with its digest in the address, so a stylesheet that changed in
// a new release is fetched again and not read from a copy the browser kept for an hour.
var prints = func() map[string]string {
	sums := map[string]string{}
	_ = fs.WalkDir(files, "static", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		text, err := files.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(text)
		sums["/"+path] = hex.EncodeToString(sum[:])[:10]
		return nil
	})
	return sums
}()

// fingerprinted is the address of one of the front door's static files with its digest.
func fingerprinted(path string) string {
	if sum, found := prints[path]; found {
		return path + "?v=" + sum
	}
	return path
}
