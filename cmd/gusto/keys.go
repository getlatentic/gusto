package main

import (
	"context"
	"flag"
	"fmt"
	"sort"

	"github.com/getlatentic/gusto/internal/evidence"
	"github.com/getlatentic/gusto/internal/signing"
)

type keysFlags struct{ tasks string }

func declareKeys(set *flag.FlagSet) *keysFlags {
	f := &keysFlags{}
	set.StringVar(&f.tasks, "tasks", "", "the `directory` of tasks to read (default $GUSTO_JOBS, else jobs)")
	return f
}

// checkKeys says which key stands behind every signed record in the tasks
// directory, and which records no key in the keyring can vouch for. It is what
// to run before a public key is removed from a keyring, and after one is
// restored. The exit code is 1 when any record cannot be attributed.
func checkKeys(args []string) int {
	set := newSet("keys")
	f := declareKeys(set)
	if code, ok := parseFlags("keys", set, args); !ok {
		return code
	}
	if set.NArg() > 0 {
		return usageError("keys", "takes no arguments")
	}
	keyring, err := signing.ConfiguredKeyring()
	if err != nil {
		return fail(err)
	}
	found, err := evidence.SignersUnder(orSetting(f.tasks, "GUSTO_JOBS", "jobs"), keyring)
	if err != nil {
		return fail(err)
	}
	names := make([]string, 0, len(found.Signed))
	for name := range found.Signed {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf("%5d signed by %s\n", found.Signed[name], name)
	}
	if found.Unsigned > 0 {
		fmt.Printf("%5d not signed\n", found.Unsigned)
	}
	for id, count := range found.Unknown {
		fmt.Printf("%5d signed by %s, which is not in the keyring: add its .pub file to %s\n", count, id, signing.KeyringEnv)
	}
	for _, name := range found.Broken {
		fmt.Printf("      %s: the signature does not hold, or cannot be read\n", name)
	}
	if !found.Sound() {
		return 1
	}
	return 0
}

// signerFromKMS writes the public key of a key AWS KMS holds into a keyring
// directory, which is all a key service key has to put there.
func signerFromKMS(dir, name, keyID string) int {
	signer, err := signing.OpenKMS(context.Background(), keyID)
	if err != nil {
		return fail(err)
	}
	if err := signing.WritePublic(dir, name, signer.Public()); err != nil {
		return fail(err)
	}
	fmt.Printf("key %s\n%s.pub  the public key, for %s\n%s=%s%s  on the machine that signs; the private key stays in KMS\n",
		signer.ID(), name, signing.KeyringEnv, signing.KeyEnv, signing.KMSPrefix, keyID)
	return 0
}
