# Threat model

What haas protects, from whom, and what each part trusts. The review that checked
this against the code, and what it found, is in [SECURITY.md](SECURITY.md).

## What is protected

| Asset | Why it matters |
| --- | --- |
| The model credential | Spends the organisation's money and is the one secret every job would want |
| Repository access | The GitHub App's key can reach every repository it is installed on |
| Each person's work | Their agents, transcripts and files must not be readable by a colleague |
| The record | Signed evidence is what an auditor and a reviewer rely on. Forging it defeats the product |
| The change record | Who changed what, in a chain that shows an edit |
| The host | Other apps share the box, and the server runs as a user that can read every job |
| Availability | One person's work must not stop everyone else's |

## Who is trusted

| Actor | Trusted with | Not trusted with |
| --- | --- | --- |
| Operator | Everything: the host, the vault, the playbook | |
| Admin | Every person's work, tokens, retention, skill sets | The host |
| Runner (a signed-in person) | Their own agents, jobs and spend | Anyone else's, and settings |
| API token | Exactly its scopes | Anything else, and any page |
| The agent inside a job | Its workspace and home in the box, the model through a per-job token | The record, the credential, the network, the host, other jobs |
| The repository's own code | Nothing: the gate runs it as the agent user | |
| The model's output | Nothing: it is text, drawn escaped | |

The agent is assumed hostile. It is driven by a model that reads repository
content and can be talked into anything.

## The three parts

### The web tier (`internal/web`)

Trusts the session it signed, an API token it hashed, and the identity
provider's signed token. It does not trust anything a browser or a caller sends
by itself.

| Boundary | Control |
| --- | --- |
| Who someone is | An HMAC-signed, HttpOnly, SameSite=Lax cookie carrying the role, 12 hours, ended on the server at sign-out. Google sign-in uses state, PKCE and a nonce, and checks the address is verified |
| What they may see | Every object carries who owns it. Somebody else's is answered as a missing one, so a guess learns nothing |
| Cross-site requests | The cookie is SameSite=Lax, so a form posted from another site does not carry it. No route changes state on a GET. The policy's `frame-ancestors 'none'` keeps a page from being framed, and `form-action 'self'` keeps forms to this server |
| What a page may run | A content security policy with no inline script but the ones carrying the response's nonce, no handler attributes, `object-src 'none'`, `base-uri 'none'` |
| What a caller may send | 4 MB a request, and 1 MB for a job |
| Guessing | Ten wrong passwords or bearer credentials from one address in ten minutes turns that address away, the right one included |
| What is drawn | Templates escape. Markdown from a model is rendered with raw HTML off and links restricted. An evidence file is served as plain text with `nosniff` |
| What is read from a job's record | A file, and not a link: what the agent could plant in `out/` is never opened |
| Where a repository comes from | An https or ssh address. A path on the server's disk only under a directory the operator names. Never an address git could read as an option |
| Caching | Nothing behind the sign-in is cached. HSTS is set over TLS |

### The model proxy (`internal/proxy`)

Trusts the per-job token, minted for one job and dropped when it ends. It does
not trust the container's headers, its destination names, or DNS.

| Boundary | Control |
| --- | --- |
| The credential | Added here and nowhere else. Whatever the container sent in its place is discarded. A redirect from the model is not followed |
| Which job | A token is one job's. An unknown and an ended token get the same answer |
| Spend | A cap is checked before a call and counted after it. Calls already in flight when a cap is reached can take a job past it, by at most their own cost |
| Where a job may reach | Only the hosts the operator named, ports 80 and 443 unless a port is named. A name is resolved once and the connection is made to that answer; a name that resolves to this machine or to a link-local address, such as a cloud's metadata service, is refused |
| What is recorded | Every call, sent or refused, in the job's own record |

### The runner and the box (`internal/runner`, `internal/sandbox`)

Trusts nothing the agent produced except as bytes to be hashed and shown.

| Boundary | Control |
| --- | --- |
| What the container holds | The job directory and, for a conversation, its own folder. No other mount and no runtime socket |
| The network | None but the proxy's socket when there is a proxy |
| Privilege | Root in the box only for the runner, so it alone writes the record. The harness and the gate run as an unprivileged user. Every capability is dropped but the five the runner uses, and no program can gain a privilege by being setuid |
| The record | `out/` is closed to the agent. Signing happens on the host after the box is gone, with a key the box never held |
| The workspace's git | The runner stages the workspace in a repository of its own, so the programs a workspace's `.git` configuration names are never run as root |
| Limits | Memory, CPU and processes per job |
| Secrets | Only the per-job token and the proxy address enter the box. The password, the signing key, the model keys and the GitHub App's key are read by the server alone |

## What the design does not defend against

- **A malicious operator or admin.** They can read everything and change settings.
  The change record shows what they changed and cannot be edited unseen.
- **The model.** A prompt-injected agent can do anything inside its box and can
  spend up to its cap. What it makes is checked by the gate and read by a person.
- **A harness with its own defects.** Third-party harness programs run inside the
  box. Their versions are pinned where the adapter was measured against them.
- **Data leaving through the model.** What an agent reads, it can send to the
  model provider. That is the provider's trust, chosen by the operator.
- **A compromised host.** The server holds the keys.
- **A shared address behind an untrusted proxy.** The wrong-attempt limit is per
  source address, and a proxy this server does not believe gives everyone one.
  Use the identity provider there.
- **A development machine.** On macOS container runtimes a bind mount ignores
  file permissions, so the box cannot close `out/` to the agent there. The
  server's own checks (links in `out/` are never read) still hold; the guarantee
  that the harness cannot write the record does not. Use Linux for anything real.
