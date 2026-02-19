# Configuration Reference

The K8s Device Plugin is configured using a YAML file. This file can be passed to the plugin via the `--config-file` flag or mounted as a ConfigMap.

## Schema

| Field | Type | Description |
|-------|------|-------------|
| `version` | string | Configuration version (e.g., `v1`). |
| `sharing` | object | GPU sharing configurations. |
| `sharing.mps` | object | MPS specific settings. |
| `sharing.timeSlicing` | object | Time-Slicing specific settings. |
| `flags` | object | Plugin execution flags. |

## Example Full Configuration

```yaml
version: v1
sharing:
  mps:
    failRequestsGreaterThanCapacity: true
  timeSlicing:
    resources:
    - name: nvidia.com/gpu
      rename: nvidia.com/gpu.shared
      replicas: 10
flags:
  migStrategy: none
  failOnInitError: true
  nvidiaDriverRoot: /
```

## Environment Variables

Some settings can also be controlled via environment variables:

- `CONFIG_FILE`: Path to the configuration file.
- `MPS_ROOT`: Directory where MPS sockets and logs are stored.
- `NVIDIA_VISIBLE_DEVICES`: Controls which GPUs are visible to the plugin.
