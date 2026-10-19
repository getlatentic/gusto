# The harness, run in a sandbox: it makes the change, asks the model through the
# gusto proxy, and then tries a host nobody allowed.
printf yes > answer.txt
exec 3<>/dev/tcp/host.openshell.internal/8041 || { echo "the proxy was not reachable"; exit 1; }
body='{"model":"fake","messages":[{"role":"user","content":"ping"}]}'
printf 'POST /model/v1/chat/completions HTTP/1.0\r\nHost: host.openshell.internal:8041\r\nAuthorization: Bearer %s\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s' \
  "$GUSTO_MODEL_TOKEN" "${#body}" "$body" >&3
answer=$(cat <&3 | tail -n 1)
echo "the model said: $answer"
if timeout 5 bash -c 'exec 4<>/dev/tcp/example.com/443' 2>/dev/null; then echo "example.com: REACHED"; else echo "example.com: refused"; fi
echo "the harness ran as: $(id -un)"
