#!/bin/bash
set -e
# ==============================================================================
# NVIDIA Setup : MPS with Memory Limits Enabled
# Feature: Proportional memory allocation per replica (20 replicas = 32GB total)
# ==============================================================================
REPLICAS=${1:-20}           # per-GPU replicas (quota per card)
MODE=${2:-aggregate}        # 'individual' (per-GPU) or 'aggregate' (all GPUs together)
TARGET_NODE=${3:-$(kubectl get nodes -o name | head -n 1 | cut -d/ -f2)}
IMAGE_ARG=${4:-${PLUGIN_IMAGE_REPO:-docker.io/linskybing/k8s-device-plugin}}
PLUGIN_IMAGE_TAG=${5:-${PLUGIN_IMAGE_TAG:-latest}}
NS=nvidia-device-plugin
# Optional: image pull secret for private registries
IMAGE_PULL_SECRET_NAME=${6:-${IMAGE_PULL_SECRET_NAME:-harbor-regcred}}
# Enable proportional memory allocation (default: true = memory limits enabled)
ENABLE_MEMORY_LIMIT=${7:-${ENABLE_MEMORY_LIMIT:-true}}

# Parse image repo and tag (handle cases like repo:tag or just repo)
if [[ "$IMAGE_ARG" == *":"* ]]; then
  # Image contains :, split it
  PLUGIN_IMAGE_REPO="${IMAGE_ARG%:*}"
  PROVIDED_TAG="${IMAGE_ARG##*:}"
  # Only use the provided tag if no explicit 5th argument was given
  if [ -z "${5}" ]; then
    PLUGIN_IMAGE_TAG="$PROVIDED_TAG"
  fi
else
  PLUGIN_IMAGE_REPO="$IMAGE_ARG"
fi

if [ "$REPLICAS" -lt 1 ]; then
  REPLICAS=1
fi

# Validate args
if [ -z "$TARGET_NODE" ] || [ -z "$PLUGIN_IMAGE_REPO" ]; then
  echo "[ERROR] Usage: ./setup.sh <replicas-per-gpu> [mode] <node-name|all> <image-repo> [image-tag] [image-pull-secret] [enable-memory-limit]"
  echo ""
  echo "Parameters:"
  echo "  <replicas-per-gpu>       : Number of MPS replicas per GPU (default: 20)"
  echo "  [mode]                   : 'individual' (each GPU separate) or 'aggregate' (all GPUs together) (default: aggregate)"
  echo "  <node-name|all>          : Target node name or 'all' for all MPS-capable nodes"
  echo "  <image-repo>             : Docker image repository"
  echo "  [image-tag]              : Docker image tag (default: latest)"
  echo "  [image-pull-secret]      : Image pull secret name (default: harbor-regcred)"
  echo "  [enable-memory-limit]    : 'true' or 'false' - Enable proportional memory allocation (default: false)"
  echo ""
  echo "Examples:"
  echo "  # Default: No memory limits, each replica can use full 32GB"
  echo "  ./setup.sh 20 aggregate gpu1 docker.io/linskybing/k8s-device-plugin latest"
  echo ""
  echo "  # Enable proportional memory: 20 replicas = 32GB, 10 replicas = 16GB, etc."
  echo "  ./setup.sh 20 aggregate gpu1 docker.io/linskybing/k8s-device-plugin latest harbor-regcred true"
  echo ""
  echo "  # Individual mode with memory limits"
  echo "  ./setup.sh 20 individual all docker.io/linskybing/k8s-device-plugin latest harbor-regcred true"
  echo ""
  echo "Memory Limit Modes:"
  echo "  false (default): Each replica can access full GPU memory (32GB)"
  echo "  true:            Memory divided proportionally by replica count"
  echo "                   - Request 20 replicas = 32GB (full GPU)"
  echo "                   - Request 10 replicas = 16GB (half GPU)"
  echo "                   - Request 1 replica = 1.6GB (1/20 of GPU)"
  exit 1
fi

