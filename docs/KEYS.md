# Signing keys

gusto signs each task's `manifest.json` on the machine that runs the task, after the box stops. The private key never enters a task directory or a container. A verifier needs only the public key.

| Setting | Holds | Where |
| --- | --- | --- |
| `GUSTO_SIGNING_KEY` | the private key: a file, or `awskms://<key>` | the machine that runs tasks |
| `GUSTO_KEYRING` | a directory of `<name>.pub` files | every machine that verifies |

The file name of a public key is the signer's name. `gusto verify` prints it.

## Make a key

```bash
gusto keygen acme-2026-09 -dir /etc/gusto/keys
```

```
key sha256:8bb9e1a2bf73b9dbdc479f688ee0c705
acme-2026-09.key  the private key, for GUSTO_SIGNING_KEY; keep it off the task directory
acme-2026-09.pub  the public key, for GUSTO_KEYRING
```

The private key file has mode 0600. `keygen` refuses to replace a key.

## Rotate

Records the old key signed still verify and still name it.

1. Make the new key: `gusto keygen acme-2026-10 -dir /etc/gusto/keys`.
2. Set `GUSTO_SIGNING_KEY` to `acme-2026-10.key` and restart the server.
3. Keep `acme-2026-09.pub` in every `GUSTO_KEYRING`.

Destroy the old private key when nothing needs it to sign. Never delete its public file.

## Check the keys

Run this before you remove a public file, and after you restore one:

```bash
gusto keys -tasks /var/lib/gusto/jobs
```

```
    1 signed by acme-2026-09
    1 signed by acme-2026-10
```

A record whose key is not in the keyring is not tampered with, but nobody can say whose it is. The check lists it as `signed by sha256:<id>, which is not in the keyring` and exits 1. Add the key's `.pub` file to `GUSTO_KEYRING`. A record whose signature does not hold for its manifest is listed by task name and also exits 1.

## Back up and restore

Back up the private key file to storage that only the operator can read. Back up every public file anywhere.

To restore on a clean machine:

1. Copy the files back. Keep the private key at mode 0600.
2. Set `GUSTO_SIGNING_KEY` and `GUSTO_KEYRING`.
3. Run `gusto keys`. It exits 0 when every signed record names a key.

A restored key has the same id, so earlier records still verify. Without a backup, a lost private key cannot sign new records, but records it signed stay verifiable while its public file is in the keyring.

## Keep the key in AWS KMS

A KMS key never leaves the service, and a machine stops signing when its access ends.

1. Make a KMS key with key spec `ECC_NIST_EDWARDS25519` and usage `SIGN_VERIFY`.
2. Allow the machine's role `kms:GetPublicKey` and `kms:Sign` on that key.
3. Write the public file: `gusto keygen acme-prod -dir /etc/gusto/keys -awskms alias/gusto-signing`.
4. Set `GUSTO_SIGNING_KEY=awskms://alias/gusto-signing`.

gusto reads AWS credentials the usual way: an instance role, single sign-on or the environment. It checks the key when a task starts, so a wrong key or missing permission stops the task before the agent works.

KMS signs at most 4096 bytes, so gusto has KMS sign the 64-byte SHA-512 digest of the manifest with plain Ed25519 (`ED25519_SHA_512`, message type `RAW`). The signature file says `"scheme": "ed25519-sha512"`. Any plain Ed25519 tool checks it, with the digest (as `sha512sum` prints it) as the message.

Older gusto builds refuse that scheme. Upgrade every verifier before the first KMS-signed record reaches it. Run the live test on your own key:

```bash
GUSTO_TEST_KMS_KEY=alias/gusto-signing go test -tags kmslive -run Live ./internal/signing
```
