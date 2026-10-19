package taskindex

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

const fieldSeparator = "\x1f"

// SearchText is what Search looks in: the fields it covers, lower-cased, so that a match is one substring
// test on one column in either database, which takes % and _ as the characters they are.
func (r Row) SearchText() string {
	return strings.ToLower(strings.Join([]string{r.ID, r.Brief, r.Asked, r.Repo}, fieldSeparator))
}

// Matches is true of a search that the row's SearchText holds. A search that holds the field separator
// matches nothing.
func (r Row) Matches(search string) bool {
	return !strings.Contains(search, fieldSeparator) && strings.Contains(r.SearchText(), strings.ToLower(search))
}

// Sum is a digest of every field of the row. Two rows that differ in any field have different sums.
func (r Row) Sum() string {
	h := sha256.New()
	for _, s := range []string{r.ID, r.State, r.Brief, r.Repo, r.AgentType, r.Model, r.Instance, r.Thread, r.Asked,
		r.RequestKey, r.StoppedBy, r.Team} {
		h.Write([]byte(strconv.Itoa(len(s)) + ":" + s))
	}
	for _, n := range []int64{r.Seq, int64(r.Exit), r.Queued, r.Started, r.Ended} {
		h.Write([]byte(strconv.FormatInt(n, 10) + ","))
	}
	return hex.EncodeToString(h.Sum(nil)[:12])
}

// Entry is what a rebuild compares the row with.
func (r Row) Entry() Entry { return Entry{State: r.State, Ended: r.Ended, Seq: r.Seq, Sum: r.Sum()} }
