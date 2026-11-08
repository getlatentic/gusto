package cloud

import "testing"

// What is left of a credit is rounded down, so a person who has used some of it never reads that all of it is left.
func TestCreditLeftRoundsDownToTheCent(t *testing.T) {
	for _, c := range []struct {
		given, used float64
		want        string
	}{
		{5, 0, "$5.00 of $5.00"},
		{5, 0.0047, "$4.99 of $5.00"},
		{0.5, 0.6, "$0.00 of $0.50"},
	} {
		if got := creditLeft(c.given, c.used); got == nil || got.Left+" of "+got.Of != c.want {
			t.Errorf("%v of %v used: %+v, not %q", c.used, c.given, got, c.want)
		}
	}
	if got := creditLeft(0, 1); got != nil {
		t.Errorf("an organization with no credit is shown %+v", got)
	}
}
