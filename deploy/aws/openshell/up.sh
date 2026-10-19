#!/usr/bin/env bash
# Makes one instance with nested virtualization, installs OpenShell on it and
# starts its gateway with the VM driver. About $0.21 an hour while it runs;
# down.sh removes everything.
source "$(dirname "$0")/env.sh"

[ -f "$state/key" ] || ssh-keygen -q -t ed25519 -N '' -C gusto-poc-openshell -f "$state/key"
export TF_VAR_public_key="$(cat "$state/key.pub")"

terraform init -input=false >/dev/null
terraform apply -input=false -auto-approve
address="$(terraform output -raw address)"

printf 'waiting for ssh at %s ' "$address"
for _ in $(seq 1 40); do
  if ssh "${ssh_options[@]}" "ubuntu@$address" true 2>/dev/null; then echo; break; fi
  printf '.'; sleep 6
done
ssh "${ssh_options[@]}" "ubuntu@$address" true

printf '[spike]\n%s ansible_user=ubuntu ansible_ssh_private_key_file=%s/key ansible_ssh_common_args="-o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=%s/known_hosts -o LogLevel=ERROR"\n' \
  "$address" "$PWD/$state" "$PWD/$state" > "$state/inventory.ini"
ansible-playbook -i "$state/inventory.ini" ansible/site.yml
echo "up: ubuntu@$address. ./probe.sh runs the test; ./down.sh removes it"
