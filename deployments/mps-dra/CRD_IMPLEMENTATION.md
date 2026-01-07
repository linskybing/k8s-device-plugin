```markdown
**CRD-backed CapacityManager — Implementation Plan**

目的
- 使用 Kubernetes CRD 作為 cluster-wide reservation store，讓 scheduler 的
  Reserve/Release 能跨 node 一致地管理百分比型 GPU 容量。

需求摘要
- 原子性：Reserve() 必須在 cluster 端保留容量並在後續失敗時可回滾。
- 可觀察性：Reservation CR 擁有 Status，反映狀態（Pending/Reserved/Failed/Released）。
- 衝突處理：多個 scheduler 實例或重試不應導致超額分配。
- 擴充性：後續可加入 TTL、優先順序、ownerRefs 等欄位。

API 設計（簡略）
- Group: `mps.nvidia.com`
- Kind: `Reservation` (namespaced)
- Spec:
  - `podKey` (string) — "ns/name" for the pod requesting reservation
  - `nodeName` (string) — target node
  - `numCards` (int)
  - `percentPerCard` (int)
  - (可選) `owner` / `priority`
- Status:
  - `phase` (Pending|Reserved|Failed|Released)
  - `message` (string)
  - `lastUpdateTime` (timestamp)

Controller Responsibilities
1. Reconcile loop: watch Reservation CRs and node device-plugin status (if needed).
2. Reserve semantics:
   - Scheduler calls CapacityManager.Reserve(podKey,node,numCards,percent) -> controller
     creates a Reservation CR in `Pending` state with `.spec` as requested.
   - Controller attempts to atomically mark Reservation as `Reserved`. Atomicity options:
     - Optimistic update via `Patch` on Reservation plus a cluster-wide Allocation CR per node, or
     - Use a per-node ReservationList CR and optimistic compare-and-swap (resourceVersion) to
       record aggregate reserved capacity.
3. Release semantics:
   - Scheduler calls CapacityManager.Release(podKey,node) -> controller updates Reservation.status.phase=Released
   - Controller reconciles freeing capacity and deletes Reservation CR (or retains with status)
4. Conflict handling:
   - Use optimistic concurrency with retries on `409` (resourceVersion mismatch).
   - Keep per-node aggregate in CR or in Reservation status to validate availability when marking Reserved.
   - If cannot satisfy reservation due to concurrent reservations, set Reservation.status.phase=Failed with message.
5. Garbage collection & finalizers:
   - Add finalizer to Reservation to ensure release logic runs before deletion.
   - Optionally TTL/cleanup for orphaned Reservations.

Atomic approaches (trade-offs)
- Single Reservation CR per pod (simple): Controller must coordinate aggregate capacity checks; risk of race if many concurrent writes.
- Per-node Aggregate CR (recommended): maintain a `NodeReservation` CR per node with aggregate reserved capacity; controller updates NodeReservation via CAS (resourceVersion) to allocate atomically.
  - Reserve flow: create Reservation CR -> CAS-update NodeReservation (reserve counts) -> if success, set Reservation.Status=Reserved; on CAS conflict retry.

RBAC & Deployment
- Controller needs verbs: get/list/watch/create/update/patch/delete on `reservations.mps.nvidia.com` and (if using NodeReservation) on `nodereservations.mps.nvidia.com`.
- Deploy as a Deployment with leader-election enabled to avoid multiple controllers racing.

Observability & Metrics
- Expose metrics: `reservations_total`, `reservations_failed_total`, `reservations_active`.
- Emit events on Reservation objects for human troubleshooting.

Testing & Validation
- Unit tests for controller reconcile logic using envtest or controller-runtime fake client.
- Integration tests: run controller against kind cluster, create concurrent Reserve requests and validate no overcommit.

Migration & Compatibility
- The in-memory `InMemoryCapacityManager` remains default until controller is enabled via configuration.
- Provide a migration/feature gate to switch CapacityManager implementation to `CRDCapacityManager`.

Implementation milestones
1. Create CRD manifest (done) and lightweight Go types (done).
2. Implement `CRDCapacityManager` client that performs create/patch/status operations (toy implementation returning errors currently).
3. Implement controller with per-node `NodeReservation` CAS-based aggregation.
4. Add RBAC manifests, deployment, and leader-election.
5. Add e2e tests simulating concurrent reservations and releases.

Appendix: example `kubectl` steps
```sh
# apply CRD
kubectl apply -f deployments/mps-dra/reservation-crd.yaml

# create an example reservation
kubectl apply -f - <<EOF
apiVersion: mps.nvidia.com/v1
kind: Reservation
metadata:
  name: example-reservation
  namespace: default
spec:
  podKey: default/my-pod
  nodeName: node-1
  numCards: 2
  percentPerCard: 50
EOF

# check status
kubectl get reservation example-reservation -n default -o yaml
```

``` 
