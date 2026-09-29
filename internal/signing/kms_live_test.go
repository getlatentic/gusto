//go:build kmslive

package signing

import (
	"context"
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

// This runs against a real KMS key, which the ordinary tests stand in for. Make
// a key with key spec ECC_NIST_EDWARDS25519 and usage SIGN_VERIFY, then:
//
//	HAAS_TEST_KMS_KEY=alias/haas-signing-test go test -tags kmslive -run Live ./internal/signing
func TestLiveKMSSignsARecordFarLargerThanKMSWillSignWhole(t *testing.T) {
	keyID := os.Getenv("HAAS_TEST_KMS_KEY")
	if keyID == "" {
		t.Skip("HAAS_TEST_KMS_KEY names no key")
	}
	signer, err := OpenKMS(context.Background(), keyID)
	if err != nil {
		t.Fatal(err)
	}
	record := []byte(strings.Repeat(`{"path":"out/a-file","sha256":"00"}`, 6000))

	signature, err := Sign(signer, record)
	if err != nil {
		t.Fatal(err)
	}
	if err := signature.Intact(record); err != nil {
		t.Fatalf("haas does not accept what KMS signed: %v", err)
	}
	if err := signature.Intact(append(record, ' ')); err == nil {
		t.Error("a changed record was accepted")
	}

	// The check a stranger with the public key would make, with nothing of haas.
	raw, err := base64.StdEncoding.DecodeString(signature.Value)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha512.Sum512(record)
	if !ed25519.Verify(signer.Public(), digest[:], raw) {
		t.Fatalf("plain Ed25519 over the SHA-512 of the %d-byte record does not verify", len(record))
	}
	t.Logf("a %d-byte record signed by %s with %s", len(record), signer.ID(), signature.Scheme)
}
