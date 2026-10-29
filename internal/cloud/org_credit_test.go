package cloud

import "testing"

// What is left of a credit is rounded down, so a person who has used some of it never reads that all of it is left.
func TestCreditLeftRoundsDownToTheCent(t *testing.T) {
	for _, c := range []struct {
		credit, used float64
		want         string
	}{
		{5, 0, "$5.00 of $5.00 left"},
		{5, 0.0047, "$4.99 of $5.00 left"},
		{0.5, 0.6, "$0.00 of $0.50 left"},
		{0, 1, ""},
	} {
		if got := creditLeft(c.credit, c.used); got != c.want {
			t.Errorf("%v of %v used: %q, not %q", c.used, c.credit, got, c.want)
		}
	}
}
