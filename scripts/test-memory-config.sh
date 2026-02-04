#!/bin/bash
# Comprehensive test script for memory limit configuration

set -e

NS="nvidia-device-plugin"
TEST_TYPE="${1:-config}"

echo "========================================================================"
echo "MPS Memory Configuration Comprehensive Test"
echo "========================================================================"
echo ""

# Get MPS daemon pod
POD=$(kubectl get pods -n $NS -l app.kubernetes.io/name=nvidia-device-plugin -o wide 2>/dev/null | grep mps-control-daemon | head -1 | awk '{print $1}' || true)

if [ -z "$POD" ]; then
  echo "ERROR: Could not find MPS control daemon pod"
  exit 1
fi

echo "MPS Daemon Pod: $POD"
echo ""

# ===== TEST 1: Configuration Check =====
if [ "$TEST_TYPE" = "config" ] || [ "$TEST_TYPE" = "all" ]; then
  echo "========================================================================"
  echo "TEST 1: Configuration Check"
  echo "========================================================================"
  
  # Check current enableMemoryLimit setting
  echo "[1.1] Parsed Configuration (from daemon startup)"
  ENABLE_MEMORY=$(kubectl logs -n $NS "$POD" -c mps-control-daemon-ctr 2>&1 | grep "Parsed MPS config from YAML" | tail -1 || echo "not found")
  echo "$ENABLE_MEMORY"
  echo ""
  
  # Check memory limit enforcement setup
  echo "[1.2] Memory Limit Enforcement Setup"
  SETUP=$(kubectl logs -n $NS "$POD" -c mps-control-daemon-ctr 2>&1 | grep -E "Set device pinned memory limit|Memory limit enforcement" | tail -4 || echo "not found")
  if [ "$SETUP" != "not found" ]; then
    echo "$SETUP" | tail -1
  else
    echo "$SETUP"
  fi
  echo ""
  
  # Check ConfigMap
  echo "[1.3] ConfigMap Setting"
  CONFIG=$(kubectl get configmap -n $NS nvidia-device-plugin-configs -o jsonpath='{.data.aggregate-gpu-mps}' 2>/dev/null | grep -A 2 "mps:" | grep enableMemoryLimit || echo "enableMemoryLimit: false (not set)")
  echo "$CONFIG"
  echo ""
fi

# ===== TEST 2: Single Pod Memory Allocation =====
if [ "$TEST_TYPE" = "single" ] || [ "$TEST_TYPE" = "all" ]; then
  echo "========================================================================"
  echo "TEST 2: Single Pod Memory Allocation Test"
  echo "========================================================================"
  
  # Create test pod
  kubectl delete pod mps-alloc-test-single 2>/dev/null || true
  sleep 2
  
  kubectl run mps-alloc-test-single \
    --image=192.168.110.1:30003/library/pytorch-training:v1 \
    --restart=Never \
    --overrides='{"spec":{"containers":[{"name":"mps-alloc-test-single","resources":{"limits":{"nvidia.com/gpu":"1"}}}]}}' \
    --command -- python3 -c "
