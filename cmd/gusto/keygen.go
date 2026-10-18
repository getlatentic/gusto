package main

import (
	"flag"
	"fmt"

	"github.com/getlatentic/gusto/internal/signing"
)

type keygenFlags struct{ dir, awsKMS string }

func declareKeygen(set *flag.FlagSet) *keygenFlags {
	f := &keygenFlags{}
	set.StringVar(&f.dir, "dir", ".", "where to write the two files")
	set.StringVar(&f.awsKMS, "awskms", "", "an AWS KMS `key` to sign with instead: its id, ARN or alias")
	return f
}

// makeKey writes a signer's key pair: the private key for GUSTO_SIGNING_KEY on
// the machine that runs tasks, and the public key for every verifier's
// GUSTO_KEYRING, where its file name is the name verify gives the signer.
func makeKey(args []string) int {
	set := newSet("keygen")
	f := declareKeygen(set)
	name, rest := split(args)
	if code, ok := parseFlags("keygen", set, rest); !ok {
		return code
	}
	if name == "" || set.NArg() > 0 {
		return usageError("keygen", "needs one name for the key")
	}
	if f.awsKMS != "" {
		return signerFromKMS(f.dir, name, f.awsKMS)
	}
	key, err := signing.Generate(f.dir, name)
	if err != nil {
		return fail(err)
	}
	fmt.Printf("key %s\n%s.key  the private key, for %s; keep it off the task directory\n%s.pub  the public key, for %s\n",
		key.ID(), name, signing.KeyEnv, name, signing.KeyringEnv)
	return 0
}