# Validate mode
if [ "$MODE" != "individual" ] && [ "$MODE" != "aggregate" ]; then
  echo "[ERROR] Invalid mode: $MODE. Use 'individual' or 'aggregate'."
  exit 1
fi

# Detect GPU count for per-GPU MPS entries
NUM_GPUS=$(nvidia-smi -L | wc -l)
TOTAL_TOKENS=$((REPLICAS * NUM_GPUS))
echo "========================================================"
echo "Setup: MPS Device Plugin Configuration"
echo "Target Node: $TARGET_NODE"
echo "Plugin Image: $PLUGIN_IMAGE_REPO:$PLUGIN_IMAGE_TAG"
echo "Mode: $MODE (each GPU $REPLICAS replicas, total tokens=$TOTAL_TOKENS)"
echo "GPUs Detected: $NUM_GPUS"
echo "Memory Limit: $ENABLE_MEMORY_LIMIT (proportional allocation)"
echo "========================================================"
command_exists() {
  command -v "$1" >/dev/null 2>&1
}
echo "[Step 1] Checking NVIDIA Drivers..."
if command_exists nvidia-smi; then
  CURRENT_DRIVER=$(nvidia-smi --query-gpu=driver_version --format=csv,noheader | head -n 1)
  echo "Driver detected: v$CURRENT_DRIVER"
else
  echo "No driver detected. Installing recommended server driver..."
  sudo apt-get update
  sudo apt-get install -y ubuntu-drivers-common
  RECOMMENDED_DRIVER=$(ubuntu-drivers devices | grep "recommended" | awk '{print $3}')
  if [ -z "$RECOMMENDED_DRIVER" ]; then
    echo "Error: Could not detect a recommended driver. Install manually."
    exit 1
  fi
  sudo apt-get install -y "$RECOMMENDED_DRIVER"
  echo "DRIVER INSTALLED. REBOOT REQUIRED. Please reboot and re-run."
  exit 0
fi

echo "[Step 2] Configuring Container Runtime..."
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg \
  && curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list | \
  sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
  sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list > /dev/null

sudo apt-get update
sudo apt-get install -y nvidia-container-toolkit
sudo nvidia-ctk runtime configure --runtime=containerd --set-as-default
sudo systemctl restart containerd
echo "[Step 3] Cleaning up old resources (mps/gpu)..."
helm uninstall nvidia-device-plugin -n $NS 2>/dev/null || true
kubectl delete daemonset nvidia-device-plugin nvidia-device-plugin-mps-control-daemon -n $NS 2>/dev/null || true
kubectl delete configmap nvidia-device-plugin-config nvidia-device-plugin-configs -n $NS 2>/dev/null || true
sudo rm -f /var/lib/kubelet/device-plugins/nvidia.sock 2>/dev/null || true
kubectl label node "$TARGET_NODE" nvidia.com/mps.capable- 2>/dev/null || true
kubectl patch node "$TARGET_NODE" --type=json --subresource=status -p='[{"op":"remove","path":"/status/capacity/nvidia.com~1mps-0"},{"op":"remove","path":"/status/capacity/nvidia.com~1mps-1"},{"op":"remove","path":"/status/capacity/nvidia.com~1mps-2"},{"op":"remove","path":"/status/capacity/nvidia.com~1mps-3"},{"op":"remove","path":"/status/allocatable/nvidia.com~1mps-0"},{"op":"remove","path":"/status/allocatable/nvidia.com~1mps-1"},{"op":"remove","path":"/status/allocatable/nvidia.com~1mps-2"},{"op":"remove","path":"/status/allocatable/nvidia.com~1mps-3"}]' 2>/dev/null || true

echo "Waiting for old pods to terminate..."
kubectl delete pod -n $NS -l app.kubernetes.io/name=nvidia-device-plugin --force --grace-period=0 2>/dev/null || true
kubectl wait --for=delete pod -n $NS -l app.kubernetes.io/name=nvidia-device-plugin --timeout=90s 2>/dev/null || true

