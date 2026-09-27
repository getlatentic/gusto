package web

import (
	"github.com/getlatentic/haas/internal/evidence"
	"github.com/getlatentic/haas/internal/job"
	"github.com/getlatentic/haas/internal/signing"
)

// UseKeyring gives the pages the keyring that names a record's signer. Without
// one, a signed record says it was not checked rather than repeat the name its
// manifest claims.
func (s *Server) UseKeyring(keyring signing.Keyring) { s.keyring = keyring }

// signedBy is who signed the record, in the words every surface uses: the
// keyring's name for the key, never the manifest's signer field.
func (s *Server) signedBy(dir job.Dir) string { return evidence.SignedBy(dir, s.keyring) }
