package evidence

import "testing"

// A turn names no check, so its harness's exit and the proxy's word on its model are what say it failed. A turn whose harness or
// model never answered must not be recorded as passed. A check, when there is one, decides.
func TestATurnWithNoCheckFailsWhenItsHarnessOrItsModelDid(t *testing.T) {
	for _, tc := range []struct {
		verdict *Verdict
		exit    int
		silent  bool
		want    int
	}{
		{nil, 0, false, 0},
		{nil, 1, false, 1},
		{nil, 0, true, 1},
		{&Verdict{Passed: true}, 1, false, 0},
		{&Verdict{Passed: true}, 0, true, 0},
		{&Verdict{Passed: false}, 0, false, 1},
	} {
		if got := Earned(tc.verdict, tc.exit, tc.silent); got != tc.want {
			t.Errorf("verdict %+v, harness exit %d, model silent %t: earned %d, want %d", tc.verdict, tc.exit, tc.silent, got, tc.want)
		}
	}
}
