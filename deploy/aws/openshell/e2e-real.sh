#!/usr/bin/env bash
# opencode, a real harness, on the sample project in an OpenShell sandbox made
# from the runner image. Run ./up.sh first.
#
# With no model settings it runs against a model that only answers "pong", which
# shows the harness starts in the sandbox, reaches haas's proxy, and is denied
# everything else. With GUSTO_MODEL_UPSTREAM set the same job runs on that model,
# and the instance reads its key from Parameter Store with its own role, where
# ./put-key.sh stored it: the key is never here, never an argument, and lands on
# the instance only in a file its user alone can read. (GUSTO_MODEL_KEY in the
# environment is still accepted, and goes over ssh's standard input.) The
# instance is destroyed by ./down.sh. Output is kept in out/e2e-real.txt.
source "$(dirname "$0")/env.sh"
address="$(terraform output -raw address)"
root="$(git rev-parse --show-toplevel)"
./image.sh runner-rootfs "opencode=opencode-ai pi=@earendil-works/pi-coding-agent"
(cd "$root" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$PWD/deploy/aws/openshell/$state/haas" ./cmd/gusto)
ssh "${ssh_options[@]}" "ubuntu@$address" 'mkdir -p ~/e2e; pkill -f "[e]2e/haas serve" || true; pkill -f "[f]ake_model.py" || true; sleep 1; rm -f ~/e2e/haas ~/e2e/model.env'
scp -q "${ssh_options[@]}" "$state/haas" e2e/fake_model.py "ubuntu@$address:e2e/"
scp -q -r "${ssh_options[@]}" e2e/sample "ubuntu@$address:e2e/"
# The credential's name and the model's id at that provider; neither is secret.
name="${GUSTO_MODEL_NAME:-provider}"; id="${GUSTO_MODEL_ID:-}"
if [ -n "${GUSTO_MODEL_UPSTREAM:-}" ] && [ -z "$id" ]; then echo "set GUSTO_MODEL_ID, the model's id at GUSTO_MODEL_UPSTREAM" >&2; exit 2; fi
if [ -n "${GUSTO_MODEL_UPSTREAM:-}" ] && [ -z "${GUSTO_MODEL_KEY:-}" ]; then
  # The instance reads the key itself, with its role, from where ./put-key.sh stored it.
  parameter="${GUSTO_MODEL_PARAMETER:-/haas/poc/model-key}"
  ssh "${ssh_options[@]}" "ubuntu@$address" "umask 077; export AWS_REGION=$AWS_REGION
    key=\$(aws ssm get-parameter --with-decryption --name '$parameter' --query Parameter.Value --output text) || exit 1
    printf 'GUSTO_MODEL_UPSTREAM=%s\nGUSTO_MODEL_NAME=%s\nGUSTO_MODEL_ID=%s\nGUSTO_MODEL_KEY=%s\n' '$GUSTO_MODEL_UPSTREAM' '$name' '$id' \"\$key\" > ~/e2e/model.env"
elif [ -n "${GUSTO_MODEL_KEY:-}" ]; then
  : "${GUSTO_MODEL_UPSTREAM:?set GUSTO_MODEL_UPSTREAM with GUSTO_MODEL_KEY}"
  printf 'GUSTO_MODEL_UPSTREAM=%s\nGUSTO_MODEL_NAME=%s\nGUSTO_MODEL_ID=%s\nGUSTO_MODEL_KEY=%s\n' "$GUSTO_MODEL_UPSTREAM" "$name" "$id" "$GUSTO_MODEL_KEY" \
    | ssh "${ssh_options[@]}" "ubuntu@$address" 'umask 077; cat > ~/e2e/model.env'
fi
ssh "${ssh_options[@]}" "ubuntu@$address" 'bash -s' < e2e/real-on-host.sh 2>&1 | tee out/e2e-real.txt
