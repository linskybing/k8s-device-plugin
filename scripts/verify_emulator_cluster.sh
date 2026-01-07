#!/bin/bash
# Verify emulator status on multiple hosts
# Usage: SSH_PASS=29166887 ./verify_emulator_cluster.sh host1 host2 ...

set -euo pipefail
if [ $# -lt 1 ]; then
  echo "Usage: $0 <host1> <host2> ..."
  exit 1
fi

SSH_PASS=${SSH_PASS:-}
REMOTE_SOCK=${REMOTE_SOCK:-/tmp/nvidia-gpu.sock.status}

for host in "$@"; do
  echo "---> Checking $host"
  if [ -n "$SSH_PASS" ] && command -v sshpass >/dev/null 2>&1; then
    sshpass -p "$SSH_PASS" ssh -o StrictHostKeyChecking=no "$host" \
      "curl --unix-socket $REMOTE_SOCK http://unix/status" 2>/dev/null || echo "$host: socket error"
  else
    ssh -o StrictHostKeyChecking=no "$host" "curl --unix-socket $REMOTE_SOCK http://unix/status" 2>/dev/null || echo "$host: socket error"
  fi
  echo "---"
done

echo "Verification complete."