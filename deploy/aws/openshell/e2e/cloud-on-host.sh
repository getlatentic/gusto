# Runs on the instance. Starts the front door as an operator would, with every
# organization's server given the OpenShell settings, or with SANDBOX=container Docker's, and the model credentials, then
# drives it with e2e/cloud as people would. Nothing is removed at the end, so the pages can be looked at.
set -uo pipefail
C=$HOME/e2e/cloud
export PATH=/opt/openshell/bin:/usr/sbin:/sbin:$PATH
export XDG_CONFIG_HOME=/etc/openshell-xdg XDG_STATE_HOME=/var/lib/openshell-xdg
export OPENSHELL_LOCAL_TLS_DIR=$XDG_STATE_HOME/openshell/tls
G="openshell --gateway spike"

pkill -f "[g]usto cloud" 2>/dev/null; pkill -f "[g]usto serve" 2>/dev/null; sleep 1
for old in $($G sandbox list 2>/dev/null | awk 'NR>1 && ($1 ~ /^gusto-/ || $1 ~ /^c-/) {print $1}'); do $G sandbox delete "$old" >/dev/null 2>&1; done
rm -rf "$C/cloud" "$C/creds"
bash "$C/credentials.sh" "$C/creds"
[ -s "$C/creds/platform.json" ] || { echo "no credentials: is /gusto/poc/model-key stored? (./put-key.sh)"; exit 1; }

MODEL=${MODEL:-platform/qwen.qwen3-vl-235b-a22b-instruct}
SANDBOX=${SANDBOX:-openshell}
if [ "$SANDBOX" = container ]; then
  cat > "$C/org.env" <<ENV
PATH=/usr/local/bin:/usr/sbin:/sbin:/usr/bin:/bin
GUSTO_SANDBOX=container
GUSTO_IMAGE=gusto-runner:e2e
GUSTO_JOB_MEMORY=3g
GUSTO_JOB_CPUS=2
GUSTO_CREDENTIALS=$C/creds
GUSTO_MODELS=$MODEL
ENV
else
  cat > "$C/org.env" <<ENV
PATH=/opt/openshell/bin:/usr/sbin:/sbin:/usr/local/bin:/usr/bin:/bin
XDG_CONFIG_HOME=$XDG_CONFIG_HOME
XDG_STATE_HOME=$XDG_STATE_HOME
OPENSHELL_LOCAL_TLS_DIR=$OPENSHELL_LOCAL_TLS_DIR
GUSTO_SANDBOX=openshell
GUSTO_OPENSHELL_IMAGE=$HOME/e2e/runner-all-rootfs.tar.gz
GUSTO_OPENSHELL_GATEWAY=spike
GUSTO_OPENSHELL_MEMORY=3Gi
GUSTO_OPENSHELL_CPU=2
GUSTO_CREDENTIALS=$C/creds
GUSTO_MODELS=$MODEL
ENV
fi
chmod 600 "$C/org.env"

GUSTO_CLOUD_ROOT=$C/cloud GUSTO_CLOUD_ADDR=127.0.0.1:8050 GUSTO_CLOUD_URL=http://gusto.test:8050 GUSTO_CLOUD_DOMAIN=gusto.test \
  GUSTO_CLOUD_ORG_ENV=$C/org.env GUSTO_CLOUD_PORT_FROM=9100 \
  setsid nohup "$C/gusto" cloud > "$C/front-door.log" 2>&1 < /dev/null &
for _ in $(seq 1 20); do curl -fs http://127.0.0.1:8050/healthz >/dev/null && break; sleep 1; done
curl -fs http://127.0.0.1:8050/healthz >/dev/null || { echo "gusto cloud did not start"; cat "$C/front-door.log"; exit 1; }

# The host's own address stands for the front door: the isolation check tries its ports from inside an agent's container.
FRONT_DOOR=$(hostname -I | awk '{print $1}') "$C/drive" -root "$C/cloud" -gusto "$C/gusto" -gateway spike -model "$MODEL" -within 12m \
  -sandbox "$SANDBOX" -probe "$C/isolation-on-host.sh"
echo "exit $?"
echo; echo "gusto cloud is left running on port 8050; its log is $C/front-door.log"
