package repoaccess_test

import (
	"context"
	"encoding/base64"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/repoaccess"
	"github.com/getlatentic/haas/internal/runner"
)

// A token is for one repository and for what the work needs of it, so a job for
// one repository cannot read another, and a clone cannot push.
func TestATokenIsForOneRepositoryAndOnlyWhatTheWorkNeeds(t *testing.T) {
	f := newForge(t)
	f.private("acme", "private", 7)
	app := f.forge().App
	ctx := context.Background()

	read, err := app.Token(ctx, "acme", "private", repoaccess.Read)
	if err != nil {
		t.Fatal(err)
	}
	write, err := app.Token(ctx, "acme", "private", repoaccess.Write)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.minted) != 2 {
		t.Fatalf("%d tokens minted", len(f.minted))
	}
	asked := f.minted[0]
	if asked.Installation != 7 || len(asked.Repositories) != 1 || asked.Repositories[0] != "private" {
		t.Errorf("the read token was asked for %+v, and must name the one repository", asked)
	}
	if len(asked.Permissions) != 1 || asked.Permissions["contents"] != "read" {
		t.Errorf("a clone asked for %v", asked.Permissions)
	}
	pushing := f.minted[1].Permissions
	if len(pushing) != 2 || pushing["contents"] != "write" || pushing["pull_requests"] != "write" {
		t.Errorf("shipping asked for %v", pushing)
	}
	if read.Expires.IsZero() || write.Value == read.Value {
		t.Errorf("tokens %+v and %+v", read, write)
	}
	if err := read.Revoke(ctx); err != nil {
		t.Fatal(err)
	}
	if len(f.revoked) != 1 || f.revoked[0] != read.Value {
		t.Errorf("revoked %v, want the read token", f.revoked)
	}
}

func TestARepositoryTheAppIsNotInstalledOnIsRefusedByName(t *testing.T) {
	f := newForge(t)
	f.private("acme", "private", 7)
	other := f.private("acme", "other", 0)

	err := f.forge().Check(context.Background(), other)
	var refused repoaccess.NotInstalledError
	if !errors.As(err, &refused) || refused.Repo != "acme/other" {
		t.Fatalf("got %v, want the repository named", err)
	}
	if !strings.Contains(err.Error(), "acme/other") {
		t.Errorf("the refusal does not name the repository: %v", err)
	}
	if len(f.minted) != 0 {
		t.Error("a token was minted for a repository the App does not reach")
	}
	if err := f.forge().Check(context.Background(), f.server.URL+"/acme/private.git"); err != nil {
		t.Errorf("a repository the App reaches was refused: %v", err)
	}
}

// The checkout answers only to the token, which is revoked once git is done,
// and the repository is left with the plain address and no credential.
func TestACloneUsesTheTokenAndLeavesNoCredentialBehind(t *testing.T) {
	f := newForge(t)
	address := f.private("acme", "private", 7)
	dir := t.TempDir()

	// The control: the forge refuses a clone with no token.
	bare := exec.Command("git", "-C", dir, "clone", "--quiet", address, "bare")
	bare.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if err := bare.Run(); err == nil {
		t.Fatal("the forge let an unauthenticated clone through, so this test proves nothing")
	}

	if err := f.forge().Clone(context.Background(), address, dir, "workspace"); err != nil {
		t.Fatal(err)
	}
	if text, err := os.ReadFile(filepath.Join(dir, "workspace", "secret.txt")); err != nil || string(text) != "the private code\n" {
		t.Fatalf("the checkout: %q, %v", text, err)
	}
	if f.authorised == 0 {
		t.Error("no git request carried a live token")
	}
	token := "ghs_test" + strings.Repeat("0", 15) + "1"
	if len(f.revoked) != 1 || f.revoked[0] != token {
		t.Errorf("revoked %v, want %s once git finished", f.revoked, token)
	}
	config, err := os.ReadFile(filepath.Join(dir, "workspace", ".git", "config"))
	if err != nil {
		t.Fatal(err)
	}
	basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	for _, leaked := range []string{token, basic, "extraheader", "x-access-token"} {
		if strings.Contains(string(config), leaked) {
			t.Errorf("the repository's configuration holds %q:\n%s", leaked, config)
		}
	}
	if !strings.Contains(string(config), address) {
		t.Errorf("the remote is not the plain address:\n%s", config)
	}
}

