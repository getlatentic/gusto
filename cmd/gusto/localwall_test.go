package main

import (
	"os"
	"testing"
)

func TestOnlyAYesAgreesToTheDownload(t *testing.T) {
	for answer, want := range map[string]bool{"y\n": true, "Yes\n": true, "\n": false, "no\n": false, "yeah sure\n": false} {
		in, err := os.CreateTemp(t.TempDir(), "in")
		if err != nil {
			t.Fatal(err)
		}
		in.WriteString(answer)
		in.Seek(0, 0)
		out, _ := os.CreateTemp(t.TempDir(), "out")
		if got := agreed(in, out); got != want {
			t.Errorf("%q agreed %v, want %v", answer, got, want)
		}
	}
}
