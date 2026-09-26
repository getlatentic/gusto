package ship

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
)

// record is what the pull request says about the run behind it: enough for a
// reviewer to know which harness and model wrote the patch, which command
// passed it, which manifest to check it against, and who signed that manifest
// as a keyring checked it rather than as the manifest claims.
type record struct {
	spec     job.Spec
	manifest evidence.Manifest
	signer   string
	digest   string
	at       string
	links    links
}

func (r record) title() string {
	first, _, _ := strings.Cut(strings.TrimSpace(r.spec.Brief), "\n")
	runes := []rune(strings.TrimSpace(first))
	if len(runes) > 72 {
		runes = append(runes[:71], '…')
	}
	return "haas: " + string(runes)
}

func (r record) body() string {
	var out strings.Builder
	out.WriteString("A haas job wrote this patch, and its gate passed. It is here for review.\n\n")
	out.WriteString("| | |\n| --- | --- |\n")
	fmt.Fprintf(&out, "| harness | `%s` |\n", r.manifest.Harness)
	fmt.Fprintf(&out, "| model | %s |\n", r.model())
	fmt.Fprintf(&out, "| gate | exit %d |\n", r.manifest.Gate.Code)
	fmt.Fprintf(&out, "| manifest | [sha256 `%s`](%s) |\n", r.digest, r.links.blob("out/manifest.json"))
	fmt.Fprintf(&out, "| ran against | `%s` |\n", r.at)
	fmt.Fprintf(&out, "| signer | %s |\n", r.signer)
	fmt.Fprintf(&out, "\nThe gate:\n\n~~~\n%s\n~~~\n", shell(r.manifest.Gate.Command))
	fmt.Fprintf(&out, "\nThe brief:\n\n%s\n", quoted(r.spec.Brief))
	out.WriteString(r.links.section(r.digest, r.at))
	return out.String()
}

// model names what the job asked for and where it answered. Either can be
// empty: a harness may use its own default, and a harness needing no model has
// no endpoint.
func (r record) model() string {
	asked := "the harness's default"
	if r.spec.Model != "" {
		asked = "`" + r.spec.Model + "`"
	}
	if r.manifest.ModelEndpoint == "" {
		return asked
	}
	return asked + " at `" + r.manifest.ModelEndpoint + "`"
}

func (r record) message(dir job.Dir) string {
	return fmt.Sprintf("%s\n\nThe patch from haas job %s, whose gate passed.\nManifest sha256 %s.\n",
		r.title(), filepath.Base(filepath.Clean(dir.Path())), r.digest)
}

// shell writes the gate as a person would type it. The gate is an argv, and
// joining it with spaces would lose where one argument ends.
func shell(argv []string) string {
	words := make([]string, len(argv))
	for i, word := range argv {
		words[i] = word
		if word == "" || strings.ContainsFunc(word, unsafe) {
			words[i] = "'" + strings.ReplaceAll(word, "'", `'\''`) + "'"
		}
	}
	return strings.Join(words, " ")
}

func unsafe(letter rune) bool {
	return !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./=:,+@%", letter)
}

func quoted(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i, line := range lines {
		lines[i] = "> " + line
	}
	return strings.Join(lines, "\n")
}

// Describe is the plan as a person reads it. A dry run and a real run print the
// same lines, so what the dry run showed is what happened.
func (p Plan) Describe(dry bool) string {
	var out strings.Builder
	if dry {
		out.WriteString("a dry run: nothing below happens\n\n")
	}
	fmt.Fprintf(&out, "commit   %s, which is %s on %s\n", p.Commits.Change, p.Patch, p.At)
	fmt.Fprintf(&out, "commit   %s, which is job.json and out/ on top of it\n", p.Commits.Evidence)
	fmt.Fprintf(&out, "push     %s and %s to %s\n", p.Branch, p.Evidence, p.Remote)
	fmt.Fprintf(&out, "open     a pull request on %s/%s from %s into %s\n", p.Owner, p.Name, p.Branch, p.Base)
	fmt.Fprintf(&out, "\ntitle    %s\n\n%s\n", p.Title, p.Body)
	return out.String()
}
