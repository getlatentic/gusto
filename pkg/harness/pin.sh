#!/usr/bin/env bash
# Writes pins.json: the version and registry integrity of every npm package the runner image installs, so an image is built
# from the same bytes every time and a tampered registry fails the build. Run it to move a pin, then run the tests.
#
#   pkg/harness/pin.sh                      # keep every version, refresh the integrity
#   pkg/harness/pin.sh opencode-ai@1.19.3   # move one package to that version
#   pkg/harness/pin.sh opencode-ai@latest   # move one package to what the registry calls latest
set -euo pipefail
cd "$(dirname "$0")"
python3 - "$@" <<'EOF'
import json, subprocess, sys
pins = json.load(open("pins.json")) if __import__("os").path.exists("pins.json") else {"packages": {}}
packages = pins["packages"]
moves = {}
for arg in sys.argv[1:]:
    name, _, version = arg.rpartition("@")
    if not name or not version:
        sys.exit(f"{arg}: say package@version")
    moves[name] = version
for name in sorted(set(packages) | set(moves)):
    version = moves.get(name) or packages.get(name, {}).get("version") or "latest"
    seen = json.loads(subprocess.check_output(["npm", "view", f"{name}@{version}", "version", "dist.integrity", "dist.tarball", "--json"]))
    if isinstance(seen, list):
        sys.exit(f"{name}@{version} matches several versions: pin one")
    packages[name] = {"version": seen["version"], "integrity": seen["dist.integrity"], "tarball": seen["dist.tarball"]}
    print(f"{name}@{seen['version']}")
json.dump({"_note": "Written by pin.sh. Each package the runner image installs, at one version, with the registry's sha512 integrity of that tarball.", "packages": packages}, open("pins.json", "w"), indent=2)
open("pins.json", "a").write("\n")
EOF
