# Signing keys

gusto signs each job's `manifest.json` on the machine that runs the job, after
the box has stopped. The private key never enters a job directory or a
container. A verifier needs only the public key.

| Setting | Holds | Where |
| --- | --- | --- |
| `GUSTO_SIGNING_KEY` | the private key: a file, or `awskms://<key>` | the machine that runs jobs |
| `GUSTO_KEYRING` | a directory of `<name>.pub` files | every machine that verifies |

The file name of a public key is the signer's name. `gusto verify` prints it.

## Make a key

```bash
gusto keygen acme-2026-09 -dir /etc/gusto/keys
```

```
key sha256:8bb9e1a2bf73b9dbdc479f688ee0c705
acme-2026-09.key  the private key, for GUSTO_SIGNING_KEY; keep it off the job directory
acme-2026-09.pub  the public key, for GUSTO_KEYRING
```

The private key file has mode 0600. `keygen` refuses to replace a key.

## Rotate

A rotation makes a second key and keeps the first key's public file. Every
record the first key signed still verifies and still names it.

1. Make the new key: `gusto keygen acme-2026-10 -dir /etc/gusto/keys`.
2. Set `GUSTO_SIGNING_KEY` to `acme-2026-10.key` and restart the server.
3. Keep `acme-2026-09.pub` in every `GUSTO_KEYRING`.

`gusto verify` names the key for each record:

```
signed by acme-2026-09 (key sha256:8bb9e1a2bf73b9dbdc479f688ee0c705)
signed by acme-2026-10 (key sha256:44a41e1e4c5db73b143856acf97c04d6)
```

Destroy the old private key when nothing needs it to sign. Never delete its
public file.

## Check the keys

```bash
gusto keys -jobs /var/lib/gusto/jobs
```

```
    1 signed by acme-2026-09
    1 signed by acme-2026-10
```

Run it before you remove a public file, and after you restore one. A record
whose key is not in the keyring is not tampered with. Nobody can say whose it
is. The check lists it by key id and exits 1:

```
    1 signed by acme-2026-10
    1 signed by sha256:8bb9e1a2bf73b9dbdc479f688ee0c705, which is not in the keyring: add its .pub file to GUSTO_KEYRING
```

A record whose signature does not hold for its manifest is listed by job name
and also exits 1.

## Back up and restore

Back up two things:

- the private key file, to storage that only the operator can read;
- every public file, which is safe to keep anywhere.

Restore on a clean machine:

1. Copy the files back. Keep the private key at mode 0600.
2. Set `GUSTO_SIGNING_KEY` and `GUSTO_KEYRING`.
3. Run `gusto keys`. It exits 0 when every signed record names a key.

A restored key has the same id, so what it signed before verifies and what it
signs next names the same signer. The test
`TestAKeyRestoredFromABackupOnACleanMachineSignsAsTheSameKey` does these steps.

Without a backup, a lost private key means no new record can be signed under
that name. Records it already signed stay verifiable while its public file is
in the keyring.

## Keep the key in AWS KMS

A KMS key never leaves the service. A machine that is broken into holds no key
to steal, and it stops signing when its access to the key ends.

1. Make a KMS key with key spec `ECC_NIST_EDWARDS25519` and usage `SIGN_VERIFY`.
2. Allow the machine's role `kms:GetPublicKey` and `kms:Sign` on that key.
3. Write the public file: `gusto keygen acme-prod -dir /etc/gusto/keys -awskms alias/gusto-signing`.
4. Set `GUSTO_SIGNING_KEY=awskms://alias/gusto-signing`.

gusto reads the AWS credentials the usual way: an instance role, single sign-on
or the environment. It checks the key when a job starts, so a wrong key or a
missing permission stops the job before the agent works.

KMS signs at most 4096 bytes at a time, and a manifest can be larger. So gusto
has KMS sign the 64-byte SHA-512 digest of the manifest with plain Ed25519
(`ED25519_SHA_512`, message type `RAW`). The signature file says
`"scheme": "ed25519-sha512"`. Any tool that checks plain Ed25519 checks it: the
message is the digest, which `sha512sum` prints. The public key and its id are
the same kind as a file key's.

A gusto build older than this one refuses that scheme, so upgrade every verifier
before the first KMS-signed record reaches it.

The live test signs a 210,000-byte record with a real KMS key, and checks it with
gusto and with plain Ed25519 alone. Run it on your own key:

```bash
GUSTO_TEST_KMS_KEY=alias/gusto-signing go test -tags kmslive -run Live ./internal/signing
```
