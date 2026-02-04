# NVIDIA MPS Setup Guide

## Overview

This guide explains how to configure NVIDIA GPU MPS (Multi-Process Service) in Kubernetes with proper memory allocation per replica.

## Problem & Solution

**Problem**: Each GPU has 31.8GB memory. When using 20 replicas, each replica should get 31.8GB ÷ 20 = 1.59GB, but system may report lower due to improper configuration.

**Solution**: Use either:
- **Aggregate mode** (default): All 4 GPUs share 20 replicas total. Resource name: `nvidia.com/gpu`
- **Individual mode**: Each GPU has 20 independent replicas. Resource names: `nvidia.com/gpu0`, `gpu1`, `gpu2`, `gpu3`

## Quick Start

### Install (Aggregate Mode - Default)

```bash
cd /home/user/k8s-gpu-platform/k8s-device-plugin

# Install on all nodes
./scripts/setup.sh 20 aggregate all docker.io/linskybing/k8s-device-plugin:latest

# Install on specific node
./scripts/setup.sh 20 aggregate gpu1 docker.io/linskybing/k8s-device-plugin:latest
```

### Install (Individual GPU Mode)

```bash
# Each GPU gets 20 independent replicas (total: 80 tokens)
./scripts/setup.sh 20 individual all docker.io/linskybing/k8s-device-plugin:latest
```

### Verify Installation

```bash
# Check available resources
kubectl describe node gpu1 | grep nvidia.com

# Expected output (aggregate mode):
#   nvidia.com/gpu: 80

# Expected output (individual mode):
#   nvidia.com/gpu0: 20
#   nvidia.com/gpu1: 20
#   nvidia.com/gpu2: 20
#   nvidia.com/gpu3: 20
```

### Check MPS Configuration

```bash
# View MPS daemon logs
kubectl logs -n nvidia-device-plugin -l app.kubernetes.io/name=nvidia-device-plugin \
  -c mps-control-daemon-ctr | grep "Set device pinned"

# Expected output:
# Set device pinned memory limit: device=0, total_memory_gb=31.84, replicas=20, limit_per_replica_mib=1630
```

## Pod Configuration

### Aggregate Mode (Shared Pool)

Request N tokens from the shared pool:

```yaml
resources:
  limits:
    nvidia.com/gpu: "10"  # Use 10 tokens from pool of 80
```

Memory available: 10 × 1.59GB = 15.9GB

### Individual GPU Mode (Per-GPU)

Request tokens from specific GPU:

```yaml
resources:
  limits:
    nvidia.com/gpu0: "5"   # Use 5 tokens from GPU 0
    nvidia.com/gpu1: "5"   # Use 5 tokens from GPU 1
```

Memory available: 5 × 1.59GB = 7.95GB per GPU

## Configuration Modes

| Aspect | Aggregate | Individual |
|--------|-----------|------------|
| Default | Yes | No |
| Resource Name | `nvidia.com/gpu` | `gpu0`, `gpu1`, `gpu2`, `gpu3` |
| Total Replicas | 20 (all GPUs share) | 80 (4 GPUs x 20 each) |
| Memory Per Replica | 1.59GB | 1.59GB |
| Use Case | Simple multi-GPU setup | GPU isolation needed |

## Memory Calculation

Formula: `Memory per replica = GPU total memory / number of replicas`

Example with 20 replicas:
- GPU memory: 31.8GB
- Replicas: 20
- Per replica: 31.8GB ÷ 20 = 1.59GB

For different replica counts:
- 2 replicas: 15.9GB each
- 5 replicas: 6.36GB each
- 10 replicas: 3.18GB each
- 20 replicas: 1.59GB each

## Usage Examples

### Example 1: Deploy on All Nodes (Aggregate)

```bash
./scripts/setup.sh 20 aggregate all docker.io/linskybing/k8s-device-plugin:latest
```

Creates: 4 GPUs × 20 replicas = 80 total tokens in shared pool

### Example 2: Deploy with Low Latency (Individual)

```bash
./scripts/setup.sh 2 individual all docker.io/linskybing/k8s-device-plugin:latest
```

Creates: 4 GPUs × 2 replicas = 8 total tokens (15.9GB per replica)

### Example 3: High Throughput (Individual)

```bash
./scripts/setup.sh 30 individual all docker.io/linskybing/k8s-device-plugin:latest
```

Creates: 4 GPUs × 30 replicas = 120 total tokens (1.06GB per replica)

