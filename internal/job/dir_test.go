package job

import "testing"

// A folder of the workspace is relative and never leaves it.
func TestAFolderOfTheWorkspaceNeverLeavesIt(t *testing.T) {
	for dir, want := range map[string]string{"": "", ".": "", "src": "src", "src/": "src", "a/./b": "a/b", "a/b/../c": "a/c"} {
		if got, err := CleanDir(dir); err != nil || got != want {
			t.Errorf("%q gave %q %v, not %q", dir, got, err, want)
		}
	}
	for _, bad := range []string{"/etc", "..", "../x", "a/../../x", "a\\b", "a\x00b", "a\nb"} {
		if got, err := CleanDir(bad); err == nil {
			t.Errorf("%q was taken as %q", bad, got)
		}
	}
}
