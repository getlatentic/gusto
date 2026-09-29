#!/usr/bin/env bash
# The plan in docs/matrix/box.json on this backend: thirteen harness and model pairs, each
# started in an OpenShell sandbox made from an image that carries all six harnesses, through
# haas's proxy to real models. Run ./up.sh first, and ./put-key.sh for /haas/poc/model-key
# (Bedrock) and, with HAAS_MODEL_PARAMETER=/haas/poc/deepseek-key, DeepSeek's.
# The instance reads both keys itself. The cells run one after another on the instance,
# detached, and this script follows them. AGAIN=all, or AGAIN=harness@model,..., runs
# pairs that already have a verdict from an earlier run of this script again. Output is kept in out/e2e-matrix.txt.
source "$(dirname "$0")/env.sh"
address="$(terraform output -raw address)"
root="$(git rev-parse --show-toplevel)"
harnesses="$(python3 -c '
import re,sys
text=open(sys.argv[1]).read()
block=text.split("runner_harnesses:")[1].split("\n\n")[0]
print(" ".join(re.findall(r"^\s+- (\S+=\S+)\s*$",block,re.M)))' "$root/deploy/ansible/group_vars/all.yml")"
./image.sh runner-all-rootfs "$harnesses"
(cd "$root" && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o "$PWD/deploy/aws/openshell/$state/haas" ./cmd/haas)
ssh "${ssh_options[@]}" "ubuntu@$address" 'mkdir -p ~/e2e/matrix; pkill -f "[e]2e/haas (serve|matrix)" || true; sleep 1; rm -f ~/e2e/haas'
scp -q "${ssh_options[@]}" "$state/haas" "ubuntu@$address:e2e/"
scp -q -r "${ssh_options[@]}" e2e/sample "ubuntu@$address:e2e/"
scp -q "${ssh_options[@]}" "$root/docs/matrix/box.json" "ubuntu@$address:e2e/matrix/plan.json"
scp -q "${ssh_options[@]}" e2e/matrix-on-host.sh "ubuntu@$address:e2e/"
ssh "${ssh_options[@]}" "ubuntu@$address" "export AWS_REGION=$AWS_REGION AGAIN='${AGAIN:-}' BEDROCK_PARAMETER=${HAAS_MODEL_PARAMETER:-/haas/poc/model-key} DEEPSEEK_PARAMETER=${HAAS_DEEPSEEK_PARAMETER:-/haas/poc/deepseek-key}
  setsid nohup bash ~/e2e/matrix-on-host.sh > ~/e2e/matrix-host.log 2>&1 < /dev/null &"
{
  until ssh "${ssh_options[@]}" "ubuntu@$address" 'test -f ~/e2e/matrix/done'; do
    ssh "${ssh_options[@]}" "ubuntu@$address" 'tail -n 3 ~/e2e/matrix/run.log 2>/dev/null; ls ~/e2e/matrix/plan-matrix/one-line-fix 2>/dev/null | wc -l' | tr '\n' ' '; echo
    sleep 30
  done
  ssh "${ssh_options[@]}" "ubuntu@$address" 'cat ~/e2e/matrix-host.log; echo "== table"; cat ~/e2e/matrix/table.txt; echo "== run.log"; tail -n 30 ~/e2e/matrix/run.log
    for C in ~/e2e/matrix/plan-matrix/one-line-fix/*/; do
      echo "-- $(basename "$C")"
      tr -d "\n " < "$C/state.json" 2>/dev/null | cut -c1-200; echo
      echo "transcript: $(head -c 400 "$C/out/transcript.log" | tr "\n" " ")"
      echo "stderr: $(head -c 400 "$C/out/stderr.log" | tr "\n" " ")"
      echo "gate: $(tr -d "\n " < "$C/out/gates.json" 2>/dev/null)  model calls: $(grep -c . "$C/out/proxy.jsonl" 2>/dev/null)  denied: $(grep -c DENIED "$C/out/sandbox-events.log" 2>/dev/null)"
    done'
} 2>&1 | tee out/e2e-matrix.txt
