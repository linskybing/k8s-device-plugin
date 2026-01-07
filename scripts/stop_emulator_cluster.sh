#!/bin/bash
# Stop gpumps-emulator on multiple hosts
# Usage: SSH_PASS=29166887 ./stop_emulator_cluster.sh host1 host2 ...

set -euo pipefail
if [ $# -lt 1 ]; then
  echo "Usage: $0 <host1> <host2> ..."
  exit 1
fi

SSH_PASS=${SSH_PASS:-}

for host in "$@"; do
  echo "---> Stopping on $host"
  if [ -n "$SSH_PASS" ] && command -v sshpass >/dev/null 2>&1; then
    sshpass -p "$SSH_PASS" ssh -o StrictHostKeyChecking=no "$host" "pkill -f gpumps-emulator || true"
  else
    ssh -o StrictHostKeyChecking=no "$host" "pkill -f gpumps-emulator || true"
  fi
  echo "$host: stopped"
done

echo "Stop complete."