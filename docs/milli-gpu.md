# Milli-GPU Support

Milli-GPU support allows users to request a fraction of a physical GPU. This is implemented through the "Replicated Resources" mechanism.

## Replicated Resources

A physical GPU can be split into multiple logical units. For example, if you want to support "milli-GPU" with a granularity of 100m (0.1 GPU), you can configure the physical GPU to have 10 replicas.

### Configuration

In the device plugin configuration, you define how many logical replicas each physical device should represent.

```yaml
version: v1
sharing:
  timeSlicing:
    resources:
    - name: nvidia.com/gpu
      replicas: 10
```

With this configuration, a pod requesting `nvidia.com/gpu: 1` is actually getting 1/10th of a physical GPU.

## Custom Resource Names

To avoid confusion with standard physical GPUs, you can rename the replicated resources.

```yaml
version: v1
sharing:
  timeSlicing:
    resources:
    - name: nvidia.com/gpu
      rename: nvidia.com/gpu.shared
      replicas: 10
```

Now, pods can request `nvidia.com/gpu.shared: 2` to get 20% of a GPU.

## Example Pod Specification

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: gpu-share-pod
spec:
  containers:
  - name: cuda-container
    image: nvidia/cuda:12.0-base
    resources:
      limits:
        nvidia.com/gpu.shared: 1 # Requests 10% of a GPU
```
