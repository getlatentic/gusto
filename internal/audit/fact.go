package audit

import "encoding/json"

// The words an export writes where a value would be. Each one is a statement a
// reader can act on, which a missing field is not: a field left out reads the
// same as a field nobody thought of.
const (
	// NotRecorded is something the platform does not write down yet.
	NotRecorded = "not recorded"
	// Redacted is something the export's policy removed. The redactions list
	// says which rule removed it and from which file.
	Redacted = "redacted"
	// None is something that was recorded as not happening, such as a gate for
	// a turn in a conversation, which claims nothing passed.
	None = "none"
	// NotMeasured is a sum the proxy could not make, because an answer it
	// counts carried no usage or the model has no price. It is never written
	// as zero, which would read as free.
	NotMeasured = "not measured"
)

// Fact is a value the export either knows or says plainly why it does not. It
// is written as the value itself, or as one of the words above in its place.
type Fact[T any] struct {
	value  T
	absent string
}

func known[T any](value T) Fact[T] { return Fact[T]{value: value} }

func absent[T any](why string) Fact[T] { return Fact[T]{absent: why} }

func (f Fact[T]) MarshalJSON() ([]byte, error) {
	if f.absent != "" {
		return json.Marshal(f.absent)
	}
	return json.Marshal(f.value)
}
