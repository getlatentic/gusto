package web

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
)

var pixels = regexp.MustCompile(`(^|[^\w.#])-?\d+(\.\d+)?px\b`)

// Sizes are in rem, so that they follow the person's text size. The vendored design tokens are the one place that still says px, and theme.css
// puts rem over the ones the app uses.
func TestOurStylesheetsSizeThingsInRem(t *testing.T) {
	err := fs.WalkDir(assets, "static", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".css") || strings.HasSuffix(path, "tokens.css") {
			return err
		}
		body, err := fs.ReadFile(assets, path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(body), "\n") {
			if pixels.MatchString(line) {
				t.Errorf("%s:%d sizes in px: %s", path, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
