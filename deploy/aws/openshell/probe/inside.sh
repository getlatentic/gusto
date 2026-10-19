# Runs inside a sandbox.
echo "guest kernel: $(uname -r)"
echo "dns example.com: $(getent hosts example.com | head -1 || echo none)"
for target in 93.184.216.34:443 1.1.1.1:53 169.254.169.254:80 10.0.0.2:22; do
  host=${target%:*}; port=${target#*:}
  if timeout 4 bash -c "echo > /dev/tcp/$host/$port" 2>/dev/null; then echo "tcp $target: OPEN"; else echo "tcp $target: refused"; fi
done
for file in /etc/gusto-test /root/x /tmp/x; do
  if ( echo x > "$file" ) 2>/dev/null; then echo "write $file: allowed"; else echo "write $file: denied"; fi
done
if cat /etc/shadow >/dev/null 2>&1; then echo "read /etc/shadow: allowed"; else echo "read /etc/shadow: denied"; fi
