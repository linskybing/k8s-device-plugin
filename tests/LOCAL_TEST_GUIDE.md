Local verification & simulation guide
=================================

目的
----
提供一組快速且可在開發機上執行的步驟，用於驗證本專案新增的本地/排程器邏輯，而不需存取真實 Kubernetes 叢集或 GPU 節點。

可執行項目（建議順序）
---------------------
1. 單元測試（最快且涵蓋大部分邏輯）：

   ```sh
   # 在專案根目錄
   go test ./internal/scheduler -v
   go test ./internal/plugin -v
   ```

2. 全域測試（針對 repository 的測試套件）：

   ```sh
   go test ./... -run TestReserveLogic -v
   ```

3. 本地模擬 node-local socket 的測試

   - 許多排程器/Reserve helper 測試已使用臨時 unix domain socket 的測試伺服器來模擬 node `/status` 與 `/reserve` 行為；可直接執行相關測試：

     ```sh
     go test ./internal/scheduler -run ReserveForPod -v
     ```

4. 手動執行 node-local server（快速偵錯）

   - 在開發機啟動 `internal/plugin` 的 server（需 root 權限寫入到預設 socket 路徑）：

     ```sh
     go run ./cmd/nvidia-device-plugin --status-socket /tmp/nvidia-gpu.sock.status
     # 然後用 curl (over unix socket) 或 repo 中的 client 來測試 /status /reserve /unreserve
     ```

5. 以 `KUBECONFIG` 驗證 CRD client（非 InCluster）

   - `CRDCapacityManager` 在本地會回傳未設定（因為使用 `InClusterConfig`），若要測試和 API server 的互動，可在一個真實叢集或 `kind` 上執行，或改寫暫時使用 `rest.InClusterConfig` 的替代（非建議）。優先使用 `kind` 來做 CRD 驗證（見 `deployments/mps-dra/KIND_E2E_GUIDE.md`）。

進一步自動化（可選）
-------------------
- 建議建立 Makefile target 來快速執行本地驗證：

  ```make
  test-local:
      go test ./internal/scheduler -v
      go test ./internal/plugin -v
  ```

常見問題
--------
- 測試失敗表示哪一層出問題？
  - unit tests fail → fix logic in package
  - integration-style tests that use unix sockets → verify no socket path collisions and that tests have permissions to create sockets

結語
----
若你想，我可以把其中一個本地驗證步驟包成 script（例如 `scripts/run-local-tests.sh`）以便同事快速執行。