echo "[Step 3b] Resetting kubelet device-plugin state..."
if command -v systemctl >/dev/null 2>&1; then
  if [ "$TARGET_NODE" = "all" ]; then
    echo "Resetting kubelet on all GPU nodes (requires SSH access or manual restart)..."
    # For multi-node, you may need to SSH to each node or use ansible/pssh
    echo "[WARN] Multi-node kubelet restart not automated. Please restart kubelet on each GPU node manually if needed."
  else
    sudo systemctl stop kubelet || true
    sudo rm -f /var/lib/kubelet/device-plugins/nvidia*.sock 2>/dev/null || true
    sudo rm -f /var/lib/kubelet/device-plugins/kubelet_internal_checkpoint 2>/dev/null || true
    sudo rm -f /var/lib/kubelet/device-plugins/*.json 2>/dev/null || true
    sudo systemctl start kubelet || true
    kubectl wait --for=condition=Ready node/$TARGET_NODE --timeout=120s 2>/dev/null || true
  fi
else
  echo "[WARN] systemctl not found; please manually restart kubelet and clear /var/lib/kubelet/device-plugins if stale resources persist."
fi

echo "[Step 4] Updating node labels for config mode..."
CONFIG_KEY=$([ "$MODE" = "individual" ] && echo "individual-gpu-mps" || echo "aggregate-gpu-mps")

if [ "$TARGET_NODE" = "all" ]; then
  kubectl get nodes -o json | jq -r '.items[] | select(.status.allocatable | has("nvidia.com/gpu") or has("feature.node.kubernetes.io/pci-10de.present")) | .metadata.name' | while read -r node; do
    kubectl label node "$node" nvidia.com/device-plugin.config="$CONFIG_KEY" --overwrite
  done
else
  kubectl label node "$TARGET_NODE" nvidia.com/device-plugin.config="$CONFIG_KEY" --overwrite
fi

echo "[Step 5] Building Helm values (config + image + node)..."
VALUES_FILE=$(mktemp /tmp/nvdp-values-XXXX.yaml)

# Generate GPU configs for individual mode
if [ "$MODE" = "individual" ]; then
  GPU_CONFIGS=""
  for ((i = 0; i < NUM_GPUS; i++)); do
    GPU_CONFIGS+="        - index: $i
          name: gpu$i
          mps:
            enabled: true
            replicas: $REPLICAS
            enableMemoryLimit: $ENABLE_MEMORY_LIMIT
"
  done
else
  GPU_CONFIGS=""
fi

cat <<EOF > "$VALUES_FILE"
image:
  repository: "$PLUGIN_IMAGE_REPO"
  tag: "$PLUGIN_IMAGE_TAG"
  pullPolicy: IfNotPresent

config:
  default: "$CONFIG_KEY"
  fallbackStrategies:
    - named
  map:
    $CONFIG_KEY: |
      version: v1
      flags:
        migStrategy: none
        plugin:
          passDeviceSpecs: true
          deviceListStrategy:
            - envvar
            - cdi-annotations
          deviceIDStrategy: uuid
      resources:
        gpus:
          - pattern: "*"
            name: gpu
      sharing:
        mps:
          renameByDefault: $([ "$MODE" = "individual" ] && echo "true" || echo "false")
          failRequestsGreaterThanOne: $([ "$MODE" = "individual" ] && echo "true" || echo "false")
          enableMemoryLimit: $ENABLE_MEMORY_LIMIT
          resources:
            - name: nvidia.com/gpu
              replicas: $REPLICAS
              devices: "all"
$([ "$MODE" = "individual" ] && echo "      individualGPU:
        enabled: true
        namePattern: \"gpu%d\"
        gpuConfigs:
$GPU_CONFIGS" || echo "")

nvidiaDriverRoot: "/"
affinity: null

gfd:
  enabled: false
nfd:
  enabled: false

mps:
  root: "/run/nvidia/mps"

tolerations:
  - operator: Exists

EOF

if [ "$TARGET_NODE" != "all" ]; then
  cat <<EOF >> "$VALUES_FILE"
nodeSelector:
  kubernetes.io/hostname: "$TARGET_NODE"
EOF
fi

cat <<EOF >> "$VALUES_FILE"
podSecurityContext:
  runAsNonRoot: false

securityContext:
  privileged: true
  capabilities:
    add:
      - SYS_ADMIN
      - SYS_RAWIO
  allowPrivilegeEscalation: true

imagePullSecrets:
  - name: "$IMAGE_PULL_SECRET_NAME"

EOF

echo "[Step 6] Installing Helm Chart..."
if [ "$TARGET_NODE" = "all" ]; then
  echo "Labeling all GPU nodes with nvidia.com/mps.capable=true..."
  kubectl get nodes -o json | jq -r '.items[] | select(.status.allocatable | has("nvidia.com/gpu") or has("feature.node.kubernetes.io/pci-10de.present")) | .metadata.name' | while read -r node; do
    kubectl label node "$node" nvidia.com/mps.capable=true --overwrite
  done
else
  kubectl label node "$TARGET_NODE" nvidia.com/mps.capable=true --overwrite
fi
helm upgrade --install nvidia-device-plugin /home/user/k8s-gpu-platform/k8s-device-plugin/deployments/helm/nvidia-device-plugin \
  --namespace $NS \
  --create-namespace \
  -f "$VALUES_FILE" \
  --wait --timeout 180s

echo "[Step 6b] Waiting for DaemonSets to become Ready..."
kubectl rollout status ds/nvidia-device-plugin -n $NS --timeout=120s || true
kubectl rollout status ds/nvidia-device-plugin-mps-control-daemon -n $NS --timeout=120s || true

echo ""
echo "========================================================"
echo "Setup Complete!"
echo "========================================================"
echo ""
echo "Configuration Summary:"
echo "  Mode: $MODE"
echo "  Replicas per GPU: $REPLICAS"
if [ "$MODE" = "individual" ]; then
  TOTAL_MEM=$(nvidia-smi --query-gpu=memory.total --format=csv,noheader,nounits | head -1)
  PER_REPLICA=$((TOTAL_MEM / REPLICAS))
  echo "  Resource Type: nvidia.com/gpu0, nvidia.com/gpu1, ... (per-GPU)"
  echo "  Each GPU provides $REPLICAS tokens"
  echo "  Memory per replica: ~${PER_REPLICA} MB (~${PER_REPLICA}MB)"
  echo "  Total MPS tokens available: $TOTAL_TOKENS (4 GPUs × $REPLICAS replicas)"
  echo ""
  echo "  Example pod request:"
  echo "    resources:"
  echo "      limits:"
  echo "        nvidia.com/gpu0: 5  # 5 replicas from GPU 0"
  echo "        nvidia.com/gpu1: 5  # 5 replicas from GPU 1"
else
  TOTAL_MEM=$(nvidia-smi --query-gpu=memory.total --format=csv,noheader,nounits | head -1)
  TOTAL_MEM=$((TOTAL_MEM * NUM_GPUS))
  PER_REPLICA=$((TOTAL_MEM / TOTAL_TOKENS))
  echo "  Resource Type: nvidia.com/gpu (aggregated)"
  echo "  All GPUs share the same token pool"
  echo "  Total pool size: $TOTAL_TOKENS tokens"
  echo "  Memory per replica: ~${PER_REPLICA} MB"
  echo ""
  echo "  Example pod request:"
  echo "    resources:"
  echo "      limits:"
  echo "        nvidia.com/gpu: 10  # 10 tokens from the pool"
fi
echo ""
echo "To verify: kubectl describe node $TARGET_NODE | grep nvidia.com"
echo ""
echo "To list all resources:"
echo "  kubectl describe nodes | grep nvidia.com"
echo ""


# cd /home/user/k8s/k8s-device-plugin && docker build -t nvidia/k8s-device-plugin -f deployments/container/Dockerfile .