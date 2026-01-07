#!/bin/bash
# Deploy gpumps-emulator binary to hosts and start emulator
# Usage: SSH_PASS=29166887 EMULATOR_DEVICES=GPU-0,GPU-1,GPU-2,GPU-3 ./deploy_emulator_cluster.sh host1 host2 ...
# If SSH_PASS is not set, script will attempt passwordless SSH/scp.

set -euo pipefail
if [ $# -lt 1 ]; then
  echo "Usage: $0 <host1> <host2> ..."
  exit 1
fi

SSH_PASS=${SSH_PASS:-}
EMU_DEVS=${EMULATOR_DEVICES:-GPU-0,GPU-1}
BINARY=${BINARY:-/tmp/gpumps-emulator}
REMOTE_SOCK=${REMOTE_SOCK:-/tmp/nvidia-gpu.sock.status}

for host in "$@"; do
  echo "---> Deploying to $host"
  if [ -n "$SSH_PASS" ] && command -v sshpass >/dev/null 2>&1; then
    sshpass -p "$SSH_PASS" scp "$BINARY" "$host":/tmp/ || { echo "scp to $host failed"; continue; }
    sshpass -p "$SSH_PASS" ssh -o StrictHostKeyChecking=no "$host" \
      "EMULATOR_DEVICES=$EMU_DEVS nohup /tmp/$(basename $BINARY) -sock $REMOTE_SOCK > /tmp/emulator.log 2>&1 &" || echo "start on $host failed"
  else
    scp "$BINARY" "$host":/tmp/ || { echo "scp to $host failed"; continue; }
    ssh -o StrictHostKeyChecking=no "$host" \
      "EMULATOR_DEVICES=$EMU_DEVS nohup /tmp/$(basename $BINARY) -sock $REMOTE_SOCK > /tmp/emulator.log 2>&1 &" || echo "start on $host failed"
  fi
  echo "Deployed and started on $host"
done

echo "All done. Use verify_emulator_cluster.sh to check status."