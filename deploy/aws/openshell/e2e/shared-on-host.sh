# Runs on the instance. Part A: two harnesses, one after the other, in one sandbox on one
# project. Part B: the app they wrote is started in that sandbox and shared two ways, with
# `openshell service expose` and with `openshell forward`. The sandbox and the forward are
# left running so the app can be looked at; e2e-concurrency.sh removes them.
set -uo pipefail
S=$HOME/e2e/shared
export PATH=/opt/openshell/bin:/usr/sbin:/sbin:$PATH
export XDG_CONFIG_HOME=/etc/openshell-xdg XDG_STATE_HOME=/var/lib/openshell-xdg
export OPENSHELL_LOCAL_TLS_DIR=$XDG_STATE_HOME/openshell/tls
G="openshell --gateway spike"
for old in $($G sandbox list 2>/dev/null | awk 'NR>1 && $1 ~ /^haas-/ {print $1}'); do $G sandbox delete "$old" >/dev/null 2>&1; done
pkill -f "[o]penshell.*forward" 2>/dev/null || true
export GUSTO_HARNESSES=""
bash "$S/credentials.sh" "$S/creds"

echo "== A: two harnesses in one sandbox, on one project"
"$S/shared" -credentials "$S/creds" -project "$S/sample" -gateway spike -image "$HOME/e2e/runner-all-rootfs.tar.gz"
NAME=$($G sandbox list | awk 'NR>1 && $1 ~ /^haas-/ {print $1}' | head -1)
[ -n "$NAME" ] || { echo "no sandbox was left"; exit 1; }
# exec reads standard input, which here is the rest of this script, so it is given none.
inside() { $G sandbox exec --name "$NAME" --workdir /sandbox/workspace --no-login-shell --no-tty -- "$@" </dev/null; }
echo "-- the project after both turns"; inside sh -c 'ls -la; echo; node --test 2>&1 | tail -8'

echo; echo "== B: the app the agents wrote, started in the sandbox and shared"
inside sh -c 'PORT=8080 setsid nohup node server.js > /sandbox/app.log 2>&1 < /dev/null & sleep 2; echo started'
echo "-- asked from inside the sandbox, in a second exec (so the app outlived the first)"
inside node -e "fetch('http://127.0.0.1:8080/').then(r=>r.text()).then(t=>console.log(t.slice(0,400))).catch(e=>console.log('failed:',e.cause&&e.cause.code||e.message))"
inside node -e "fetch('http://127.0.0.1:8080/health').then(r=>r.text()).then(t=>console.log('health:',t)).catch(e=>console.log('failed:',e.cause&&e.cause.code||e.message))"
echo "-- openshell service expose"
$G service expose "$NAME" 8080 app 2>&1 | head -12; $G service list 2>&1 | head -8
echo "-- asked from the host, at the URL the gateway gave, with nothing but the host name"
curl -s -m 8 -H "Host: default--$NAME--app.openshell.localhost:18080" http://127.0.0.1:18080/ | head -12; echo
curl -s -m 8 -H "Host: default--$NAME--app.openshell.localhost:18080" http://127.0.0.1:18080/health -w "\nHTTP %{http_code}\n"
echo "-- openshell forward (its background process is given no pipe, or this script would wait on it)"
$G forward start -d 8080 "$NAME" > /tmp/forward.out 2>&1 < /dev/null; head -3 /tmp/forward.out; sleep 2
echo "-- asked from the host, on its own port 8080"
curl -s -m 5 http://127.0.0.1:8080/health -w "\nHTTP %{http_code}\n"
echo "-- what the app logged"; inside sh -c 'tail -5 /sandbox/app.log'
echo; echo "the sandbox $NAME is left running, with its app forwarded to 127.0.0.1:8080 on this host"
