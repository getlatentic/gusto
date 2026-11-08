package roomwork

import (
	"bytes"
	"slices"
	"strings"
	"unicode/utf8"
)

// Two turns that changed different lines of one text file are both kept: the file is merged line by line against the version
// both began from, as diff3 does. Lines both changed, or changes that touch, are a conflict, and the file is saved beside as before.

// mostMergeBytes is the largest file merged line by line, which bounds what a merge holds in memory. A larger file conflicts whole.
const mostMergeBytes = 1 << 20

// mostEdits is the most lines that may differ between two versions for them to be compared. It bounds the comparison's work
// and memory, about mostEdits squared numbers; two versions further apart than this conflict whole.
const mostEdits = 1000

// isText says whether a file's bytes are text a person reads by lines: UTF-8, with no zero byte, and not too large to merge.
func isText(data []byte) bool {
	return len(data) <= mostMergeBytes && utf8.Valid(data) && bytes.IndexByte(data, 0) < 0
}

// mergeLines merges ours and theirs, two changes of base. It says false when they change the same lines, or lines that touch.
func mergeLines(base, ours, theirs []byte) ([]byte, bool) {
	b, o, t := lines(base), lines(ours), lines(theirs)
	inOurs, ok := matches(b, o)
	if !ok {
		return nil, false
	}
	inTheirs, ok := matches(b, t)
	if !ok {
		return nil, false
	}
	var out strings.Builder
	i, x, y := 0, 0, 0
	for i < len(b) || x < len(o) || y < len(t) {
		if i < len(b) && inOurs[i] == x && inTheirs[i] == y {
			out.WriteString(b[i])
			i, x, y = i+1, x+1, y+1
			continue
		}
		next := i
		for next < len(b) && (inOurs[next] < 0 || inTheirs[next] < 0) {
			next++
		}
		toX, toY := len(o), len(t)
		if next < len(b) {
			toX, toY = inOurs[next], inTheirs[next]
		}
		chunk, ok := settle(b[i:next], o[x:toX], t[y:toY])
		if !ok {
			return nil, false
		}
		for _, line := range chunk {
			out.WriteString(line)
		}
		i, x, y = next, toX, toY
	}
	return []byte(out.String()), true
}

// settle is what a stretch of the file becomes when each side may have changed it: the side that changed it, or either when both
// changed it the same. Both changing it differently is a conflict.
func settle(base, ours, theirs []string) ([]string, bool) {
	switch {
	case slices.Equal(ours, base):
		return theirs, true
	case slices.Equal(theirs, base), slices.Equal(ours, theirs):
		return ours, true
	}
	return nil, false
}

// lines splits text after each newline, keeping it, so joining them gives the text back.
func lines(text []byte) []string {
	split := strings.SplitAfter(string(text), "\n")
	if split[len(split)-1] == "" {
		split = split[:len(split)-1]
	}
	return split
}

// matches is, for each line of a, the line of b it stays as, or -1 when it went: a longest run of lines both keep, in order.
// It says false when the two differ by more than mostEdits lines.
func matches(a, b []string) ([]int, bool) {
	found := make([]int, len(a))
	for i := range found {
		found[i] = -1
	}
	front := 0
	for front < len(a) && front < len(b) && a[front] == b[front] {
		found[front] = front
		front++
	}
	back := 0
	for back < len(a)-front && back < len(b)-front && a[len(a)-1-back] == b[len(b)-1-back] {
		found[len(a)-1-back] = len(b) - 1 - back
		back++
	}
	pairs, ok := shortestEdit(a[front:len(a)-back], b[front:len(b)-back])
	for _, pair := range pairs {
		found[front+pair[0]] = front + pair[1]
	}
	return found, ok
}

// shortestEdit is the lines a and b have in common along the fewest insertions and deletions that turn a into b, as pairs of
// their indexes, found by Myers' O(ND) algorithm. trace keeps, for each number of edits d, the furthest each diagonal reached
// with one edit fewer, which is what walking back needs.
func shortestEdit(a, b []string) ([][2]int, bool) {
	n, m := len(a), len(b)
	limit := min(n+m, mostEdits)
	off := limit + 1
	reach := make([]int, 2*limit+3)
	var trace [][]int
	for d := 0; d <= limit; d++ {
		trace = append(trace, slices.Clone(reach[off-d-1:off+d+2]))
		for k := -d; k <= d; k += 2 {
			x := reach[off+k-1] + 1
			if k == -d || (k != d && reach[off+k-1] < reach[off+k+1]) {
				x = reach[off+k+1]
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x, y = x+1, y+1
			}
			reach[off+k] = x
			if x >= n && y >= m {
				return walkBack(trace, n, m), true
			}
		}
	}
	return nil, false
}

// walkBack follows the trace from the end of both texts to their start, and gives the lines kept on the way in order.
func walkBack(trace [][]int, n, m int) [][2]int {
	var pairs [][2]int
	x, y := n, m
	for d := len(trace) - 1; d >= 0; d-- {
		at := func(k int) int { return trace[d][k+d+1] }
		k := x - y
		from := k - 1
		if k == -d || (k != d && at(k-1) < at(k+1)) {
			from = k + 1
		}
		fromX := at(from)
		fromY := fromX - from
		for x > fromX && y > fromY {
			x, y = x-1, y-1
			pairs = append(pairs, [2]int{x, y})
		}
		x, y = fromX, fromY
	}
	slices.Reverse(pairs)
	return pairs
}
