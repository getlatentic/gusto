package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
)

// carriedIn hashes every file in an export except the summary, which cannot
// hold its own hash. A file counts as covered only when the run's manifest
// names these exact bytes; a redacted copy is the exporter's, not the run's.
func carriedIn(dir job.Dir, manifest evidence.Manifest) ([]Carried, error) {
	names, err := filesIn(dir)
	if err != nil {
		return nil, err
	}
	carried := make([]Carried, 0, len(names))
	for _, name := range names {
		text, err := os.ReadFile(filepath.Join(dir.Path(), filepath.FromSlash(name)))
		if err != nil {
			return nil, err
		}
		sum := digest(text)
		carried = append(carried, Carried{Name: name, SHA256: sum, Covered: vouches(manifest, name, sum)})
	}
	return carried, nil
}

// filesIn lists an export as slash paths inside it: the regular files at its
// root and in out/. A directory anywhere else is not part of the format, and
// evidence.Check does not read one either.
func filesIn(dir job.Dir) ([]string, error) {
	var names []string
	for _, branch := range []string{"", "out"} {
		listing, err := os.ReadDir(filepath.Join(dir.Path(), branch))
		if err != nil {
			return nil, err
		}
		for _, entry := range listing {
			name := path.Join(branch, entry.Name())
			if entry.Type().IsRegular() && name != SummaryFile {
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names, nil
}

// vouches says whether the manifest holds this hash for this file. The
// manifest's own hash is nowhere, because the manifest is what is trusted.
func vouches(manifest evidence.Manifest, name, sum string) bool {
	if name == SpecFile {
		return manifest.Job == sum
	}
	if name == ManifestFile {
		return true
	}
	held, named := originalOf(manifest, name)
	return named && held == sum
}

// originalOf is the hash the run recorded for a file of the export.
func originalOf(manifest evidence.Manifest, name string) (string, bool) {
	if name == SpecFile {
		return manifest.Job, true
	}
	inside, found := cutOut(name)
	if !found {
		return "", false
	}
	return hashIn(manifest, inside)
}

// cutOut is a file's name inside out/, which is how the manifest names it.
func cutOut(name string) (string, bool) {
	dir, base := path.Split(name)
	return base, dir == "out/"
}

func digest(text []byte) string {
	sum := sha256.Sum256(text)
	return hex.EncodeToString(sum[:])
}
