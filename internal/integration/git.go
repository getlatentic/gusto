package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Clone is how a plugin is fetched from a git address. It is a variable so that a test can fetch from a folder.
type Clone struct {
	// AllowPrivate lets the address be on a private network: a git server inside the organisation. Without it only the public
	// internet is reached, so an address cannot be used to look at what the server can see and the internet cannot.
	AllowPrivate bool
	// Timeout is how long a fetch may take.
	Timeout time.Duration
	// Resolve is how a host name is looked up, and a test replaces it.
	Resolve func(ctx context.Context, host string) ([]net.IP, error)
	// Scheme overrides which schemes are taken, which is https alone. A test adds file.
	Schemes []string
}

// Fetch clones an address at a branch, a tag or a commit into a new folder under parent, and returns the folder holding the plugin,
// which is path inside the repository, or its top. The source it returns names the commit that was fetched.
func (c Clone) Fetch(ctx context.Context, address, ref, subfolder, parent string) (string, Source, error) {
	if err := c.check(ctx, address); err != nil {
		return "", Source{}, err
	}
	if ref != "" && (strings.HasPrefix(ref, "-") || strings.ContainsAny(ref, " \t\r\n\x00:~^?*[\\")) {
		return "", Source{}, fmt.Errorf("%q is not a branch, a tag or a commit", ref)
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 2 * time.Minute
	}
	ctx, stop := context.WithTimeout(ctx, timeout)
	defer stop()
	into, err := os.MkdirTemp(parent, "clone-")
	if err != nil {
		return "", Source{}, err
	}
	commit, err := c.fetch(ctx, address, ref, into)
	if err != nil {
		os.RemoveAll(into)
		return "", Source{}, err
	}
	top := into
	if subfolder != "" {
		clean := filepath.Clean(filepath.FromSlash(subfolder))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			os.RemoveAll(into)
			return "", Source{}, fmt.Errorf("%q is not a folder in the repository", subfolder)
		}
		top = filepath.Join(into, clean)
		if !directory(into, top) {
			os.RemoveAll(into)
			return "", Source{}, fmt.Errorf("the repository has no folder %q", subfolder)
		}
	}
	return top, Source{Kind: "git", Address: address, Ref: ref, Commit: commit}, nil
}

func (c Clone) check(ctx context.Context, address string) error {
	parsed, err := url.Parse(address)
	if err != nil || (parsed.Host == "" && parsed.Scheme != "file") {
		return errors.New("the address must be a full git address, such as https://github.com/example/plugin.git")
	}
	allowed := c.Schemes
	if len(allowed) == 0 {
		allowed = []string{"https"}
	}
	ok := false
	for _, scheme := range allowed {
		ok = ok || parsed.Scheme == scheme
	}
	switch {
	case !ok:
		return fmt.Errorf("the address must be %s", strings.Join(allowed, " or "))
	case parsed.User != nil:
		return errors.New("the address must not hold a user name or a password")
	case strings.HasPrefix(address, "-"):
		return errors.New("the address is not an address")
	}
	if c.AllowPrivate || parsed.Scheme == "file" {
		return nil
	}
	resolve := c.Resolve
	if resolve == nil {
		resolve = func(ctx context.Context, host string) ([]net.IP, error) {
			return net.DefaultResolver.LookupIP(ctx, "ip", host)
		}
	}
	ips, err := resolve(ctx, parsed.Hostname())
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("the host %s cannot be found", parsed.Hostname())
	}
	for _, ip := range ips {
		if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
			return fmt.Errorf("%s is on a private network, which this does not reach unless an administrator allows it", parsed.Hostname())
		}
	}
	return nil
}

// fetch gets one commit, at depth one, with nothing of the machine's git configuration and no questions asked, and says which commit
// it is. It never runs anything the repository holds: no hooks, no filters, no submodules.
func (c Clone) fetch(ctx context.Context, address, ref, into string) (string, error) {
	run := func(args ...string) (string, error) {
		command := exec.CommandContext(ctx, "git", append([]string{
			"-c", "core.hooksPath=/dev/null", "-c", "protocol.allow=never", "-c", "protocol.https.allow=always", "-c", "protocol.file.allow=" + fileAllowed(address),
			"-c", "http.followRedirects=false", "-c", "core.fsmonitor=false", "-c", "core.symlinks=false", "-c", "submodule.recurse=false",
		}, args...)...)
		command.Dir = into
		command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + into, "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_ALLOW_PROTOCOL=https:file", "LC_ALL=C"}
		var out, problems bytes.Buffer
		command.Stdout, command.Stderr = &out, &problems
		if err := command.Run(); err != nil {
			if ctx.Err() != nil {
				return "", errors.New("fetching the plugin took too long")
			}
			return "", fmt.Errorf("git %s: %s", args[0], firstLine(strings.TrimSpace(problems.String())))
		}
		return strings.TrimSpace(out.String()), nil
	}
	if _, err := run("init", "--quiet"); err != nil {
		return "", err
	}
	want := ref
	if want == "" {
		want = "HEAD"
	}
	if _, err := run("fetch", "--quiet", "--depth", "1", "--no-tags", "--no-recurse-submodules", address, want); err != nil {
		return "", err
	}
	if _, err := run("checkout", "--quiet", "--detach", "FETCH_HEAD"); err != nil {
		return "", err
	}
	return run("rev-parse", "HEAD")
}

func fileAllowed(address string) string {
	if strings.HasPrefix(address, "file:") {
		return "always"
	}
	return "never"
}