// An address written for ssh is reached over HTTPS with the token, because the
// token is what the App holds.
func TestAnSSHAddressIsClonedOverHTTPSWithTheToken(t *testing.T) {
	f := newForge(t)
	f.private("acme", "private", 7)
	host := strings.TrimPrefix(f.server.URL, "http://")
	host = host[:strings.LastIndex(host, ":")]
	dir := t.TempDir()

	if err := f.forge().Clone(context.Background(), "git@"+host+":acme/private.git", dir, "workspace"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "workspace", "secret.txt")); err != nil {
		t.Fatal(err)
	}
}

// Somebody starting a job names a repository, and what is left in the job's
// directory afterwards is what the agent, the record and the transcript can see.
func TestNothingInAJobsDirectoryHoldsTheToken(t *testing.T) {
	f := newForge(t)
	address := f.private("acme", "private", 7)
	f.env(t)
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{Brief: "read it", Repo: address, Ref: "HEAD", Gate: []string{"true"}, Harness: "fixture"}
	if err := spec.Write(dir); err != nil {
		t.Fatal(err)
	}

	if err := runner.Fill(spec, dir); err != nil {
		t.Fatal(err)
	}
	token := "ghs_test" + strings.Repeat("0", 15) + "1"
	basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	filepath.WalkDir(dir.Path(), func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return nil
		}
		text, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, leaked := range []string{token, basic} {
			if strings.Contains(string(text), leaked) {
				t.Errorf("%s holds the credential", path)
			}
		}
		return nil
	})
	if _, err := os.Stat(dir.Spec()); err != nil {
		t.Fatal(err)
	}
	if len(f.revoked) != 1 {
		t.Errorf("the token was not revoked after the clone: %v", f.revoked)
	}
}

// A job for a repository the App does not reach is refused before anything is
// cloned, said in the record, and names the repository.
func TestAJobForARepositoryTheAppDoesNotReachIsRefusedInTheRecord(t *testing.T) {
	f := newForge(t)
	other := f.private("acme", "other", 0)
	f.env(t)
	dir := job.Dir(t.TempDir())
	if err := dir.Make(); err != nil {
		t.Fatal(err)
	}
	spec := job.Spec{Brief: "read it", Repo: other, Ref: "HEAD", Gate: []string{"true"}, Harness: "fixture"}

	err := runner.Fill(spec, dir)
	var refused repoaccess.NotInstalledError
	if !errors.As(err, &refused) {
		t.Fatalf("got %v, want a refusal naming the repository", err)
	}
	transcript, readErr := os.ReadFile(dir.OutFile("transcript.log"))
	if readErr != nil || !strings.Contains(string(transcript), "acme/other") || !strings.Contains(string(transcript), "refused this job before the harness started") {
		t.Errorf("the record says %q (%v)", transcript, readErr)
	}
	if _, err := os.Stat(filepath.Join(dir.Path(), "workspace", ".git")); err == nil {
		t.Error("a workspace was cloned for a refused job")
	}
	if len(f.minted) != 0 || f.authorised != 0 {
		t.Errorf("the forge saw %d tokens and %d authorised requests", len(f.minted), f.authorised)
	}
}

