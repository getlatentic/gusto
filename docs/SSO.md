# Sign in with your identity provider

`gusto serve` signs people in through any OpenID Connect provider. The provider's groups decide what each person may do. The server stores no passwords for people. The shared password stays as a way in for the people named in `GUSTO_ADMINS`.

## At the provider

| Setting | Value |
| --- | --- |
| Application type | web, with a client secret |
| Redirect address | `https://<the pages' address>/auth/callback` |
| Scopes | `openid` and `email` (the server always asks for them), and the scope that makes the provider send groups, often `groups` |
| Groups in the token | the claim named by `GUSTO_OIDC_GROUPS_CLAIM` (`groups` by default), as a list or one string |

The server reads the provider's discovery document at startup. A wrong issuer stops the server there.

## On the server

| Setting | Says |
| --- | --- |
| `GUSTO_OIDC_ISSUER` | the provider's issuer address. Setting it turns single sign-on on |
| `GUSTO_OIDC_CLIENT_ID`, `GUSTO_OIDC_CLIENT_SECRET` | the application's id and secret |
| `GUSTO_OIDC_REDIRECT` | the redirect address, exactly as the provider holds it |
| `GUSTO_OIDC_SCOPES` | scopes besides `openid` and `email`, comma separated |
| `GUSTO_OIDC_GROUPS_CLAIM` | where the provider puts a person's groups |
| `GUSTO_RUNNER_GROUPS` | groups whose members are runners: they start agents and see their own work |
| `GUSTO_ADMIN_GROUPS` | groups whose members are admins: they also see everybody's work and change what agents are given. An admin group outranks a runner group |
| `GUSTO_ALLOWED` | addresses, or a domain written `@example.com`, that may sign in. Optional when groups are set. With no groups it is the whole gate, and everybody it admits is a runner |
| `GUSTO_ADMINS` | addresses that are admins whatever their groups say |

The server refuses to start with a provider and neither a group nor an allowed list. Otherwise anybody with an account at the provider could spend the server's model credential.

A person who signs in but matches no group returns to the login page with a refusal. The refusal goes in the change record ([CHANGES.md](CHANGES.md)).

On Compose, set these in `.env` ([deploy/compose/env.example](../deploy/compose/env.example)). On Kubernetes, set `auth.oidc` in the chart's values.

## What is checked on each release

`install-check` starts [Dex](https://dexidp.io/) beside each install, with its mock connector. The connector signs in one fixed person in the group `authors`. The check:

1. Sets `GUSTO_ADMIN_GROUPS=authors`, signs in, and reads an admin page.
2. On Compose, sets both group lists to a group that person is not in, signs in again, and expects the refusal.

No company's own provider has been tried. Test yours before handover.
