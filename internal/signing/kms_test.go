package signing

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
)

// service stands in for KMS: it holds a private key, hands out only the public
// half, and signs the way a real KMS key does for ED25519_SHA_512 with a RAW
// message, which is plain Ed25519 over at most 4096 bytes. The size limit is
// what a record too large to sign whole runs into.
type service struct {
	private ed25519.PrivateKey
	spec    types.KeySpec
	usage   types.KeyUsageType
	algs    []types.SigningAlgorithmSpec
	// stranger, when set, signs in place of the key the service names.
	stranger ed25519.PrivateKey
	calls    int
}

func newService(t *testing.T) *service {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &service{private: private, spec: types.KeySpecEccNistEdwards25519, usage: types.KeyUsageTypeSignVerify,
		algs: []types.SigningAlgorithmSpec{types.SigningAlgorithmSpecEd25519Sha512, types.SigningAlgorithmSpecEd25519PhSha512}}
}

func (s *service) GetPublicKey(_ context.Context, in *kms.GetPublicKeyInput, _ ...func(*kms.Options)) (*kms.GetPublicKeyOutput, error) {
	if *in.KeyId != "alias/gusto" {
		return nil, errors.New("NotFoundException")
	}
	der, err := x509.MarshalPKIXPublicKey(s.private.Public())
	if err != nil {
		return nil, err
	}
	return &kms.GetPublicKeyOutput{KeySpec: s.spec, KeyUsage: s.usage, SigningAlgorithms: s.algs, PublicKey: der}, nil
}

func (s *service) Sign(_ context.Context, in *kms.SignInput, _ ...func(*kms.Options)) (*kms.SignOutput, error) {
	s.calls++
	if in.SigningAlgorithm != types.SigningAlgorithmSpecEd25519Sha512 || in.MessageType != types.MessageTypeRaw {
		return nil, errors.New("ValidationException: ED25519_SHA_512 signs a RAW message")
	}
	if len(in.Message) > 4096 {
		return nil, errors.New("ValidationException: a RAW message is at most 4096 bytes")
	}
	signer := s.private
	if s.stranger != nil {
		signer = s.stranger
	}
	return &kms.SignOutput{Signature: ed25519.Sign(signer, in.Message)}, nil
}

func opened(t *testing.T, s *service) Signer {
	t.Helper()
	signer, err := newKMSKey(context.Background(), s, "alias/gusto")
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

// A record too large for a key service to sign whole is still signed, because
// what it signs is the digest, and a verifier that knows nothing of KMS checks
// it against the public key alone.
func TestAKMSKeySignsARecordOfAnySizeAndAVerifierChecksIt(t *testing.T) {
	signer := opened(t, newService(t))
	manifest := []byte(strings.Repeat(`{"path":"out/a-file","sha256":"00"}`, 4000))

	signature, err := Sign(signer, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if signature.Scheme != SchemeDigest {
		t.Errorf("the scheme is %q", signature.Scheme)
	}
	if err := signature.Intact(manifest); err != nil {
		t.Fatalf("the signature does not hold: %v", err)
	}
	if err := signature.Intact(append(manifest, ' ')); !errors.Is(err, ErrMismatch) {
		t.Errorf("a changed record was not refused: %v", err)
	}
	text, err := signature.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if read, err := Decode("manifest.sig", text); err != nil || read.Intact(manifest) != nil {
		t.Fatalf("the signature file does not round trip: %v", err)
	}
}

// The two schemes do not stand in for each other: a signature over the digest
// is not a signature over the bytes, and the reverse.
func TestADigestSignatureIsNotAPlainOneOrTheReverse(t *testing.T) {
	signer := opened(t, newService(t))
	message := []byte("the record")
	digested, err := Sign(signer, message)
	if err != nil {
		t.Fatal(err)
	}
	digested.Scheme = Scheme
	if err := digested.Intact(message); !errors.Is(err, ErrMismatch) {
		t.Errorf("a digest signature passed as plain: %v", err)
	}

	key := generatedIn(t, t.TempDir(), "acme")
	plain, err := Sign(key, message)
	if err != nil {
		t.Fatal(err)
	}
	plain.Scheme = SchemeDigest
	if err := plain.Intact(message); !errors.Is(err, ErrMismatch) {
		t.Errorf("a plain signature passed as a digest one: %v", err)
	}
}

func generatedIn(t *testing.T, dir, name string) Key {
	t.Helper()
	key, err := Generate(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// A key that cannot sign a record is refused when it is opened, before any run
// starts, and the refusal says what it is and what was needed.
func TestAKMSKeyThatCannotSignARecordIsRefusedWhenOpened(t *testing.T) {
	for name, change := range map[string]func(*service){
		"a symmetric or other key type":  func(s *service) { s.spec = types.KeySpecEccNistP256 },
		"a key for encryption":           func(s *service) { s.usage = types.KeyUsageTypeEncryptDecrypt },
		"a key without the signing form": func(s *service) { s.algs = []types.SigningAlgorithmSpec{types.SigningAlgorithmSpecEd25519PhSha512} },
	} {
		t.Run(name, func(t *testing.T) {
			s := newService(t)
			change(s)
			if _, err := newKMSKey(context.Background(), s, "alias/gusto"); err == nil {
				t.Fatal("the key was accepted")
			}
		})
	}
	if _, err := newKMSKey(context.Background(), newService(t), "alias/missing"); err == nil || !strings.Contains(err.Error(), "alias/missing") {
		t.Errorf("a key that is not there was not named: %v", err)
	}
	if _, err := newKMSKey(context.Background(), newService(t), ""); err == nil {
		t.Error("an empty key was accepted")
	}
}

// If the service answers with a signature the key it names did not make, no
// record is sealed with it.
func TestASignatureThatDoesNotVerifyAgainstTheKeyIsNeverReturned(t *testing.T) {
	s := newService(t)
	signer := opened(t, s)
	_, stranger, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s.stranger = stranger
	if _, err := Sign(signer, []byte("the record")); err == nil || !strings.Contains(err.Error(), "does not verify") {
		t.Fatalf("a signature from another key was returned: %v", err)
	}
}

func TestAKMSKeyIsNamedByThePublicKeyAFileKeyringHolds(t *testing.T) {
	s := newService(t)
	signer := opened(t, s)
	ring := t.TempDir()
	if err := WritePublic(ring, "prod", signer.Public()); err != nil {
		t.Fatal(err)
	}
	keyring, err := OpenKeyring(ring)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := Sign(signer, []byte("the record"))
	if err != nil {
		t.Fatal(err)
	}
	named, err := keyring.Signer(signature, []byte("the record"))
	if err != nil || named.Name != "prod" {
		t.Fatalf("the keyring named %q: %v", named.Name, err)
	}
}
