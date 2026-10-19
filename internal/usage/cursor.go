package usage

import (
	"encoding/base64"
	"errors"
	"strings"
)

// ErrBadCursor is a cursor that the method and sort it was given to did not make.
var ErrBadCursor = errors.New("usage: invalid cursor")

// maxLimit is the most rows a page holds.
const maxLimit = 1000

// A cursor holds the sort key of the last row of a page, so a listing continues after it however the rows
// around it have changed. Its first part names the listing that made it, so that a cursor from one listing
// is refused by another. The last part may hold any text.
func encodeCursor(kind string, parts ...string) Cursor {
	return Cursor(base64.RawURLEncoding.EncodeToString([]byte(kind + "|" + strings.Join(parts, "|"))))
}

// decodeCursor returns the n parts of a cursor made for the listing kind.
func decodeCursor(c Cursor, kind string, n int) ([]string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(string(c))
	if err != nil {
		return nil, ErrBadCursor
	}
	parts := strings.SplitN(string(raw), "|", n+1)
	if len(parts) != n+1 || parts[0] != kind {
		return nil, ErrBadCursor
	}
	return parts[1:], nil
}

func pageLimit(p Page) int {
	if p.Limit <= 0 {
		return 50
	}
	return min(p.Limit, maxLimit)
}
