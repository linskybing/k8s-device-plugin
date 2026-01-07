# 將 GPUMPS 調度器 Plugin 整合進 kube-scheduler

這份指南說明如何將 `GPUMPSPlugin` 編譯並包含到 `kube-scheduler`，以及如何部署 `kube-scheduler` 以便讓它能夠存取各 node 的 device-plugin `/status` unix socket。

前置條件
- 具有 Kubernetes 控制平面修改權限（能更新 `kube-system` 中的 `kube-scheduler` Deployment/Pod）。
- 構建環境：Go 工具鏈與 kube 源碼相容版本。

步驟概覽
1. 在 kube-scheduler 原始碼中加入 plugin 套件引用與註冊。
2. 編譯 `kube-scheduler` binary（包含 plugin）。
3. 部署修改版 `kube-scheduler`（或替換現有 Deployment），並將 hostPath `/var/lib/kubelet/device-plugins` 掛載至 scheduler Pod。
4. 啟用 plugin（若使用 Dynamic Kube-scheduler 的 plugin config，需在 ComponentConfig 中啟用）。

詳情步驟

1) 在 kube-scheduler code 中加入 plugin

- 找到 `k8s.io/kubernetes/plugins` 或 `k8s.io/kubernetes/cmd/kube-scheduler/app` 中的 plugin 註冊位置（取決於 Kubernetes 版本）。
- 在註冊的列表中加入一個 entry，讓 scheduler 能夠建立 `GPUMPSPlugin`。例如：

```go
import (
    gpumps "github.com/NVIDIA/k8s-device-plugin/internal/scheduler"
)

// 在 factory 或插件建構時候加入
factory.Register("GPUMPSPlugin", func(args ...interface{}) (framework.Plugin, error) {
    return gpumps.New(ctx, &framework.PluginFactoryArgs{Handle: handle})
})
```

注意：實際整合方式依 Kubernetes 版本與插件框架不同（有些版本使用 registry/registry.go 或 PluginFactory）。請依你所用的 kube-scheduler 原始碼位置修改。

2) 編譯 kube-scheduler

在 kube 源碼根目錄執行：

```bash
# 在 Kubernetes 源碼目錄
make WHAT=cmd/kube-scheduler
# 或直接 go build 指令（依你的 GOPATH 與模組路徑）
go build -o _output/kube-scheduler ./cmd/kube-scheduler
```

將產物上傳/替換到控制平面節點上。

3) 修改 kube-scheduler PodSpec（Deployment/StaticPod）以掛載 hostPath

- 若你的集群使用 static pod（如 kubeadm），編輯 `/etc/kubernetes/manifests/kube-scheduler.yaml`，在 `spec.containers[0].volumeMounts` 加入：

```yaml
volumeMounts:
  - name: device-plugin-sock
    mountPath: /var/lib/kubelet/device-plugins
    readOnly: true
volumes:
  - name: device-plugin-sock
    hostPath:
      path: /var/lib/kubelet/device-plugins
      type: Directory
```

若使用 Deployment（在 `kube-system` 命名空間），編輯該 Deployment 相同地加入 `volumes`/`volumeMounts`。

4) 啟用 plugin（Scheduler Configuration）

- 若你的 kube-scheduler 使用 ComponentConfig（`--config` 指定），需在 `KubeSchedulerConfiguration` 中把 `GPUMPSPlugin` 加入 `profiles.plugins` 中的 `score` 或 `filter` 階段。如下為範例片段：

```yaml
profiles:
  - plugins:
      score:
        enabled:
          - name: GPUMPSPlugin
```

5) 驗證

- 確認 scheduler 日誌有載入 `GPUMPSPlugin`。
- 在 scheduler Pod 內 `curl --unix-socket /var/lib/kubelet/device-plugins/nvidia-gpu.sock.status http://unix/status` 應該能夠取得節點 socket（如果權限/路徑允許）。
- 創建一個帶 MPS 註解的 Pod，觀察 scheduler 是否以分數選擇節點並呼叫 Reserve 流。

附註
- 如果不希望修改 kube-scheduler binary，可考慮下列替代：
  - 使用外部調度器（scheduler extender）或使用 Scheduling Framework 的外掛機制（視 Kubernetes 版本而定）。
  - 建立一個獨立的「預選服務」來計算分數並標註 Node，供 scheduler Filter/Score 使用。

如果你要，我可以：
- 幫你草擬 PR patch（在 kube 源碼中加入 plugin 的必要 imports 與註冊程式碼），或
- 幫你準備一份修改後的 `kube-scheduler` static pod manifest 範例。
