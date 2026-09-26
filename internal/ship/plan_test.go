package ship

import (
	"testing"

	"github.com/getlatentic/haas/internal/job"
)

func TestTheGitHubRepositoryIsReadOffTheRemote(t *testing.T) {
	for remote, want := range map[string]string{
		"https://github.com/getlatentic/haas":     "getlatentic/haas",
		"https://github.com/getlatentic/haas.git": "getlatentic/haas",
		"git@github.com:getlatentic/haas.git":     "getlatentic/haas",
		"ssh://git@github.com/getlatentic/haas":   "getlatentic/haas",
	} {
		owner, name, err := repository("", remote)
		if err != nil || owner+"/"+name != want {
			t.Errorf("%s: got %s/%s, %v", remote, owner, name, err)
		}
	}
	if _, _, err := repository("", "/srv/git/haas.git"); err == nil {
		t.Error("a remote that is not on GitHub should ask for -github")
	}
	if owner, name, err := repository("o/r", "/srv/git/haas.git"); err != nil || owner != "o" || name != "r" {
		t.Errorf("-github should win: %s/%s, %v", owner, name, err)
	}
}

func TestABranchNameIsSafeForGit(t *testing.T) {
	for dir, want := range map[string]string{
		"/jobs/20260926-0900-fix the THING!": "haas/20260926-0900-fix-the-thing-0123456789ab",
		"/jobs/...":                          "haas/job-0123456789ab",
	} {
		if got := branchName(job.Dir(dir), "0123456789abcdef"); got != want {
			t.Errorf("%s: got %s, want %s", dir, got, want)
		}
	}
}