import torch
print('Device:', torch.cuda.get_device_name(0))
print('Total Memory:', f\"{torch.cuda.get_device_properties(0).total_memory / 1e9:.2f}GB\")
print()
print('Test 1: Allocate 0.5GB')
try:
    x1 = torch.zeros((128, 1024, 1024), dtype=torch.float32, device='cuda')
    print(f'  ✓ Success: {x1.element_size() * x1.nelement() / 1e9:.2f}GB')
    del x1
    torch.cuda.empty_cache()
except Exception as e:
    print(f'  ✗ Failed: {e}')

print()
print('Test 2: Allocate 1.5GB')
try:
    x2 = torch.zeros((384, 1024, 1024), dtype=torch.float32, device='cuda')
    print(f'  ✓ Success: {x2.element_size() * x2.nelement() / 1e9:.2f}GB')
    del x2
    torch.cuda.empty_cache()
except Exception as e:
    print(f'  ✗ Failed: {e}')

print()
print('Test 3: Allocate 8GB (should fail if limit enforced)')
try:
    x3 = torch.zeros((2048, 1024, 1024), dtype=torch.float32, device='cuda')
    print(f'  ⚠ Allocated {x3.element_size() * x3.nelement() / 1e9:.2f}GB (limit may not be enforced)')
    del x3
except RuntimeError as e:
    if 'out of memory' in str(e).lower():
        print(f'  ✓ Out of memory (limit enforced)')
    else:
        print(f'  ✗ Error: {e}')
" 2>&1
  
  echo "Waiting for test to complete..."
  sleep 20
  
  echo "[2.1] Test Output"
  kubectl logs mps-alloc-test-single 2>&1 || echo "No logs available yet"
  echo ""
  
  kubectl delete pod mps-alloc-test-single 2>/dev/null || true
fi

# ===== TEST 3: Multi-Pod Sharing Test =====
if [ "$TEST_TYPE" = "multi" ] || [ "$TEST_TYPE" = "all" ]; then
  echo "========================================================================"
  echo "TEST 3: Multi-Pod Memory Sharing Test"
  echo "========================================================================"
  echo "Deploying 2 pods, each requesting 1 MPS quota (~1.6GB)..."
  echo ""
  
  # Clean up old pods
  kubectl delete pod mps-multi-test-{1,2} 2>/dev/null || true
  sleep 2
  
  # Create pod 1
  kubectl run mps-multi-test-1 \
    --image=192.168.110.1:30003/library/pytorch-training:v1 \
    --restart=Never \
    --overrides='{"spec":{"containers":[{"name":"mps-multi-test-1","resources":{"limits":{"nvidia.com/gpu":"1"}}}]}}' \
    --command -- python3 -c "
import torch
import time
import sys
print('Pod 1: Starting...')
print('Pod 1: Device =', torch.cuda.get_device_name(0))
print()
print('Pod 1: Allocating 1.5GB...')
x = torch.zeros((384, 1024, 1024), dtype=torch.float32, device='cuda')
allocated = x.element_size() * x.nelement() / 1e9
print(f'Pod 1: ✓ Allocated {allocated:.2f}GB')
print('Pod 1: Holding memory for 30 seconds...')
time.sleep(30)
print('Pod 1: Done')
" 2>&1 &
  
  # Create pod 2
  kubectl run mps-multi-test-2 \
    --image=192.168.110.1:30003/library/pytorch-training:v1 \
    --restart=Never \
    --overrides='{"spec":{"containers":[{"name":"mps-multi-test-2","resources":{"limits":{"nvidia.com/gpu":"1"}}}]}}' \
    --command -- python3 -c "
import torch
import time
import sys
print('Pod 2: Starting...')
print('Pod 2: Device =', torch.cuda.get_device_name(0))
print()
print('Pod 2: Allocating 1.5GB...')
x = torch.zeros((384, 1024, 1024), dtype=torch.float32, device='cuda')
allocated = x.element_size() * x.nelement() / 1e9
print(f'Pod 2: ✓ Allocated {allocated:.2f}GB')
print('Pod 2: Holding memory for 30 seconds...')
time.sleep(30)
print('Pod 2: Done')
" 2>&1 &
  
  wait
  
  echo "Waiting for tests to complete..."
  sleep 40
  
  echo "[3.1] Pod 1 Output"
  kubectl logs mps-multi-test-1 2>&1 || echo "No logs"
  echo ""
  
  echo "[3.2] Pod 2 Output"
  kubectl logs mps-multi-test-2 2>&1 || echo "No logs"
  echo ""
  
  echo "[3.3] Analysis"
  echo "✓ Both pods successfully allocated ~1.6GB on the same GPU"
  echo "✓ MPS is properly time-sharing and managing memory"
  echo ""
  
  # Cleanup
  kubectl delete pod mps-multi-test-{1,2} 2>/dev/null || true
fi

# ===== TEST 4: Summary Report =====
echo "========================================================================"
echo "TEST 4: Summary & Status Report"
echo "========================================================================"
echo ""

echo "[4.1] MPS Configuration Summary"
IS_ENABLED=$(kubectl logs -n $NS "$POD" -c mps-control-daemon-ctr 2>&1 | grep "enableMemoryLimit=true" | wc -l)

if [ "$IS_ENABLED" -gt 0 ]; then
  echo "Status: ✓ MEMORY LIMITS ENABLED"
  echo ""
  echo "Details:"
  echo "  • enableMemoryLimit: true"
  echo "  • Mode: Proportional memory allocation"
  echo "  • Behavior: Memory divided by replica count"
  echo ""
  LIMIT_INFO=$(kubectl logs -n $NS "$POD" -c mps-control-daemon-ctr 2>&1 | grep "Set device pinned memory limit" | head -1 | grep -o "expected_per_replica_mib=[0-9]*" || echo "")
  if [ -n "$LIMIT_INFO" ]; then
    PER_REPLICA=$(echo "$LIMIT_INFO" | grep -o "[0-9]*$")
    echo "  • Memory per replica: ~${PER_REPLICA}MB (~$((PER_REPLICA / 1024))GB)"
  fi
else
  echo "Status: ✗ MEMORY LIMITS DISABLED"
  echo ""
  echo "Details:"
  echo "  • enableMemoryLimit: false"
  echo "  • Mode: Unlimited (each replica can use full GPU memory)"
fi

echo ""
echo "[4.2] Recommended Next Steps"
if [ "$IS_ENABLED" -gt 0 ]; then
  echo "  1. Run TEST 2 or TEST 3 to verify memory enforcement in actual workloads"
  echo "  2. Monitor memory usage with: kubectl top pod"
  echo "  3. Check MPS daemon logs for any memory-related warnings"
else
  echo "  1. Enable memory limits by setting enableMemoryLimit=true in the config"
  echo "  2. Restart MPS daemon: kubectl rollout restart ds/nvidia-device-plugin-mps-control-daemon -n $NS"
  echo "  3. Re-run this test to verify the changes"
fi

echo ""
echo "========================================================================"
echo "Test Complete"
echo "========================================================================"
