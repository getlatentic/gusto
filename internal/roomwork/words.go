package roomwork

import (
	"fmt"
	"path"
	"strings"
)

// shownConflicts is how many conflicts a note names before it counts the rest.
const shownConflicts = 5

// sideName is the name a turn's version of a conflicting file is saved under: the agent's name goes before the extension, so
// report.md becomes report.bea.md. A second name for the same file gets a number.
func sideName(name, agent string, n int) string {
	folder, base := path.Split(name)
	stem, ext := base, ""
	if dot := strings.LastIndexByte(base, '.'); dot > 0 {
		stem, ext = base[:dot], base[dot:]
	}
	tag := slug(agent)
	if n > 1 {
		tag = fmt.Sprintf("%s-%d", tag, n)
	}
	return folder + stem + "." + tag + ext
}

// slug is a name as a file name carries it: lower case letters and digits, the rest joined by single dashes.
func slug(name string) string {
	var out strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
			dash = false
		case !dash && out.Len() > 0:
			out.WriteByte('-')
			dash = true
		}
	}
	if word := strings.Trim(out.String(), "-"); word != "" {
		return word
	}
	return "agent"
}

// Words says what the conflicts were in the plain sentences a room shows, or nothing when there were none.
func (o Outcome) Words() string {
	var lines []string
	for i, each := range o.Conflicts {
		if i == shownConflicts {
			lines = append(lines, fmt.Sprintf("%d more files had the same trouble.", len(o.Conflicts)-shownConflicts))
			break
		}
		lines = append(lines, each.words())
	}
	return strings.Join(lines, " ")
}

func (c Conflict) words() string {
	switch {
	case c.Deleted && c.Other != "":
		return fmt.Sprintf("%s deleted %s while %s changed it, so it stays.", c.Agent, c.Path, c.Other)
	case c.Deleted:
		return fmt.Sprintf("%s deleted %s after it was changed by somebody else, so it stays.", c.Agent, c.Path)
	case c.Other != "":
		return fmt.Sprintf("%s and %s both changed %s. %s is %s and %s is %s.", c.Other, c.Agent, c.Path,
			c.Path, possessive(c.Other)+" version", c.Side, possessive(c.Agent)+" version")
	}
	return fmt.Sprintf("%s changed %s while somebody else did. %s is the version that was there and %s is %s.", c.Agent, c.Path,
		c.Path, c.Side, possessive(c.Agent)+" version")
}

func possessive(name string) string {
	if strings.HasSuffix(name, "s") {
		return name + "'"
	}
	return name + "'s"
}
