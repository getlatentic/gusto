# gusto cloud on a shared host

Production gusto cloud runs on one host off AWS, shared with other services. That host runs gusto cloud, every organization's
`gusto serve`, and their agents in rootless podman containers as the `gusto` user. It is `ROLE=all` of the cloud role in
`deploy/aws/openshell/ansible`. A client demo is a separate, short-lived host on AWS (the **cloud up** workflow) and does not
talk to this one.

- Secrets are files under `/etc/gusto/params`, for root alone, at their Parameter Store names. `gusto param` reads them there
  (`GUSTO_PARAM_DIR`).
- Releases, the runner image and the front door's media are in an R2 bucket. CI copies each release there
  (`publish.sh`), and the host pulls the newest every five minutes (`gusto-upgrade`, a systemd timer). CI holds no key to the host.
- The host's other sites, its default site and its firewall are left alone. gusto's site answers its own names only. gusto cloud
  listens on 127.0.0.1:8050, the operator's page on 127.0.0.1:8092, and the organizations' servers from 9100.

## Files

```
site.yml            makes the host: the gusto user, its subordinate ids, the secrets, the store's key, the upgrade timer
gusto-upgrade       brings the host to the newest release in the store, or the one named
publish.sh          CI: copies a release, its runner image and the media from the bucket on AWS to the store
make-vault.sh       makes greencloud.vault.yml from Parameter Store, encrypted, without printing a secret
move-data.sh        moves production's data from the front door on AWS, once
vault.example.yml   what the vault holds
```

## Once

1. Make an R2 bucket (`gusto-store`) and an R2 API token that may read and write it.
2. Set the repository variables `GUSTO_STORE_ENDPOINT` and `GUSTO_STORE_BUCKET`, and the secrets `GUSTO_STORE_KEY_ID` and
   `GUSTO_STORE_SECRET`. The next merge to main copies its release to the store.
3. Make the vault. It reads production's parameters with your AWS profile and asks for the R2 key:

   ```bash
   AWS_PROFILE=gusto-poc VAULT_PASSWORD_FILE=~/.config/lpcg/vault_pass.txt PUBLIC_IP=<host address> STORE_ENDPOINT=https://<account id>.r2.cloudflarestorage.com deploy/greencloud/make-vault.sh
   ```

4. Make the host ready, with the name still pointing at AWS:

   ```bash
   ansible-playbook -i greencloud, -u root deploy/greencloud/site.yml -e @deploy/greencloud/greencloud.vault.yml --vault-password-file ~/.config/lpcg/vault_pass.txt
   ```

5. Move the data. This stops gusto cloud on AWS:

   ```bash
   AWS_PROFILE=gusto-poc FRONT_DOOR=<instance id> HOST=root@<host address> BUCKET=gusto-store-083804847775 deploy/greencloud/move-data.sh
   ```

6. Point the name at the host: run step 4 again with `-e gusto_point_name=true`. gusto-boot points `NAME` and `APPS_NAME`.
7. Update `/opt/INVENTORY.md` on the host with the ports above and the `gusto` user.

To go back, start gusto cloud on the AWS front door again and point the name at it.
