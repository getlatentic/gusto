#!/usr/bin/env bash
# The harnesses the runner image carries, as name=package, from the deployment's own list.
python3 -c '
import re,sys
text=open(sys.argv[1]).read()
block=text.split("runner_harnesses:")[1].split("\n\n")[0]
print(" ".join(re.findall(r"^\s+- (\S+=\S+)\s*$",block,re.M)))' "$(git rev-parse --show-toplevel)/deploy/ansible/group_vars/all.yml"
