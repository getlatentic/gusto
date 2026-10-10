package web

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

var cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)

// A declaration left outside any rule is not dropped on its own: the parser reads it as the selector of the next block and throws that
// block away with it, so one stray line silently removes the rule after it.
func TestOurStylesheetsHaveNoDeclarationOutsideARule(t *testing.T) {
	err := fs.WalkDir(assets, "static", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".css") {
			return err
		}
		body, err := fs.ReadFile(assets, path)
		if err != nil {
			return err
		}
		for _, problem := range strayDeclarations(cssComment.ReplaceAllString(string(body), "")) {
			t.Errorf("%s: %s", path, problem)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// strayDeclarations names each top-level prelude that holds a declaration, and a brace that closes nothing or is never closed.
func strayDeclarations(css string) []string {
	var problems []string
	depth, prelude := 0, strings.Builder{}
	for _, r := range css {
		switch {
		case r == '{':
			if depth == 0 && strings.Contains(prelude.String(), ";") && !strings.HasPrefix(strings.TrimSpace(prelude.String()), "@") {
				problems = append(problems, "a declaration outside a rule: "+strings.Join(strings.Fields(prelude.String()), " "))
			}
			depth++
			prelude.Reset()
		case r == '}':
			if depth == 0 {
				problems = append(problems, "a brace that closes nothing after: "+strings.Join(strings.Fields(prelude.String()), " "))
			} else {
				depth--
			}
			prelude.Reset()
		case depth == 0:
			prelude.WriteRune(r)
		}
	}
	if depth != 0 {
		problems = append(problems, "a block that is never closed")
	}
	return problems
}