// Without an App, and for anything not on the forge, a repository is reached as
// git reaches it on this machine, which is what every earlier install does.
func TestWithoutAnAppOrOffTheForgeAGitCloneIsUnchanged(t *testing.T) {
	work := t.TempDir()
	run := func(args ...string) {
		command := exec.Command("git", args...)
		command.Dir = work
		command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, output)
		}
	}
	run("init", "--quiet", "-b", "main", ".")
	os.WriteFile(filepath.Join(work, "a.txt"), []byte("a"), 0o644)
	run("add", ".")
	run("commit", "--quiet", "-m", "one")

	for name, forge := range map[string]repoaccess.Forge{
		"no App":         {Web: "https://github.com"},
		"an App, off it": newForge(t).forge(),
	} {
		into := t.TempDir()
		if err := forge.Clone(context.Background(), work, into, "workspace"); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(into, "workspace", "a.txt")); err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if err := forge.Check(context.Background(), work); err != nil {
			t.Errorf("%s: a local path was refused: %v", name, err)
		}
	}
}

func TestEveryFormOfAnAddressIsReadAndOnlyTheForgesAreOurs(t *testing.T) {
	forge := repoaccess.Forge{Web: "https://github.com"}
	for address, want := range map[string]string{
		"https://github.com/acme/api":             "acme/api",
		"https://github.com/acme/api.git":         "acme/api",
		"https://github.com/acme/api/":            "acme/api",
		"git@github.com:acme/api.git":             "acme/api",
		"git@github.com:acme/api":                 "acme/api",
		"ssh://git@github.com/acme/api.git":       "acme/api",
		"https://GitHub.com/acme/api.git":         "acme/api",
		"https://gitlab.com/acme/api.git":         "",
		"git@gitlab.com:acme/api.git":             "",
		"/tmp/a/local/repo":                       "",
		"file:///tmp/a/local/repo":                "",
		"https://github.com/acme":                 "",
		"https://github.com/acme/api/tree/main/x": "",
		"": "",
	} {
		repo, ours := forge.Locate(address)
		got := ""
		if ours {
			got = repo.String()
		}
		if got != want {
			t.Errorf("%q read as %q, want %q", address, got, want)
		}
	}
}

func TestATokenHeaderIsAddedAfterConfigurationAlreadyThere(t *testing.T) {
	env := []string{"PATH=/bin", "GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=safe.directory", "GIT_CONFIG_VALUE_0=*"}
	got := repoaccess.WithToken(env, "https://ghe.example/", "tok")
	has := func(want string) bool {
		for _, entry := range got {
			if entry == want {
				return true
			}
		}
		return false
	}
	basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:tok"))
	for _, want := range []string{"GIT_CONFIG_COUNT=2", "GIT_CONFIG_KEY_0=safe.directory", "GIT_CONFIG_VALUE_0=*",
		"GIT_CONFIG_KEY_1=http.https://ghe.example/.extraheader", "GIT_CONFIG_VALUE_1=AUTHORIZATION: basic " + basic} {
		if !has(want) {
			t.Errorf("missing %q in %v", want, got)
		}
	}
}

func TestAnAppNamedByHalfIsAnErrorAndNoneIsNoApp(t *testing.T) {
	t.Setenv(repoaccess.AppIDEnv, "")
	t.Setenv(repoaccess.AppKeyEnv, "")
	forge, err := repoaccess.FromEnv()
	if err != nil || forge.App != nil || forge.Web != "https://github.com" || forge.API != "https://api.github.com" {
		t.Fatalf("with nothing set: %+v, %v", forge, err)
	}
	t.Setenv(repoaccess.AppIDEnv, "1")
	if _, err := repoaccess.FromEnv(); err == nil {
		t.Error("an App id without a key was accepted")
	}
	t.Setenv(repoaccess.AppIDEnv, "")
	t.Setenv(repoaccess.AppKeyEnv, "/nowhere")
	if _, err := repoaccess.FromEnv(); err == nil {
		t.Error("a key without an App id was accepted")
	}
	t.Setenv(repoaccess.AppIDEnv, "1")
	if _, err := repoaccess.FromEnv(); err == nil {
		t.Error("a key that is not there was accepted")
	}
}
