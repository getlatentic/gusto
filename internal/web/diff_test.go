package web

import (
	"reflect"
	"testing"
)

// twoFiles is `git diff` after `add -N`, as a runner writes it: one file
// changed, one file new.
const twoFiles = `diff --git a/answer.txt b/answer.txt
index 9dc6a5a..396a7ab 100644
--- a/answer.txt
+++ b/answer.txt
@@ -1,2 +1,2 @@ the answer
-no
+yes
 unchanged
\ No newline at end of file
diff --git a/notes.md b/notes.md
new file mode 100644
index 0000000..e69de29
--- /dev/null
+++ b/notes.md
@@ -0,0 +1 @@
+--- not a header
`

func TestAPatchIsReadFileByFileWithTheNumbersOfBothSides(t *testing.T) {
	files := parseDiff(twoFiles)
	if len(files) != 2 {
		t.Fatalf("two files, got %d: %+v", len(files), files)
	}
	answer := files[0]
	if answer.Name != "answer.txt" || answer.Added != 1 || answer.Removed != 1 {
		t.Errorf("answer.txt read as %+v", answer)
	}
	if answer.Hunks[0].Header != "@@ -1,2 +1,2 @@ the answer" {
		t.Errorf("the hunk header is %q", answer.Hunks[0].Header)
	}
	want := []diffLine{
		{Kind: removedLine, Old: 1, Text: "no"},
		{Kind: addedLine, New: 1, Text: "yes"},
		{Kind: contextLine, Old: 2, New: 2, Text: "unchanged"},
		{Kind: noteLine, Text: `\ No newline at end of file`},
	}
	if got := answer.Hunks[0].Lines; !reflect.DeepEqual(got, want) {
		t.Errorf("the lines are\n%+v\nwant\n%+v", got, want)
	}
	// Inside a hunk a line that reads like a file header is an added line.
	notes := files[1]
	if notes.Name != "notes.md" || notes.Added != 1 || notes.Hunks[0].Lines[0].Text != "--- not a header" {
		t.Errorf("notes.md read as %+v", notes)
	}
}

func TestAnEmptyPatchHasNoFiles(t *testing.T) {
	if files := parseDiff(""); len(files) != 0 {
		t.Fatalf("an empty patch gave %+v", files)
	}
}

func TestABinaryFileIsNamedAndSaysSo(t *testing.T) {
	files := parseDiff("diff --git a/logo.png b/logo.png\nnew file mode 100644\nindex 0000000..8c3f1a2\n" +
		"Binary files /dev/null and b/logo.png differ\n")
	if len(files) != 1 || files[0].Name != "logo.png" || !files[0].Binary || len(files[0].Hunks) != 0 {
		t.Fatalf("a binary stanza read as %+v", files)
	}
}

func TestADeletedFileKeepsItsOldName(t *testing.T) {
	files := parseDiff("diff --git a/gone.txt b/gone.txt\ndeleted file mode 100644\n--- a/gone.txt\n+++ /dev/null\n@@ -1 +0,0 @@\n-bye\n")
	if len(files) != 1 || files[0].Name != "gone.txt" || files[0].Removed != 1 {
		t.Fatalf("a deletion read as %+v", files)
	}
}

func TestARenameIsNamedByWhereTheFileWent(t *testing.T) {
	files := parseDiff("diff --git a/old name.txt b/new name.txt\nsimilarity index 100%\nrename from old name.txt\nrename to new name.txt\n")
	if len(files) != 1 || files[0].Name != "new name.txt" || len(files[0].Meta) != 3 {
		t.Fatalf("a rename read as %+v", files)
	}
}

func TestANameGitQuotedIsReadBack(t *testing.T) {
	files := parseDiff("diff --git \"a/caf\\303\\251.txt\" \"b/caf\\303\\251.txt\"\n--- \"a/caf\\303\\251.txt\"\n+++ \"b/caf\\303\\251.txt\"\n@@ -1 +1 @@\n-a\n+b\n")
	if len(files) != 1 || files[0].Name != "café.txt" {
		t.Fatalf("a quoted name read as %+v", files)
	}
}

// A patch cut at the page's limit ends part way through a hunk, and what was
// read before the cut is still drawn.
func TestAPatchCutPartWayThroughAHunkKeepsWhatItHas(t *testing.T) {
	files := parseDiff("diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1,40 +1,40 @@\n-one\n+two\n")
	if len(files) != 1 || len(files[0].Hunks[0].Lines) != 2 {
		t.Fatalf("a cut patch read as %+v", files)
	}
}
