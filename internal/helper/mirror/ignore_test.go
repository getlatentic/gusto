package mirror

import "testing"

func TestWhatIsLeftOutAndWhatIsNot(t *testing.T) {
	ignore := NewIgnore("*.log", "build/", "/secrets.txt", "docs/drafts", "# a comment", "", "!keep.log", "  spaced.txt  ")
	for name, test := range map[string]struct {
		path  string
		isDir bool
		skip  bool
	}{
		"git's own folder":                    {".git", true, true},
		"a file in it":                        {".git/config", false, true},
		"a package folder deep down":          {"web/node_modules", true, true},
		"a file inside it":                    {"web/node_modules/left-pad/index.js", false, true},
		"an operating system's litter":        {"src/.DS_Store", false, true},
		"a compiled python file":              {"a/b/mod.pyc", false, true},
		"a log at any depth":                  {"a/b/run.log", false, true},
		"a folder pattern matches the folder": {"build", true, true},
		"and what is in it":                   {"build/out.o", false, true},
		"but a file of that name is not it":   {"build", false, false},
		"an anchored name at the root":        {"secrets.txt", false, true},
		"not the same name deeper":            {"a/secrets.txt", false, false},
		"a path from the root":                {"docs/drafts/one.md", false, true},
		"not the same path deeper":            {"x/docs/drafts/one.md", false, false},
		"a spaced pattern is trimmed":         {"spaced.txt", false, true},
		"ordinary work":                       {"src/main.go", false, false},
		"a partial name is not a match":       {"builds/out", false, false},
		"the sync's own scratch":              {"src/.gusto-write-123", false, true},
		"a name that only starts like git":    {".github/workflows/x.yml", false, false},
	} {
		if got := ignore.Skips(test.path, test.isDir); got != test.skip {
			t.Errorf("%s: %q skipped %v, want %v", name, test.path, got, test.skip)
		}
	}
	if len(ignore.Unsupported) != 1 || ignore.Unsupported[0] != "!keep.log" {
		t.Errorf("a pattern that is not understood was not reported: %v", ignore.Unsupported)
	}
}
