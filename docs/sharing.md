# GPU Sharing Strategies

The K8s Device Plugin supports two primary strategies for sharing a single physical GPU among multiple containers: NVIDIA MPS and Time-Slicing.

## NVIDIA MPS (Multi-Process Service)

MPS allows multiple CUDA processes to be processed concurrently on the same GPU. This is particularly useful for small workloads that do not fully utilize the GPU's compute power.

### How it Works

1. The `mps-control-daemon` starts an MPS control daemon and a server on each node.
2. Containers requesting shared GPU resources are configured to connect to the MPS server.
3. The MPS server handles the scheduling of CUDA kernels from multiple processes onto the GPU hardware.

### Benefits

- **Increased Throughput**: Allows overlapping execution of kernels from different containers.
- **Hardware Isolation**: Provides limited memory and compute isolation between shared clients.

### Example Configuration

```yaml
sharing:
  mps:
    failRequestsGreaterThanCapacity: true
```

## Time-Slicing

Time-Slicing allows multiple containers to share a GPU by context-switching between them. Each container gets a slice of time on the GPU.

### How it Works

The plugin over-advertises the number of available GPUs to Kubernetes. If a node has 1 physical GPU and a replication factor of 10 is configured, Kubernetes will see 10 "logical" GPUs.

### Benefits

- **Simple Setup**: Does not require the MPS daemon.
- **Compatibility**: Works with all NVIDIA GPUs that support CUDA.

### Example Configuration

```yaml
sharing:
  timeSlicing:
    resources:
    - name: nvidia.com/gpu
      replicas: 10
```
