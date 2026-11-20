# gusto cloud on a shared host

Production gusto cloud runs on one host off AWS, shared with other services. That host runs gusto cloud, every organization's
`gusto serve`, and their agents in rootless podman containers as the `gusto` user. It is `ROLE=all` of the cloud role in
`deploy/aws/openshell/ansible`. A client demo is a separate, short-lived host on AWS (the **cloud up** workflow, with its own
`/gusto/demo-config`) and does not talk to this one.

- The host builds gusto itself. Every five minutes `gusto-upgrade` fetches `main` from GitHub. When `main` has moved, it builds
  the commit with `bundle.sh` as the `gusto-build` user, at low priority and with a memory cap. When the runner image's inputs
  changed, it builds the image with podman as `gusto` and removes the old one. Then it runs the commit's playbook. No CI job
  delivers a release, and nothing outside the host holds a key to it.
- A failed build leaves the running release as it was, and mails `cloud_demo_notify` the end of the log.
- Secrets are files under `/etc/gusto/params`, for root alone, at their Parameter Store names. `gusto param` reads them there
  (`GUSTO_PARAM_DIR`). The certificate stays in `/etc/letsencrypt`.
- The host's other sites, its default site and its firewall are left alone. gusto's site answers its own names only. gusto cloud
  listens on 127.0.0.1:8050, the operator's page on 127.0.0.1:8092, and the organizations' servers from 9100.

## Files

```
site.yml                       makes the host: the users, Go, the secrets, the settings, the upgrade and its timer
gusto-upgrade                  builds and installs the newest commit of main, or the one named
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

2. Make the host ready, with the name still pointing at AWS. The first build takes about 20 minutes, most of it the runner image:

   ```bash
   ansible-playbook -i greencloud, -u root deploy/greencloud/site.yml -e @deploy/greencloud/greencloud.vault.yml --vault-password-file ~/.config/lpcg/vault_pass.txt
   ```

3. Move the data. This stops gusto cloud on AWS:

   ```bash
   AWS_PROFILE=gusto-poc FRONT_DOOR=<instance id> HOST=root@<host address> BUCKET=gusto-store-083804847775 deploy/greencloud/move-data.sh
   ```

4. Point the name at the host: run step 2 again with `-e gusto_point_name=true`. gusto-boot points `NAME` and `APPS_NAME`.
5. Update `/opt/INVENTORY.md` on the host with the ports above and the `gusto` and `gusto-build` users.

To go back, start gusto cloud on the AWS front door again and point the name at it.

To install a commit now, or one that is not the newest: `systemctl start gusto-upgrade`, or `gusto-upgrade <commit>` as root.
