package ship

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/getlatentic/gusto/internal/job"
)

// links is where the pull request sends a reviewer: files at the evidence
// commit on GitHub, to read, and the one command that fetches that commit and
// checks it, which needs nothing but git and the repository.
type links struct {
	owner   string
	name    string
	remote  string
	ref     string
	commits Commits
}

// blob addresses a file by the evidence commit's name rather than by its ref,
// because GitHub's pages resolve a commit anywhere in the repository and the
// name cannot move.
func (l links) blob(file string) string {
	return fmt.Sprintf("https://github.com/%s/%s/blob/%s/%s", l.owner, l.name, l.commits.Evidence, file)
}

func (l links) tree() string {
	return fmt.Sprintf("https://github.com/%s/%s/tree/%s", l.owner, l.name, l.commits.Evidence)
}

// command is what a reviewer types, and the pull request prints it whole. It
// names the manifest's hash, so what it fetches is held to the hash the pull
// request names and not to whatever the ref holds by then.
func (l links) command(digest string) string {
	return shell([]string{"gusto", "check", printable(l.remote), digest})
}

// printable is the remote as a pull request may show it. A GitHub remote is
// written the one way every reviewer can read, whatever the shipper's own git
// uses, and any other URL loses the credentials it may carry, because the body
// is public to everybody who can read the pull request.
func printable(remote string) string {
	bare := withoutCredentials(remote)
	if path := githubPath(bare); path != "" {
		return "https://github.com/" + path + ".git"
	}
	return bare
}

func withoutCredentials(remote string) string {
	address, err := url.Parse(remote)
	if err != nil || address.User == nil {
		return remote
	}
	address.User = nil
	return address.String()
}

// section is the part of the body about the evidence. The transcript is linked
// and never pasted: it can run to megabytes, and a pasted copy is not the file
// the manifest hashes.
func (l links) section(digest, at string) string {
	var out strings.Builder
	out.WriteString("\n### The evidence\n\n")
	fmt.Fprintf(&out, "The run's job.json and out/ are committed, byte for byte, at [`%s`](%s) on `%s`. ", short(l.commits.Evidence), l.tree(), l.ref)
	out.WriteString("That ref is not a branch, so none of it is part of this change.\n\n")
	fmt.Fprintf(&out, "- [manifest.json](%s), whose sha256 is `%s`\n", l.blob("out/manifest.json"), digest)
	fmt.Fprintf(&out, "- [transcript.log](%s), what the harness said\n", l.blob("out/transcript.log"))
	fmt.Fprintf(&out, "- [gate.log](%s), the gate's whole output\n", l.blob("out/gate.log"))
	out.WriteString("\nTo check it, wherever git can reach this repository:\n\n")
	fmt.Fprintf(&out, "~~~\n%s\n~~~\n\n", l.command(digest))
	fmt.Fprintf(&out, "It fetches `%s`, holds out/manifest.json to the sha256 above, re-hashes every file against the manifest, ", l.ref)
	fmt.Fprintf(&out, "checks that `%s` is out/diff.patch on `%s`, and runs the gate again. ", short(l.commits.Change), short(at))
	out.WriteString("Running the gate runs this pull request's code, as any review that runs its tests does.\n")
	return out.String()
}

// evidenceMessage says what the evidence commit is to anybody who finds it
// with git alone.
func evidenceMessage(dir job.Dir, digest string) string {
	return fmt.Sprintf("gusto evidence for job %s\n\nout/manifest.json has sha256 %s. The parent commit is the patch it vouches for.\n",
		filepath.Base(filepath.Clean(dir.Path())), digest)
}

func short(name string) string {
	if len(name) > 12 {
		return name[:12]
	}
	return name
}
