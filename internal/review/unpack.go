package review

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
)

// unpack writes the evidence commit's files into the job directory, each as
// the exact bytes git holds. Only the names a job directory gives its evidence
// are written, and only as plain files: anything else in the commit is
// reported and never written, so a crafted commit cannot put a file in the
// workspace or outside the directory.
func unpack(ctx context.Context, found fetched, dir job.Dir) ([]string, error) {
	listing, err := git(ctx, "-C", found.workspace, "ls-tree", "-r", "-z", found.evidence)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir.Out(), 0o755); err != nil {
		return nil, err
	}
	var wrong []string
	for _, line := range strings.Split(strings.TrimSuffix(listing, "\x00"), "\x00") {
		if line == "" {
			continue
		}
		mode, blob, name := entry(line)
		if !plainFile(mode) || !jobFile(name) {
			wrong = append(wrong, fmt.Sprintf("the evidence carries %s, which a job directory does not hold", name))
			continue
		}
		if err := written(ctx, found.workspace, blob, filepath.Join(dir.Path(), filepath.FromSlash(name))); err != nil {
			return nil, err
		}
	}
	return wrong, nil
}

// entry reads one line of ls-tree: the mode, the object and the path.
func entry(line string) (string, string, string) {
	meta, name, _ := strings.Cut(line, "\t")
	fields := strings.Fields(meta)
	if len(fields) != 3 {
		return "", "", name
	}
	return fields[0], fields[2], name
}

func plainFile(mode string) bool {
	return mode == "100644" || mode == "100755"
}

// jobFile is job.json, or a file directly in out/.
func jobFile(name string) bool {
	if name == "job.json" {
		return true
	}
	file, found := strings.CutPrefix(name, "out/")
	return found && file != "" && file != "." && file != ".." && !strings.Contains(file, "/")
}

func written(ctx context.Context, workspace, blob, path string) error {
	text, err := gitBytes(ctx, "-C", workspace, "cat-file", "blob", blob)
	if err != nil {
		return err
	}
	return os.WriteFile(path, text, 0o644)
}

// held holds manifest.json to the hash the pull request names, then re-hashes
// every file against the manifest the way `haas verify` does. The first check
// is what ties the files to the pull request: whoever can push can move the
// ref, and only a manifest with the named hash is the one the pull request
// vouched for.
func held(dir job.Dir, digest string) ([]string, error) {
	text, err := os.ReadFile(dir.OutFile("manifest.json"))
	if os.IsNotExist(err) {
		return []string{"manifest.json is missing"}, nil
	}
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(text)
	if got := hex.EncodeToString(sum[:]); got != digest {
		return []string{fmt.Sprintf("manifest.json hashes to %s, and the pull request names %s", got, digest)}, nil
	}
	return evidence.Check(dir)
}
