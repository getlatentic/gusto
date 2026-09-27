package web

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
)

//go:embed static
var assets embed.FS

// serveStatic hands out what is vendored in the binary.
//
// Nothing here is fetched at run time. A page that reaches a content delivery
// network is a page whose contents somebody else can change after it was
// reviewed, and an air-gapped install cannot reach one at all.
func serveStatic() http.Handler {
	files := http.FileServer(http.FS(assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The file is part of the binary, so it changes only when the binary
		// does, and the binary's own name is in the path nobody else serves.
		w.Header().Set("Cache-Control", "public, max-age=3600")
		files.ServeHTTP(w, r)
	})
}

// fingerprints is a short hash of each file under static/, computed once.
// A page names a file with its hash in the query, so a file that changed in a
// new binary is fetched again rather than served from an hour-old cache.
var fingerprints = func() map[string]string {
	sums := map[string]string{}
	_ = fs.WalkDir(assets, "static", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		text, err := assets.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(text)
		sums["/"+path] = hex.EncodeToString(sum[:])[:10]
		return nil
	})
	return sums
}()

// asset is a static file's address with its fingerprint.
func asset(path string) string {
	if sum, found := fingerprints[path]; found {
		return path + "?v=" + sum
	}
	return path
}
