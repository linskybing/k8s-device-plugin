#!/bin/bash
# Comprehensive test script for memory limit configuration

set -e

NS="nvidia-device-plugin"
TEST_MODE="${1:-disabled}"

echo "========================================================================"
echo "Memory Limit Configuration Test: $TEST_MODE"
echo "========================================================================"
echo ""

# Get MPS daemon pod
POD=$(kubectl get pods -n $NS -l app.kubernetes.io/name=nvidia-device-plugin -o wide | grep mps-control-daemon | head -1 | awk '{print $1}' || true)

if [ -z "$POD" ]; then
  echo "ERROR: Could not find MPS control daemon pod"
  exit 1
fi

echo "Using pod: $POD"
echo ""

# Check current enableMemoryLimit setting
echo "[1] Current Configuration"
ENABLE_MEMORY=$(kubectl logs -n $NS "$POD" -c mps-control-daemon-ctr 2>&1 | grep "enableMemoryLimit=" | head -1 || echo "not found")
echo "Setting: $ENABLE_MEMORY"
echo ""

# Check memory limit enforcement status
echo "[2] Memory Limit Enforcement Status"
STATUS=$(kubectl logs -n $NS "$POD" -c mps-control-daemon-ctr 2>&1 | grep -E "Memory limit enforcement|Set device pinned memory limit" | head -1 || echo "not found")
if [ -z "$STATUS" ] || [ "$STATUS" = "not found" ]; then
  STATUS=$(kubectl logs -n $NS "$POD" -c mps-control-daemon-ctr 2>&1 | grep "proportional\|disabled" | head -1 || echo "Status not found in logs")
fi
echo "Status: $STATUS"
echo ""

# Check device configuration
echo "[3] Device Memory Configuration"
DEVICES=$(kubectl logs -n $NS "$POD" -c mps-control-daemon-ctr 2>&1 | grep "device=\"0\"" | head -3 || echo "not found")
if [ "$DEVICES" != "not found" ]; then
  echo "$DEVICES" | while read -r line; do
    DEVICE=$(echo "$line" | sed 's/.*device="\([0-9]\)".*/\1/')
    TOTAL=$(echo "$line" | sed 's/.*total_memory_gb=\([0-9.]*\).*/\1/' || echo "?")
    REPLICAS=$(echo "$line" | sed 's/.*replicas=\([0-9]*\).*/\1/' || echo "?")
    LIMIT=$(echo "$line" | sed 's/.*limit_per_device_mib=\([0-9]*\).*/\1/' || echo "0")
    
    if [ "$LIMIT" = "0" ] || [ -z "$LIMIT" ]; then
      echo "  Device $DEVICE: ${TOTAL}GB, Replicas=$REPLICAS, Limit=NONE (unlimited)"
    else
      PER_REPLICA=$((LIMIT / REPLICAS))
      echo "  Device $DEVICE: ${TOTAL}GB, Replicas=$REPLICAS, Limit=$LIMIT MiB (~${PER_REPLICA} MiB per replica)"
    fi
  done
else
  echo "$DEVICES"
fi
echo ""

# Determine expected behavior
echo "[4] Expected Behavior"
if echo "$ENABLE_MEMORY" | grep -q "false" || echo "$STATUS" | grep -q "disabled"; then
  echo "Mode: UNLIMITED (enableMemoryLimit=false)"
  echo "Expected: Each replica can access full GPU memory"
  echo "Example: 1 replica = full 32GB, 10 replicas = full 32GB each"
  echo ""
  echo "Use case: Maximum performance, no sharing constraints"
else
  echo "Mode: PROPORTIONAL (enableMemoryLimit=true)"
  echo "Expected: Memory divided by replica count"
  echo "Example: 20 replicas = 1.6GB per replica, 10 replicas = 3.2GB per replica"
  echo ""
  echo "Use case: Fair memory sharing among replicas"
fi
echo ""

# Configuration file check
echo "[5] ConfigMap Status"
CONFIG=$(kubectl get configmap -n $NS nvidia-device-plugin-configs -o jsonpath='{.data.aggregate-gpu-mps}' 2>/dev/null | grep -A 2 "mps:" | grep enableMemoryLimit || echo "not found")
if [ -z "$CONFIG" ] || [ "$CONFIG" = "not found" ]; then
  CONFIG="enableMemoryLimit: false (default/implicit)"
fi
echo "ConfigMap setting: $CONFIG"
echo ""

echo "========================================================================"
echo "Test Complete"
echo "========================================================================"
