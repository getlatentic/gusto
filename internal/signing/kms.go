package signing

import (
	"context"
	"crypto/ed25519"
	"crypto/sha512"
	"crypto/x509"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
)

// KMSPrefix in GUSTO_SIGNING_KEY names a key AWS KMS holds instead of a file:
// awskms://<key id, key ARN or alias>. The private half never leaves the
// service, so a machine that is broken into holds nothing to steal and loses
// the ability to sign the moment its access to the key is taken away.
const KMSPrefix = "awskms://"

// kmsCall is how long one call to the key service may take before a run stops.
const kmsCall = 30 * time.Second

// kmsAPI is the part of the key service a signer calls.
type kmsAPI interface {
	GetPublicKey(context.Context, *kms.GetPublicKeyInput, ...func(*kms.Options)) (*kms.GetPublicKeyOutput, error)
	Sign(context.Context, *kms.SignInput, ...func(*kms.Options)) (*kms.SignOutput, error)
}

// kmsKey is a signer whose private half is a key in AWS KMS.
type kmsKey struct {
	api    kmsAPI
	keyID  string
	public ed25519.PublicKey
}

var kmsSigners sync.Map

// OpenKMS is the signer for a KMS key, using whatever credentials the AWS
// configuration finds: an instance role, single sign-on, or the environment.
// A key that cannot sign Ed25519 is refused here, before a run starts, and not
// when the first record is sealed.
func OpenKMS(ctx context.Context, keyID string) (Signer, error) {
	if cached, found := kmsSigners.Load(keyID); found {
		return cached.(Signer), nil
	}
	loaded, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("the AWS configuration could not be read: %w", err)
	}
	signer, err := newKMSKey(ctx, kms.NewFromConfig(loaded), keyID)
	if err != nil {
		return nil, err
	}
	kmsSigners.Store(keyID, signer)
	return signer, nil
}

func newKMSKey(ctx context.Context, api kmsAPI, keyID string) (Signer, error) {
	if keyID == "" {
		return nil, errors.New(KeyEnv + " names " + KMSPrefix + " and no key")
	}
	ctx, cancel := context.WithTimeout(ctx, kmsCall)
	defer cancel()
	answer, err := api.GetPublicKey(ctx, &kms.GetPublicKeyInput{KeyId: &keyID})
	if err != nil {
		return nil, fmt.Errorf("the public key of %s could not be read from KMS: %w", keyID, err)
	}
	if answer.KeySpec != types.KeySpecEccNistEdwards25519 || answer.KeyUsage != types.KeyUsageTypeSignVerify {
		return nil, fmt.Errorf("%s is a %s key for %s, and a record is signed with an ECC_NIST_EDWARDS25519 key for SIGN_VERIFY", keyID, answer.KeySpec, answer.KeyUsage)
	}
	if !slices.Contains(answer.SigningAlgorithms, types.SigningAlgorithmSpecEd25519Sha512) {
		return nil, fmt.Errorf("%s does not offer ED25519_SHA_512, which signs a record", keyID)
	}
	parsed, err := x509.ParsePKIXPublicKey(answer.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("the public key KMS gave for %s is not readable: %w", keyID, err)
	}
	public, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("%s is not an Ed25519 key", keyID)
	}
	return &kmsKey{api: api, keyID: keyID, public: public}, nil
}

func (k *kmsKey) ID() string                { return IDOf(k.public) }
func (k *kmsKey) Public() ed25519.PublicKey { return k.public }
func (k *kmsKey) Scheme() string            { return SchemeDigest }

// Sign has KMS sign the SHA-512 digest of the message with plain Ed25519, and
// checks the answer
// against the public key before returning it, so a key that was swapped or a
// service that answered wrongly stops the run and never seals a record that
// would not verify.
func (k *kmsKey) Sign(message []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), kmsCall)
	defer cancel()
	digest := sha512.Sum512(message)
	answer, err := k.api.Sign(ctx, &kms.SignInput{
		KeyId:            &k.keyID,
		Message:          digest[:],
		MessageType:      types.MessageTypeRaw,
		SigningAlgorithm: types.SigningAlgorithmSpecEd25519Sha512,
	})
	if err != nil {
		return nil, fmt.Errorf("KMS could not sign with %s: %w", k.keyID, err)
	}
	if !verify(SchemeDigest, k.public, message, answer.Signature) {
		return nil, fmt.Errorf("the signature KMS returned for %s does not verify against its public key", k.keyID)
	}
	return answer.Signature, nil
}

// signerFor is the signer GUSTO_SIGNING_KEY names: a KMS key when it starts with
// the KMS prefix, and a file otherwise.
func signerFor(ctx context.Context, name string) (Signer, error) {
	if id, isKMS := strings.CutPrefix(name, KMSPrefix); isKMS {
		return OpenKMS(ctx, id)
	}
	return ReadKey(name)
}
