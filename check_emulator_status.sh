#!/bin/bash
# 多節點 emulator 狀態驗證腳本
# 用法: ./check_emulator_status.sh <host1> <host2> ...
# 須確保每台主機 emulator 已啟動，且 /tmp/nvidia-gpu.sock.status 可被 ssh 連線存取

if [ $# -lt 1 ]; then
  echo "Usage: $0 <host1> <host2> ..."
  exit 1
fi

for host in "$@"; do
  echo "Checking $host ..."
  ssh "$host" 'curl --unix-socket /tmp/nvidia-gpu.sock.status http://unix/status' 2>/dev/null || echo "$host: connection or socket error"
  echo "---"
done
