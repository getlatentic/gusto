package dbkit

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSON is a column that holds a value of any type that encoding/json handles: TEXT on SQLite and jsonb on
// Postgres. A NULL column reads as the zero V.
type JSON[T any] struct{ V T }

// Value implements driver.Valuer. The JSON goes as a string, which SQLite stores as text and Postgres
// accepts for jsonb.
func (j JSON[T]) Value() (driver.Value, error) {
	b, err := json.Marshal(j.V)
	if err != nil {
		return nil, fmt.Errorf("dbkit: encode json column: %w", err)
	}
	return string(b), nil
}

// Scan implements sql.Scanner.
func (j *JSON[T]) Scan(v any) error {
	var raw []byte
	switch x := v.(type) {
	case nil:
		var zero T
		j.V = zero
		return nil
	case string:
		raw = []byte(x)
	case []byte:
		raw = x
	default:
		return fmt.Errorf("dbkit: json column held a %T", v)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("dbkit: decode json column: %w", err)
	}
	j.V = out
	return nil
}
