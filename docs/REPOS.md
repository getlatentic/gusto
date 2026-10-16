# Private repositories

A job for a private repository gets a token for that one repository. The token
comes from a GitHub App, it is revoked when the work is done, and it never
enters the box the agent runs in.

The only long-lived secret is the App's private key. It sits in the vault, and
in a file that only the server reads.

## Make the App

1. On GitHub, open Settings, Developer settings, GitHub Apps, New GitHub App.
2. Give it these repository permissions and no others:
   - Contents: read and write. Write is for `gusto ship`; a clone needs only read.
   - Pull requests: read and write. `gusto ship` opens one.
   - Metadata: read.
3. Turn the webhook off.
4. Make the App private to your organisation, and install it on the
   repositories jobs may use. Choose selected repositories, not all.
5. Note the App id, and generate a private key. GitHub gives a `.pem` file.

## Give it to gusto

| Setting | Holds |
| --- | --- |
| `GUSTO_GITHUB_APP_ID` | the App id |
| `GUSTO_GITHUB_APP_KEY` | the path of the private key file |
| `GUSTO_GITHUB_URL`, `GUSTO_GITHUB_API` | a GitHub Enterprise Server, such as `https://ghe.example` and `https://ghe.example/api/v3`. github.com when unset |

Set both of the first two or neither. The server refuses to start with one.

With the playbook, put the key in the vault as `github_app_private_key` and set
`github_app_id` in `group_vars`. The playbook writes the key at mode 0600 where
only the server reads it.

## What happens to a job

1. A job names a repository, in any form git takes: `https://github.com/acme/api`,
   `git@github.com:acme/api.git`, or `ssh://git@github.com/acme/api.git`.
2. The server asks GitHub whether the App is installed on `acme/api`. When it is
   not, the job is refused before anything is cloned. The record says
   `the GitHub App is not installed on acme/api` and names what to ask an owner for.
3. The server mints a token for `acme/api` alone, with `contents: read`.
4. git clones over HTTPS. The token goes to git in its environment, for the
   forge's address only, and not in an argument or a file. An address written
   for ssh is cloned over HTTPS, because the token is what the App holds.
5. The token is revoked when git finishes. The repository is left with the plain
   address and no credential in its configuration.
6. The box starts with the checkout. It has no token and no way to reach GitHub.

A repository that is not on the forge, and a local path, is reached as git
reaches it on the machine. A deployment with no App behaves as it always did.

## Opening a pull request

`gusto ship` mints a second token for the push target, with `contents: write` and
`pull_requests: write`, for that repository alone, and revokes it when shipping
is done. Without an App it uses `GUSTO_GITHUB_TOKEN`, as before.

## What was tested

The unit tests run against a fake GitHub API and a real `git http-backend` that
answers only a token the fake minted and has not revoked. They show that the
token names one repository, that a clone uses it, that it is revoked after, and
that neither the job directory nor the checkout's `.git/config` holds it.

The live test runs against github.com with the App `getlatentic-gusto` installed
on one repository. It clones the repository, checks that a repository the App is
not installed on is refused by name, and checks that a revoked token is refused.
It passed five runs in a row. In those runs a revoked token was refused over the
API after 100 to 200 ms and over git after 400 to 700 ms. One earlier
measurement saw git take 5.6 s, so a revoked token can still clone for a few
seconds. Its own expiry is one hour. Run it on your own App:

```bash
GUSTO_TEST_GITHUB_REPO=acme/api GUSTO_TEST_GITHUB_OTHER=acme/not-installed \
  GUSTO_GITHUB_APP_ID=... GUSTO_GITHUB_APP_KEY=app.pem \
  go test -tags githublive -run Live ./internal/repoaccess
```

## GitHub answers "not found" now and then

GitHub's git servers sometimes answer `Repository not found` to a token the API
has just minted and vouches for. In 25 clones on one afternoon, 4 got that answer
and the next try worked. A clone with a token therefore tries up to four times,
waiting 1 s, 2 s and 4 s between tries, and only for that refusal. A clone that
fails for another reason fails at once.

The first clone after installing the App on a repository can hit the same
answer. If it keeps failing after four tries, the message says GitHub refused
the token four times running.
