package harness

import (
	"strings"

	"github.com/getlatentic/haas/internal/job"
)

// SetupFolder is where the job's setup sits in the listing a digest is taken
// over: its skills and its MCP server definitions, beside the harness's own
// files. A harness directory may not hold a folder of that name, so one path
// in the listing never means two files.
const SetupFolder = "setup"

// setupOf reads the setup the job carries, or nothing when it carries none or
// there is no job.
func setupOf(dir job.Dir) (Tree, error) {
	if dir == "" || !isDir(dir.SetupDir()) {
		return nil, nil
	}
	return walk(dir.SetupDir())
}

// with is the tree and a setup together, the setup's files under setup/, in
// the path order a digest needs.
func (t Tree) with(setup Tree) Tree {
	joined := append(Tree{}, t...)
	for _, file := range setup {
		file.Path = SetupFolder + "/" + file.Path
		joined = append(joined, file)
	}
	joined.sort()
	return joined
}

func (t Tree) holdsSetup() bool {
	for _, file := range t {
		if strings.HasPrefix(file.Path, SetupFolder+"/") {
			return true
		}
	}
	return false
}
