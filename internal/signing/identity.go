package signing

import "context"

// Identity is who stands behind the records a runner writes: the key it signs them with, if it has one, and the name it gives.
type Identity struct {
	Key  Signer
	Name string
}

// Open is the identity of a runner with the key at path and the name given. No path is no key, which leaves a run unsigned
// rather than failing it; a path that is not a key is an error, because a runner that meant to sign must not quietly stop.
func Open(ctx context.Context, path, name string) (Identity, error) {
	if path == "" {
		return Identity{Name: name}, nil
	}
	key, err := signerFor(ctx, path)
	if err != nil {
		return Identity{}, err
	}
	return Identity{Key: key, Name: name}, nil
}

// Signed says the identity holds a key.
func (i Identity) Signed() bool { return i.Key != nil }

// Says is the name a manifest gives for who stands behind it: the name given, the key's id, or unsigned. It is the runner's own
// word: a verifier names the signer from its keyring instead.
func (i Identity) Says() string {
	switch {
	case i.Name != "":
		return i.Name
	case i.Key != nil:
		return i.Key.ID()
	}
	return "unsigned"
}
