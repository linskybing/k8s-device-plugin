# Changes for MPS Milli-GPU and Per-GPU Resource Mapping

This feature adds support for Milli-GPU requests in Kubernetes using NVIDIA MPS, allows for explicit physical GPU mapping, and optimizes resource utilization through a Bin Packing allocation strategy.

## Key Features

### 1. Milli-GPU Support (e.g., `700m`)
- Relaxed the restriction on requesting multiple units of a replicated resource when using the MPS sharing strategy.
- Implemented dynamic calculation of `CUDA_MPS_ACTIVE_THREAD_PERCENTAGE` based on the number of requested units relative to the total replicas.
- Allows requests like `nvidia.com/gpu: 1500` to be treated as 1.5 GPUs.

### 2. Per-GPU Resource Naming (`NamedResources`)
- Added a new flag `--named-resources` (env: `NAMED_RESOURCES`).
- When enabled, the plugin exposes individual GPUs as `nvidia.com/gpu-0`, `nvidia.com/gpu-1`, etc., instead of the generic `nvidia.com/gpu`.
- Ensured mutual exclusivity between generic and named resources to prevent over-subscription.
- Updated the device discovery logic to match resources based on GPU index.

### 3. Bin Packing Allocation Strategy
- Replaced the default load-balancing (distributed) allocation with a **Bin Packing** strategy for replicated resources.
- The plugin now prefers to allocate replicas from GPUs that are already partially occupied.
- This minimizes fragmentation and preserves complete GPUs for large resource requests.

### 4. Configurable Strict VRAM Management
- Added a new flag `--mps-memory-limiting` (env: `MPS_MEMORY_LIMITING`).
- When enabled, the plugin dynamicallly calculates and injects the `CUDA_MPS_PINNED_DEVICE_MEM_LIMIT` environment variable.
- This ensures a container is strictly limited to a proportion of the VRAM corresponding to its request (e.g., a `500m` request on a 16GB card is limited to 8GB).
- Disabled the default pinned memory limit in the MPS control daemon to allow for this per-container dynamic limiting.

## Technical Improvements
- **Code Optimization**: Consolidated loops in the MPS allocation response logic for better performance.
- **Resource Naming**: Enabled custom resource naming by removing the hardcoded disablement in the main plugin entry point.
- **Flexibility**: Updated `api/config/v1/config.go` to allow `FailRequestsGreaterThanOne` to be configurable for MPS.

## Affected Files
- `api/config/v1/config.go`
- `api/config/v1/flags.go`
- `cmd/mps-control-daemon/mps/daemon.go`
- `cmd/nvidia-device-plugin/main.go`
- `internal/plugin/mps.go`
- `internal/plugin/server.go`
- `internal/rm/allocate.go`
- `internal/rm/device_map.go`
- `internal/rm/rm.go`
