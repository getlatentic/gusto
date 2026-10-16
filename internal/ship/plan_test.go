package ship

import (
	"testing"

	"github.com/getlatentic/gusto/internal/job"
)

func TestTheGitHubRepositoryIsReadOffTheRemote(t *testing.T) {
	for remote, want := range map[string]string{
		"https://github.com/getlatentic/gusto":     "getlatentic/gusto",
		"https://github.com/getlatentic/gusto.git": "getlatentic/gusto",
		"git@github.com:getlatentic/gusto.git":     "getlatentic/gusto",
		"ssh://git@github.com/getlatentic/gusto":   "getlatentic/gusto",
	} {
		owner, name, err := repository("", remote)
		if err != nil || owner+"/"+name != want {
			t.Errorf("%s: got %s/%s, %v", remote, owner, name, err)
		}
	}
	if _, _, err := repository("", "/srv/git/gusto.git"); err == nil {
		t.Error("a remote that is not on GitHub should ask for -github")
	}
	if owner, name, err := repository("o/r", "/srv/git/gusto.git"); err != nil || owner != "o" || name != "r" {
		t.Errorf("-github should win: %s/%s, %v", owner, name, err)
	}
}

func TestABranchNameIsSafeForGit(t *testing.T) {
	for dir, want := range map[string]string{
		"/jobs/20260926-0900-fix the THING!": "gusto/20260926-0900-fix-the-thing-0123456789ab",
		"/jobs/...":                          "gusto/job-0123456789ab",
	} {
		if got := branchName(job.Dir(dir), "0123456789abcdef"); got != want {
			t.Errorf("%s: got %s, want %s", dir, got, want)
		}
	}
}

func TestThePrintedRemoteCarriesNoCredential(t *testing.T) {
	for remote, want := range map[string]string{
		"https://x-access-token:secret@github.com/o/r": "https://github.com/o/r.git",
		"git@github.com:o/r.git":                       "https://github.com/o/r.git",
		"https://someone:secret@git.example.com/o/r":   "https://git.example.com/o/r",
		"/srv/git/r.git":                               "/srv/git/r.git",
	} {
		if got := printable(remote); got != want {
			t.Errorf("%s: got %s, want %s", remote, got, want)
		}
	}
}
