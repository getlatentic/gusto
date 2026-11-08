package roomwork

import (
	"fmt"
	"math/rand"
	"slices"
	"strings"
	"testing"
)

func TestChangesToDifferentLinesOfATextAreBothKept(t *testing.T) {
	base := "# Plan\n\nGoals\nlaunch in May\n\nRisks\nnone yet\n\nOwners\nAda\n"
	ours := "# Plan\n\nGoals\nlaunch in June\n\nRisks\nnone yet\n\nOwners\nAda\n"
	theirs := "# Plan\n\nGoals\nlaunch in May\n\nRisks\nnone yet\n\nOwners\nAda\nBea\n"
	got, ok := mergeLines([]byte(base), []byte(ours), []byte(theirs))
	want := "# Plan\n\nGoals\nlaunch in June\n\nRisks\nnone yet\n\nOwners\nAda\nBea\n"
	if !ok || string(got) != want {
		t.Fatalf("merged %v:\n%s", ok, got)
	}
}

func TestChangesToTheSameLineOrLinesThatTouchConflict(t *testing.T) {
	base := "one\ntwo\nthree\nfour\n"
	for _, each := range []struct{ ours, theirs string }{
		{"one\nTWO\nthree\nfour\n", "one\nzwei\nthree\nfour\n"},
		{"one\nTWO\nthree\nfour\n", "one\ntwo\nTHREE\nfour\n"},
		{"zero\none\ntwo\nthree\nfour\n", "nil\none\ntwo\nthree\nfour\n"},
	} {
		if got, ok := mergeLines([]byte(base), []byte(each.ours), []byte(each.theirs)); ok {
			t.Errorf("%q and %q merged into %q", each.ours, each.theirs, got)
		}
	}
}

func TestTheSameChangeOnBothSidesIsKeptOnce(t *testing.T) {
	base, both := "a\nb\nc\n", "a\nB\nc\n"
	if got, ok := mergeLines([]byte(base), []byte(both), []byte(both)); !ok || string(got) != both {
		t.Fatalf("merged %v: %q", ok, got)
	}
}

func TestAFileWithNoFinalNewlineMergesAsItIs(t *testing.T) {
	got, ok := mergeLines([]byte("a\nb\nc"), []byte("A\nb\nc"), []byte("a\nb\nC"))
	if !ok || string(got) != "A\nb\nC" {
		t.Fatalf("merged %v: %q", ok, got)
	}
}

func TestOnlyTextIsMergedByLines(t *testing.T) {
	if !isText([]byte("plain\n")) || isText([]byte{0x89, 'P', 'N', 'G', 0}) || isText([]byte{0xff, 0xfe}) {
		t.Error("text was not told from bytes")
	}
	if isText([]byte(strings.Repeat("x", mostMergeBytes+1))) {
		t.Error("a file too large to merge was taken as text")
	}
}

func TestVersionsTooFarApartAreNotCompared(t *testing.T) {
	var a, b []string
	for i := range mostEdits {
		a = append(a, "a"+string(rune('0'+i%10))+"\n")
		b = append(b, "b\n")
	}
	if _, ok := matches(a, b); ok {
		t.Error("two versions that differ in every line were compared")
	}
}

// The lines kept are as many as the longest common subsequence has, checked against the plain dynamic program on random texts.
func TestTheLinesKeptAreALongestCommonRun(t *testing.T) {
	random := rand.New(rand.NewSource(7))
	text := func() []string {
		out := make([]string, random.Intn(30))
		for i := range out {
			out[i] = string(rune('a'+random.Intn(4))) + "\n"
		}
		return out
	}
	for range 500 {
		a, b := text(), text()
		found, ok := matches(a, b)
		if !ok {
			t.Fatal("small texts were not compared")
		}
		kept, last := 0, -1
		for i, j := range found {
			if j < 0 {
				continue
			}
			if j <= last || a[i] != b[j] {
				t.Fatalf("%q and %q: line %d kept as %d", a, b, i, j)
			}
			kept, last = kept+1, j
		}
		if want := longestCommon(a, b); kept != want {
			t.Fatalf("%q and %q: kept %d lines, the longest run is %d", a, b, kept, want)
		}
	}
}

func longestCommon(a, b []string) int {
	table := make([][]int, len(a)+1)
	for i := range table {
		table[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				table[i][j] = table[i+1][j+1] + 1
			} else {
				table[i][j] = max(table[i+1][j], table[i][j+1])
			}
		}
	}
	return table[0][0]
}

// A file at the size merged by lines, about 20,000 lines, with two changes far apart, and with changes on both sides near the
// most lines that are compared.
func BenchmarkMergeLines(b *testing.B) {
	var text strings.Builder
	for i := 0; text.Len() < mostMergeBytes-64; i++ {
		fmt.Fprintf(&text, "line %d of the plan, with some words after it\n", i)
	}
	base := text.String()
	all := lines([]byte(base))
	edited := func(every, offset int) []byte {
		out := slices.Clone(all)
		for i := offset; i < len(out); i += every {
			out[i] = "changed " + out[i]
		}
		return []byte(strings.Join(out, ""))
	}
	for _, each := range []struct {
		name         string
		ours, theirs []byte
	}{
		{"two-changes", edited(len(all), 10), edited(len(all), len(all)-10)},
		{"near-the-most", edited(len(all)/200, 0), edited(len(all)/200, 7)},
	} {
		b.Run(each.name, func(b *testing.B) {
			for b.Loop() {
				if _, ok := mergeLines([]byte(base), each.ours, each.theirs); !ok {
					b.Fatal("did not merge")
				}
			}
		})
	}
}
