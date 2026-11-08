package computer

import (
	"crypto/rand"
	"encoding/hex"
)

// SpareMarker begins the name of a sandbox that was made for no person yet. It is not a computer's marker: until a person's
// record names it, nothing of the organization's record owns it, and the comparison with the gateway does not take it for
// a computer that lost its record.
const SpareMarker = "s-"

// SparePrefix is the organization's part of its spare sandboxes' names, so a server that was killed can find what it left.
func SparePrefix(org string) string { return SpareMarker + digest("org\x00"+org) + "-" }

// SpareName is a name for a new spare sandbox: 19 characters, which a gateway takes, and never the same twice.
func SpareName(org string) string {
	random := make([]byte, 4)
	if _, err := rand.Read(random); err != nil {
		panic("no randomness to name a sandbox with: " + err.Error())
	}
	return SparePrefix(org) + hex.EncodeToString(random)
}

// Spares are sandboxes made ahead of the people they are for.
type Spares interface {
	// Take hands over a spare, to one caller only, or says there is none.
	Take() (sandbox string, ok bool)
	// Return gives back one that was taken and not used.
	Return(sandbox string)
}

func takeFrom(spares Spares) (string, bool) {
	if spares == nil {
		return "", false
	}
	return spares.Take()
}
