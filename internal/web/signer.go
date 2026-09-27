package web

import (
	"github.com/getlatentic/haas/internal/signing"
)

// UseKeyring gives the pages the keyring that names a record's signer. Without
// one, a signed record says it was not checked rather than repeat the name its
// manifest claims.
func (s *Server) UseKeyring(keyring signing.Keyring) { s.keyring = keyring }
