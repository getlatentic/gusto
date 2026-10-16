# Sign in with your identity provider

`gusto serve` signs people in through any OpenID Connect provider, and the shared
password stays as a way in for the people named in `GUSTO_ADMINS`. The server
does not store passwords for people. It trusts the provider's answer, and the
provider's groups decide what each person may do.

## What to set at the provider

| At the provider | Value |
| --- | --- |
| Application type | web, with a client secret |
| Redirect address | `https://<the pages' address>/auth/callback` |
| Scopes | `openid` and `email`, which the server always asks for, and the scope that makes the provider send groups. For many providers that is `groups` |
| Groups in the token | the claim named by `GUSTO_OIDC_GROUPS_CLAIM`, `groups` unless you say otherwise, as a list or as one string |

The server reaches the provider's discovery document at startup. A wrong issuer
stops the server there, not at somebody's first sign-in.

## What to set on the server

| Setting | Says |
| --- | --- |
| `GUSTO_OIDC_ISSUER` | the provider's issuer address. Naming it turns single sign-on on |
| `GUSTO_OIDC_CLIENT_ID`, `GUSTO_OIDC_CLIENT_SECRET` | the application's id and secret |
| `GUSTO_OIDC_REDIRECT` | the redirect address above, exactly as the provider holds it |
| `GUSTO_OIDC_SCOPES` | scopes besides `openid` and `email`, comma separated |
| `GUSTO_OIDC_GROUPS_CLAIM` | where the provider puts somebody's groups |
| `GUSTO_RUNNER_GROUPS` | groups whose members are runners: they start agents and see their own work |
| `GUSTO_ADMIN_GROUPS` | groups whose members are admins: they also see everybody's work and change what agents are given. An admin group outranks a runner group |
| `GUSTO_ALLOWED` | addresses, or a domain written `@example.com`, that may sign in. Optional when groups are named; with no groups it is the whole gate, and everybody it lets in is a runner |
| `GUSTO_ADMINS` | addresses that are admins whatever their groups say |

The server refuses to start with a provider and neither a group nor an allowed
list, because anybody with an account at that provider could then spend the
server's model credential. A person the provider signs in but no group maps to
is sent back to the login page with a refusal, and the refusal is in the record
of what changed on the platform ([CHANGES.md](CHANGES.md)).

On Compose these go in `.env` ([deploy/compose/env.example](../deploy/compose/env.example)).
On Kubernetes they are `auth.oidc` in the chart's values.

## What is checked on each release

`install-check` starts [Dex](https://dexidp.io/) beside each install, with its
mock connector, which signs in one fixed person who is in the group `authors`. It
then:

1. sets `GUSTO_ADMIN_GROUPS=authors`, signs in through the provider, and reads an
   admin page;
2. on Compose, sets both group lists to a group that person is not in, signs in
   again, and expects the refusal.

Dex answers with a groups claim, so the path from a provider's groups to a role
runs against a real provider's tokens. No company's own provider has been tried.
