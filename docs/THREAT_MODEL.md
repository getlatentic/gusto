# Threat model

gusto protects the assets below from a hostile agent, a hostile caller and a curious colleague. It assumes the agent is hostile, because a model that reads repository content can be talked into anything. The review that checked this against the code is [SECURITY.md](SECURITY.md).

## What is protected

| Asset | Why it matters |
| --- | --- |
| The model credential | Spends the organization's money. Every task would want it |
| Repository access | The GitHub App's key reaches every repository it is installed on |
| Each person's work | Agents, transcripts and files must not be readable by a colleague |
| The record and the change record | Signed record is what an auditor relies on, and forging it defeats the product. The change record shows who changed what, in a chain that shows an edit |
| The host | Other apps share the box, and the server's user can read every task |
| Availability | One person's work must not stop everyone else's |

## Who is trusted

| Actor | Trusted with | Not trusted with |
| --- | --- | --- |
| Operator | Everything: the host, the vault, the playbook | |
| Admin | Every person's work, tokens, retention, skill sets | The host |
| Runner (a signed-in person) | Their own agents, tasks and spend | Anyone else's, and settings |
| API token | Exactly its scopes | Anything else, and any page |
| The agent in a task | Its workspace and home in the box, the model through a per-task token | The record, the credential, the network, the host, other tasks |
| The repository's code | Nothing. The check runs it as the agent user | |
| The model's output | Nothing. It is text, drawn escaped | |

## The web tier (`internal/web`)

It trusts the session it signed, an API token it hashed, and the identity provider's signed token. It trusts nothing else a browser or caller sends.

| Boundary | Control |
| --- | --- |
| Who someone is | An HMAC-signed, HttpOnly, SameSite=Lax cookie carrying the role, 12 hours, ended on the server at sign-out. Google sign-in uses state, PKCE and a nonce, and checks the address is verified |
| What they may see | Every object carries its owner. Somebody else's is answered as missing, so a guess learns nothing |
| Cross-site requests | SameSite=Lax keeps the cookie off a form posted from another site. No route changes state on a GET. `frame-ancestors 'none'` blocks framing and `form-action 'self'` keeps forms to this server, and, behind gusto cloud, to gusto cloud, where sign-out continues |
| What a page may run | A content security policy: no inline script except with the response's nonce, no handler attributes, `object-src 'none'`, `base-uri 'none'` |
| Size and guessing | 4 MB a request, 1 MB for a task. Ten wrong passwords or bearer credentials from one address in ten minutes turns that address away, the right one included |
| What is drawn | Templates escape. Model Markdown has raw HTML off and links restricted. Record files are plain text with `nosniff`. A task's record is read as files, never links, so what the agent plants in `out/` is never opened |
| Repository source and caching | An https or ssh address. A path on the server's disk only under a directory the operator names. Never an address git could read as an option. Nothing behind the sign-in is cached, and HSTS is set over TLS |

## The model proxy (`pkg/proxy`)

It trusts the per-task token, minted for one task and dropped when it ends. It trusts nothing from the container: not its headers, destination names or DNS.

| Boundary | Control |
| --- | --- |
| Credential and task | The credential is added here only. Whatever the container sent in its place is discarded. A redirect from the model is not followed. A token is one task's, and an unknown and an ended token get the same answer |
| Spend and record | A cap is checked before a call and counted after. Calls in flight when a cap is reached can pass it, by at most their own cost. Every call, sent or refused, is in the task's record |
| Where a task may reach | Only the hosts the operator named, on ports 80 and 443 unless a port is named. A name resolves once and the connection goes to that answer. A name resolving to this machine or a link-local address, such as a cloud metadata service, is refused |

## The runner and the box (`internal/runner`, `internal/sandbox`)

It treats everything the agent produced as bytes to hash and show.

| Boundary | Control |
| --- | --- |
| Mounts and network | The task directory and, for a conversation, its own folder. No other mount, no runtime socket, and no network but the proxy's socket |
| Privilege | Only the runner is root in the box, so it alone writes the record. The agent and check run unprivileged. All capabilities are dropped but the five the runner uses, and no setuid program gains privilege. The runner stages the workspace in a repository of its own, so programs named by the workspace's `.git` configuration never run as root |
| The record | `out/` is closed to the agent. Signing happens on the host after the box is gone, with a key the box never held |
| Limits and secrets | Memory, CPU and processes are limited per task. Only the per-task token and the proxy address enter the box. The password, signing key, model keys and GitHub App key are read by the server alone |

## The OpenShell backend

With `GUSTO_SANDBOX=openshell` the agent runs in a microVM on a gateway and the runner stays on the host. The gateway controls every sandbox and is trusted as the server is. It differs from a container as follows.

| Boundary | Control |
| --- | --- |
| The record and the check | The runner writes the record on the host after the agent finishes, and is never root inside a sandbox. The check runs in the sandbox, and `gusto verify` re-runs it where verify is run |
| Network | A deny-by-default policy per task: the proxy and the operator's allowed hosts. The sandbox's log of what it allowed and denied is in the record and signed |
| What goes in and comes back | It is given the proxy address, a per-task token and skills, and nothing of the server's environment. The workspace comes back with links kept as links and never followed |
| The token | It never shows on the host's process list. It enters on the standard input of `openshell sandbox exec`, or for an ssh agent in a file only the sandbox's user can read, which the command removes as it starts. Inside, it is in the agent's environment. It is worthless once the task ends |

## Not covered

- A malicious operator or admin can read everything and change settings. The change record shows what they changed and cannot be edited unseen.
- A prompt-injected agent can do anything inside its box and spend up to its cap. The check checks what it makes and a person reads it.
- Third-party agent programs run inside the box with their own defects. Versions are pinned where the adapter was measured. What an agent reads, it can send to the model provider, which the operator chooses. A compromised host is not defended, because the server holds the keys.
- The wrong-attempt limit is per source address. A proxy this server does not believe gives everyone one address. Use the identity provider there.
- On macOS container runtimes a bind mount ignores file permissions, so the box cannot close `out/` to the agent. Links in `out/` are still never read, but the guarantee that the agent cannot write the record is lost. Use Linux for real work.