## Troubleshooting

### Resources Not Visible

Check pod logs:
```bash
kubectl logs -n nvidia-device-plugin -l app.kubernetes.io/name=nvidia-device-plugin \
  -c mps-control-daemon-ctr | tail -50
```

Look for error messages about configuration parsing.

### Pod Stuck in Pending

1. Verify node has resources:
```bash
kubectl describe node gpu1 | grep nvidia.com
```

2. Check if all tokens are already allocated:
```bash
kubectl describe node gpu1 | grep "Allocated resources" -A 20
```

3. Restart device plugin:
```bash
kubectl rollout restart ds/nvidia-device-plugin-mps-control-daemon -n nvidia-device-plugin
```

### Memory Limit Too Low

This is normal - each replica gets: Total GPU Memory ÷ Replicas

To increase memory per replica, reduce replica count or combine multiple replicas.

## CLI Reference

### Setup Script Usage

```bash
./scripts/setup.sh <replicas> [mode] <node|all> <image-repo> [image-tag] [secret-name]
```

Parameters:
- `<replicas>`: Number of replicas per GPU (default: 20)
- `[mode]`: `aggregate` (default) or `individual`
- `<node|all>`: Target node name or 'all'
- `<image-repo>`: Docker image repository
- `[image-tag]`: Docker image tag (default: latest)
- `[secret-name]`: Image pull secret (default: harbor-regcred)

Examples:
```bash
# Aggregate, 20 replicas, all nodes
./scripts/setup.sh 20 aggregate all docker.io/linskybing/k8s-device-plugin:latest

# Individual, 2 replicas, GPU node 1
./scripts/setup.sh 2 individual gpu1 docker.io/linskybing/k8s-device-plugin:latest

# Aggregate, 5 replicas, all nodes
./scripts/setup.sh 5 aggregate all docker.io/linskybing/k8s-device-plugin:latest harbor-regcred
```

## Verification Script

Run automatic verification:

```bash
chmod +x scripts/verify-mps-setup.sh
./scripts/verify-mps-setup.sh gpu1
```

This script checks:
- Node resources
- MPS daemon pods status
- MPS configuration in logs
- Memory limit per replica

## Advanced Configuration

### Custom Replica Count

Modify replica count before running setup:

```bash
./scripts/setup.sh 5 aggregate all ...  # 5 replicas instead of default 20
```

### Multi-Node Setup

```bash
# Deploy to all nodes with MPS capability
./scripts/setup.sh 20 aggregate all docker.io/linskybing/k8s-device-plugin:latest
```

### Using Environment Variables

```bash
export PLUGIN_IMAGE_REPO=docker.io/linskybing/k8s-device-plugin
export PLUGIN_IMAGE_TAG=latest
export IMAGE_PULL_SECRET_NAME=harbor-regcred

./scripts/setup.sh 20 aggregate gpu1
```

## Files Modified

- `scripts/setup.sh` - Installation script
- `scripts/verify-mps-setup.sh` - Verification script
- `deployments/helm/nvidia-device-plugin/` - Helm chart (auto-configured)

## Common Issues

### "no resources specified" error

ConfigMap may have invalid YAML. Verify:
```bash
kubectl get configmap -n nvidia-device-plugin nvidia-device-plugin-configs -o yaml
```

### MPS daemon not starting

Check driver version:
```bash
nvidia-smi --query-gpu=driver_version --format=csv,noheader
```

Must be v550+ for proper MPS support.

### Memory still showing as low

1. Verify correct configuration was deployed:
```bash
kubectl logs -n nvidia-device-plugin <pod> -c mps-control-daemon-ctr | grep "Running with config" -A 30
```

2. Check if individual GPU limits are set correctly:
```bash
kubectl logs -n nvidia-device-plugin <pod> -c mps-control-daemon-ctr | grep "Set device pinned"
```

## Performance Tips

1. Use aggregate mode for simple setups
2. Use individual mode for GPU isolation
3. Start with 20 replicas, adjust based on actual workload
4. Monitor memory usage with `nvidia-smi` in containers
5. Use multiple replicas to share single GPU effectively

## Next Steps

1. Run `./scripts/setup.sh` with appropriate parameters
2. Run `./scripts/verify-mps-setup.sh` to verify
3. Deploy test Pod with resource request
4. Check Pod logs with `kubectl logs <pod-name>`
5. Verify GPU access with `nvidia-smi` in Pod
