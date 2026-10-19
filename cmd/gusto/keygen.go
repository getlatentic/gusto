package main

import (
	"flag"
	"fmt"

	"github.com/getlatentic/gusto/internal/signing"
)

// makeKey writes a signer's key pair: the private key for GUSTO_SIGNING_KEY on
// the machine that runs jobs, and the public key for every verifier's
// GUSTO_KEYRING, where its file name is the name verify gives the signer.
func makeKey(args []string) int {
	flags := flag.NewFlagSet("keygen", flag.ContinueOnError)
	into := flags.String("dir", ".", "where to write the two files")
	kms := flags.String("awskms", "", "an AWS KMS key to sign with instead: its id, ARN or alias")
	name, rest := split(args)
	if err := flags.Parse(rest); err != nil || name == "" || flags.NArg() > 0 {
		usage()
		return 2
	}
	if *kms != "" {
		return signerFromKMS(*into, name, *kms)
	}
	key, err := signing.Generate(*into, name)
	if err != nil {
		return fail(err)
	}
	fmt.Printf("key %s\n%s.key  the private key, for %s; keep it off the job directory\n%s.pub  the public key, for %s\n",
		key.ID(), name, signing.KeyEnv, name, signing.KeyringEnv)
	return 0
}
