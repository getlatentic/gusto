package compare

import "testing"

// The sample keeps its commit while the template's files stay the same, and
// moves to a new one when they change, so every run of one lineup starts from
// the same code and a later lineup gets the newer files.
func TestTheSampleFollowsTheTemplate(t *testing.T) {
	board, err := OpenBoard(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, first, err := board.Sample("engineering", map[string]string{"a.js": "1"})
	if err != nil {
		t.Fatal(err)
	}
	_, again, _ := board.Sample("engineering", map[string]string{"a.js": "1"})
	_, changed, _ := board.Sample("engineering", map[string]string{"a.js": "2"})
	if first == "" || again != first || changed == first {
		t.Errorf("commits %s, %s, %s: want the first two equal and the third new", first, again, changed)
	}
	if _, _, err := board.Sample("x", map[string]string{"../out": "no"}); err == nil {
		t.Error("a file outside the sample was written")
	}
}
