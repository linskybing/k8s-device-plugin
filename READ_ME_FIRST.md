# MPS Memory Limit Configuration

Configurable GPU MPS memory allocation for Kubernetes.

## Quick Start

```bash
cd /home/user/k8s-gpu-platform/k8s-device-plugin
./scripts/setup.sh 20 aggregate gpu1 docker.io/linskybing/k8s-device-plugin:latest
```

## Operating Modes

**Default (Memory Limits Enabled):**
- 20 replicas divide 32GB equally
- Each replica: 32GB / 20 = 1.6GB
- Run: `./setup.sh 20 aggregate gpu1 docker.io/linskybing/k8s-device-plugin:latest`

**Unlimited Memory (Optional):**
- Each replica gets full GPU memory
- Run: `./setup.sh 20 aggregate gpu1 docker.io/linskybing/k8s-device-plugin:latest harbor-regcred false`

## Verify Configuration

```bash
./scripts/test-memory-config.sh
kubectl logs -n nvidia-device-plugin -l app.kubernetes.io/name=nvidia-device-plugin \
  -c mps-control-daemon-ctr | grep "Memory limit"
```

## Usage Example

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: gpu-job
spec:
  containers:
  - name: worker
    image: nvidia/cuda:12.2.0-runtime
    resources:
      limits:
        nvidia.com/gpu: 5
  nodeSelector:
    nvidia.com/mps.capable: "true"
```

## Setup Parameters

`./setup.sh <replicas> [mode] <node-name> <image-repo> [image-tag] [secret] [memory-limit]`

- `replicas`: Per-GPU tokens (default: 20)
- `mode`: aggregate (default) or individual
- `node-name`: Target node or 'all'
- `image-repo`: Container registry
- `image-tag`: Version (default: latest)
- `secret`: Pull secret (default: harbor-regcred)
- `memory-limit`: true (default) or false

