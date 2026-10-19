package taskindex

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrBadSort is a Page.Sort that no listing offers.
var ErrBadSort = errors.New("taskindex: unknown sort")

// ErrBadCursor is a cursor that the sort it was given to did not make.
var ErrBadCursor = errors.New("taskindex: invalid cursor")

// MostPerPage is the most rows a page holds, and DefaultPerPage the number a page of no stated size holds.
const (
	MostPerPage    = 1000
	DefaultPerPage = 50
)

// Sort names an order of rows. Every order is from the greatest, and ends in the id so that no two rows tie.
type Sort string

const (
	ByQueued Sort = "queued"
	ByEnded  Sort = "ended"
	BySeq    Sort = "seq"
)

// SortFor finds a sort by name, where the empty name is the default, queued.
func SortFor(name string) (Sort, error) {
	switch s := Sort(name); s {
	case "":
		return ByQueued, nil
	case ByQueued, ByEnded, BySeq:
		return s, nil
	}
	return "", fmt.Errorf("%w %q; use queued, ended or seq", ErrBadSort, name)
}

// Columns are the columns the sort orders by before the id, in order.
func (s Sort) Columns() []string {
	switch s {
	case ByEnded:
		return []string{"ended"}
	case BySeq:
		return []string{"seq"}
	}
	return []string{"queued", "seq"}
}

// Key is the values of the row in the sort's columns.
func (s Sort) Key(r Row) []int64 {
	switch s {
	case ByEnded:
		return []int64{r.Ended}
	case BySeq:
		return []int64{r.Seq}
	}
	return []int64{r.Queued, r.Seq}
}

// Position is the place of a row in a listing: its key and its id.
type Position struct {
	Key []int64
	ID  string
}

// PositionOf is where a row is in the sort.
func (s Sort) PositionOf(r Row) Position { return Position{Key: s.Key(r), ID: r.ID} }

// After is true when a row at p comes after the position q in the sort, whose order is from the greatest.
func (p Position) After(q Position) bool {
	for i := range p.Key {
		if p.Key[i] != q.Key[i] {
			return p.Key[i] < q.Key[i]
		}
	}
	return p.ID < q.ID
}

// NewCursor continues a listing after the row at p. The id comes last because it may hold any text.
func (s Sort) NewCursor(p Position) Cursor {
	parts := []string{string(s)}
	for _, k := range p.Key {
		parts = append(parts, strconv.FormatInt(k, 10))
	}
	return Cursor(base64.RawURLEncoding.EncodeToString([]byte(strings.Join(append(parts, p.ID), "|"))))
}

// ParseCursor is the position a cursor made for this sort holds, and ErrBadCursor for any other.
func (s Sort) ParseCursor(c Cursor) (Position, error) {
	raw, err := base64.RawURLEncoding.DecodeString(string(c))
	if err != nil {
		return Position{}, ErrBadCursor
	}
	n := len(s.Columns())
	parts := strings.SplitN(string(raw), "|", n+2)
	if len(parts) != n+2 || parts[0] != string(s) {
		return Position{}, ErrBadCursor
	}
	p := Position{ID: parts[n+1]}
	for _, text := range parts[1 : n+1] {
		k, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return Position{}, ErrBadCursor
		}
		p.Key = append(p.Key, k)
	}
	return p, nil
}

// PageSize is the number of rows a page holds.
func PageSize(p Page) int {
	if p.Limit <= 0 {
		return DefaultPerPage
	}
	return min(p.Limit, MostPerPage)
}
