package repoaccess

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The settings a deployment gives. The App is optional: without one, a
// repository is reached the way git reaches it on this machine.
const (
	AppIDEnv  = "HAAS_GITHUB_APP_ID"
	AppKeyEnv = "HAAS_GITHUB_APP_KEY"
	// WebEnv and APIEnv name a GitHub Enterprise Server. They default to github.com.
	WebEnv = "HAAS_GITHUB_URL"
	APIEnv = "HAAS_GITHUB_API"
)

// Forge is the GitHub a deployment reaches, and the App it reaches it with.
type Forge struct {
	Web string
	API string
	// App is nil when the deployment holds none.
	App *App
}

// FromEnv reads the deployment's settings. An App named by one setting and not
// the other is an error, because a runner that meant to use one must not
// quietly fall back to whatever credentials the machine happens to hold.
func FromEnv() (Forge, error) {
	forge := Forge{
		Web: strings.TrimSuffix(or(os.Getenv(WebEnv), "https://github.com"), "/"),
		API: strings.TrimSuffix(or(os.Getenv(APIEnv), "https://api.github.com"), "/"),
	}
	id, key := os.Getenv(AppIDEnv), os.Getenv(AppKeyEnv)
	switch {
	case id == "" && key == "":
		return forge, nil
	case id == "" || key == "":
		return Forge{}, fmt.Errorf("set both %s and %s, or neither", AppIDEnv, AppKeyEnv)
	}
	app, err := OpenApp(id, key, forge.API)
	if err != nil {
		return Forge{}, err
	}
	forge.App = app
	return forge, nil
}

func or(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

// Repo is a repository on the forge.
type Repo struct{ Owner, Name string }

func (r Repo) String() string { return r.Owner + "/" + r.Name }

var scp = regexp.MustCompile(`^[\w.-]+@([^:/]+):([^/]+)/(.+?)(?:\.git)?/?$`)

// Locate reads a repository address, in any of the forms git takes, and says
// whether it is on this forge. A local path, or a repository elsewhere, is not
// ours, and is reached as it always was.
func (f Forge) Locate(remote string) (Repo, bool) {
	web, err := url.Parse(f.Web)
	if err != nil {
		return Repo{}, false
	}
	if found := scp.FindStringSubmatch(remote); found != nil {
		return Repo{Owner: found[2], Name: found[3]}, strings.EqualFold(found[1], web.Hostname())
	}
	parsed, err := url.Parse(remote)
	if err != nil || parsed.Host == "" {
		return Repo{}, false
	}
	same := strings.EqualFold(parsed.Hostname(), web.Hostname())
	if parsed.Scheme == "http" || parsed.Scheme == "https" {
		same = strings.EqualFold(parsed.Host, web.Host)
	}
	owner, name, found := strings.Cut(strings.Trim(parsed.Path, "/"), "/")
	name = strings.TrimSuffix(strings.TrimSuffix(name, "/"), ".git")
	if !found || owner == "" || name == "" || strings.Contains(name, "/") {
		return Repo{}, false
	}
	return Repo{Owner: owner, Name: name}, same
}

// Check says whether the App can reach the repository, so a job for one it
// cannot is refused by name before anything starts. It asks nothing when the
// deployment holds no App or the repository is not on this forge.
func (f Forge) Check(ctx context.Context, remote string) error {
	repo, ours := f.Locate(remote)
	if f.App == nil || !ours {
		return nil
	}
	return f.App.Check(ctx, repo.Owner, repo.Name)
}

// Clone puts the repository at remote in dir/into. When the App reaches it, the
// token is minted for this one repository, read only, handed to git by
// environment where no process listing or file shows it, and revoked as soon as
// git has finished. The token is never in the repository's configuration: the
// remote it leaves behind is the plain address.
func (f Forge) Clone(ctx context.Context, remote, dir, into string) error {
	repo, ours := f.Locate(remote)
	if f.App == nil || !ours {
		return clone(ctx, nil, remote, dir, into)
	}
	token, err := f.App.Token(ctx, repo.Owner, repo.Name, Read)
	if err != nil {
		return err
	}
	defer release(token)
	return cloneWithToken(ctx, WithToken(os.Environ(), f.Web, token.Value), f.Web+"/"+repo.String()+".git", dir, into)
}

// retryDelay is the wait before the first retry, and it doubles each time.
var retryDelay = time.Second

// attempts is how many times a clone with a token is tried.
const attempts = 4

// cloneWithToken tries again when GitHub's git servers refuse a token the API
// has just minted and vouches for. Some of them answer "Repository not found"
// to a valid token now and then, and the next try succeeds, so a job for a
// repository the App reaches is not failed by it.
func cloneWithToken(ctx context.Context, env []string, remote, dir, into string) error {
	delay := retryDelay
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err = clone(ctx, env, remote, dir, into); err == nil || !refusedForNow(err) {
			return err
		}
		if attempt == attempts {
			break
		}
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return err
		}
		delay *= 2
	}
	return fmt.Errorf("GitHub refused the repository's token %d times running: %w", attempts, err)
}

// refusedForNow is a clone that failed the way an unrecognised token does.
func refusedForNow(err error) bool {
	said := err.Error()
	return strings.Contains(said, "Repository not found") || strings.Contains(said, "Invalid username or token") || strings.Contains(said, "Authentication failed")
}

// protocols are the transports git may use to clone. ext:: runs a program, and
// git:// and http:// send everything in the clear.
var protocols = "GIT_ALLOW_PROTOCOL=https:ssh:file"

func clone(ctx context.Context, env []string, remote, dir, into string) error {
	if env == nil {
		env = os.Environ()
	}
	// The double dash ends the options, so an address is never read as one.
	command := exec.CommandContext(ctx, "git", "-C", dir, "clone", "--quiet", "--", remote, into)
	command.Env = append(env, protocols, "GIT_TERMINAL_PROMPT=0")
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// WithToken adds the header that presents a token to the forge and to nothing
// else, after any configuration the environment already carries.
func WithToken(env []string, web, token string) []string {
	count := 0
	for _, entry := range env {
		if value, found := strings.CutPrefix(entry, "GIT_CONFIG_COUNT="); found {
			count, _ = strconv.Atoi(value)
		}
	}
	basic := base64.StdEncoding.EncodeToString([]byte("x-access-token:" + token))
	n := strconv.Itoa(count)
	return append(env, "GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_COUNT="+strconv.Itoa(count+1),
		"GIT_CONFIG_KEY_"+n+"=http."+strings.TrimSuffix(web, "/")+"/.extraheader",
		"GIT_CONFIG_VALUE_"+n+"=AUTHORIZATION: basic "+basic)
}

// release revokes a token when its work is done. A token that could not be
// revoked expires by itself within the hour, so a failure is said and not fatal.
func release(token Token) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := token.Revoke(ctx); err != nil {
		slog.Warn("a repository token could not be revoked and will expire on its own", "expires", token.Expires, "error", err)
	}
}

// Release revokes a token the caller minted.
func Release(token Token) { release(token) }
