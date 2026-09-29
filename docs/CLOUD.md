# The public cloud

`haas cloud` is the front door of a hosted haas. A person signs up, creates an
organisation, and the organisation gets a haas server of its own. This page
says how the parts divide, and what each stage of the build adds.

## How organisations are kept apart

Each organisation runs its own `haas serve` process. It has its own data
directory, signing key, credentials, spend ledger, change log and job queue.
The front door holds accounts and routes requests. It runs no jobs.

```
browser ──► front door (haas cloud) ──► the organisation's haas serve
             accounts, sign-in,          its own data directory and keys
             routing by host name        its jobs, in sandboxes on a gateway
```

| Question | Answer |
| --- | --- |
| Why not one server for every organisation? | `haas serve` holds one sign-in, one proxy, one ledger, one queue and one change log. Each would need an organisation on every read, and a read that misses one is a leak between organisations. A process and a directory are a boundary that no code path can forget to check. |
| What does it cost? | A process per active organisation, about the size of an idle `haas serve`. An organisation with no work can have its process stopped. |
| Where do jobs run? | In OpenShell sandboxes on a pool of gateway hosts. A sandbox belongs to one job, is deleted when the job ends, and is never reused for another organisation. Each sandbox is a microVM with its own kernel. |
| Why a shared gateway and not one per organisation? | A host with KVM is the cost. A gateway per organisation is the same isolation at about a hundred times the price, and the sandbox boundary is already per job. |

## Accounts

The front door keeps its records as files under `HAAS_CLOUD_ROOT`, written by
renaming a finished file into place. A slug or an email address is claimed by
creating its file exclusively, so two sign-ups cannot take the same one.

```
root/
  secret                      signs the front door's cookies, 0600
  people/<hash of email>.json a person: email, organisation, role, when verified
  orgs/<slug>/org.json        an organisation: name, admin, when made
  orgs/<slug>/data/           the organisation's haas data directory, 0700
  orgs/<slug>/data/quota.json what the organisation may run now
  tokens/<hash>.json          a single-use link for verifying or signing in
  outbox/                     mail, when mail goes to files
```

A person belongs to one organisation. Only the hash of an email address is in a
file name, and only the hash of a link's secret is stored.

## Sign-up

1. A person gives a work email and an organisation name. The address is
   checked, and refused when it is a free mail provider or a disposable one.
2. The person, the organisation and the organisation's directory are made, the
   person is signed in, and a verification link is mailed.
3. Until the link is followed, the organisation may run a small trial. Following
   it lifts the limit.

| | Unverified | Verified |
| --- | --- | --- |
| Jobs in total | 3 | 200 a month |
| Jobs at once | 1 | 4 |
| Invite colleagues | no | yes |

The numbers are settings. `quota.json` in the organisation's directory is where
the organisation's server reads them, through `HAAS_QUOTA`, and
[LIMITS.md](LIMITS.md) says what the server does with them.

### Abuse limits

| Limit | Default |
| --- | --- |
| Sign-up attempts from one address, refused ones too | 10 an hour |
| Mail to one address | 3 an hour |
| Sign-ups in all | 200 an hour |
| Organisations for one email domain | 3 |

A sign-in link is always answered the same way, whether or not the address has
an account, so the form cannot be used to find out who is a customer.

## Settings

| Setting | Meaning |
| --- | --- |
| `HAAS_CLOUD_ROOT` | where accounts are kept, `cloud` by default |
| `HAAS_CLOUD_ADDR` | where the front door listens, `127.0.0.1:8050` by default |
| `HAAS_CLOUD_URL` | the address links in mail begin with |
| `HAAS_CLOUD_DOMAIN` | the domain organisations are served under, as `<slug>.<domain>` |
| `HAAS_CLOUD_MAIL` | `outbox`, or `smtp://user:password@host:587?from=name@example.com` |
| `HAAS_CLOUD_FREE_MAIL` | `allow` lets free mail providers sign up, for a demonstration |

## What comes next

Sign-up and the organisation record come first
(getlatentic/planning#394). Running the organisation's server and giving it
capacity follows (getlatentic/planning#395), then inviting people
(getlatentic/planning#396), then the stack on AWS from code
(getlatentic/planning#397).
