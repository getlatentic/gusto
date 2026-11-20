# gusto cloud on a shared host

One host off AWS, shared with other services, runs two gusto clouds:

| | Production (`gusto`) | Staging (`gusto-staging`) |
| --- | --- | --- |
| Follows | the newest release tag, `vX.Y.Z` | `main` |
| Name | production's (`gusto_cloud_config`) | its own (`gusto_staging_config`) |
| User | `gusto` | `gusto-staging` |
| gusto cloud, servers, operator's page | 127.0.0.1:8050, from 9100, 127.0.0.1:8092 | 127.0.0.1:8051, from 9600, 127.0.0.1:8093 |
| Memory for an agent | 2 GiB | 1 GiB |

Each is `ROLE=all` of the cloud role in `deploy/aws/openshell/ansible` with its own `cloud_name`: gusto cloud, every
organization's `gusto serve`, and their agents in rootless podman containers as its own user. Each has its own units, settings
(`/etc/<name>`), secrets, programs, data and site. A client demo is a separate, short-lived host on AWS (the **cloud up**
workflow, with its own `/gusto/demo-config`) and does not talk to either.

- The host builds gusto itself. Every five minutes `gusto-upgrade@<name>` fetches the repository. When what the cloud follows has
  moved, it builds the commit with `bundle.sh` as `gusto-build`, at low priority and with a memory cap, one build at a time.
  When the runner image's inputs changed, it builds the image with podman as the cloud's user and removes the old one. Then it
  runs the commit's playbook. No CI job delivers a release, and nothing outside the host holds a key to it.
- To release: tag `main` (`git tag v0.1.0 && git push origin v0.1.0`). Production installs it within five minutes. A tag with a
  suffix, such as `v0.1.0-rc.3`, is not a release.
- A failed build leaves the running release as it was, and mails `cloud_demo_notify` the end of the log.
- Secrets are files under `/etc/<name>/params`, for root alone, at their Parameter Store names. `gusto param` reads them there
  (`GUSTO_PARAM_DIR`). Certificates stay in `/etc/letsencrypt`.
- The host's other sites, its default site and its firewall are left alone. Each site answers its own names only.
- With a tunnel (`gusto_tunnel_token`), cloudflared runs as `gusto-tunnel` and Cloudflare sends both clouds' names through it.
  nginx serves them on 127.0.0.1:8180 (production) and 127.0.0.1:8181 (staging), and takes the visitor's address from
  `CF-Connecting-IP`. The names are then routed in Cloudflare, not pointed at the host, and gusto needs none of the host's own
  web ports.

## The tunnel

1. In Cloudflare Zero Trust, make a tunnel of the cloudflared kind and copy its token. `make-vault.sh` asks for it.
2. Give it the public hostnames, each to `http://127.0.0.1:<port>`:

   | Hostname | Service |
   | --- | --- |
   | `gusto-staging.getlatentic.com` | `http://127.0.0.1:8181` |
   | `gusto.getlatentic.com` | `http://127.0.0.1:8180` |
   | `*.apps.getlatentic.com` | `http://127.0.0.1:8180` |

   Add staging first. Production's name has an A record to AWS: Cloudflare replaces it when its hostname is added, and that
   is the moment production moves.
3. Cloudflare's free certificate covers `getlatentic.com` and one level under it. `*.apps.getlatentic.com` is two levels
   under, and needs Advanced Certificate Manager, or shared apps at a name one level deep.

## Files

```
site.yml                       makes the host: the build user, Go, the upgrade, and each cloud (cloud.yml)
cloud.yml                      one cloud: its user, secrets, settings, timer, and its first build
gusto-upgrade                  builds and installs a cloud's newest commit, or the one named
gusto-upgrade-failed           mails the operator when an upgrade fails
make-vault.sh                  makes greencloud.vault.yml from Parameter Store, encrypted, without printing a secret
move-data.sh                   moves production's data from the front door on AWS, once
vault.example.yml              what the vault holds
```

## Once

1. Make the vault. It reads production's parameters with your AWS profile:

   ```bash
   AWS_PROFILE=gusto-poc VAULT_PASSWORD_FILE=~/.config/lpcg/vault_pass.txt PUBLIC_IP=<host address> deploy/greencloud/make-vault.sh
   ```

2. Make the host ready, with production's name still pointing at AWS. Staging gets its own name at once. Production installs
   nothing until there is a release tag. The first build of each takes about 20 minutes, most of it the runner image:

   ```bash
   ansible-playbook -i greencloud, -u root deploy/greencloud/site.yml -e @deploy/greencloud/greencloud.vault.yml --vault-password-file ~/.config/lpcg/vault_pass.txt
   ```

3. Move the data. This stops gusto cloud on AWS:

   ```bash
   AWS_PROFILE=gusto-poc FRONT_DOOR=<instance id> HOST=root@<host address> BUCKET=gusto-store-083804847775 deploy/greencloud/move-data.sh
   ```

4. Point the name at the host: run step 2 again with `-e gusto_point_name=true`. gusto-boot points `NAME` and `APPS_NAME`.
5. Update `/opt/INVENTORY.md` on the host with the ports above and the `gusto`, `gusto-staging` and `gusto-build` users.

To go back, start gusto cloud on the AWS front door again and point the name at it.

To install now: `systemctl start gusto-upgrade@gusto` (or `@gusto-staging`). To install a commit that is not the newest, as
root: `gusto-upgrade gusto <commit>`.
