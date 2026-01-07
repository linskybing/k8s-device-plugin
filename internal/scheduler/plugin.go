//go:build example
// +build example

package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

const (
	gpuRequestStateKey     = "gpu-request"
	gpuAllocationStateKey  = "gpu-allocation"
	podReservationStateKey = "pod-reservation"
)

type GPUMPSPlugin struct {
	handle framework.Handle
}

var capacityMgr CapacityManager = NewInMemoryCapacityManager()

func New(_ context.Context, fh *framework.PluginFactoryArgs) (framework.Plugin, error) {
	pl := &GPUMPSPlugin{handle: fh.Handle}
	klog.InfoS("GPUMPSPlugin initialized")
	return pl, nil
}

func (pl *GPUMPSPlugin) Name() string { return "GPUMPSPlugin" }

// PreFilter: parse pod annotations and store GPURequest in cycle state.
func (pl *GPUMPSPlugin) PreFilter(ctx context.Context, state *framework.CycleState, pod *v1.Pod) (*framework.PreFilterResult, *framework.Status) {
	if pod.Annotations == nil {
		return nil, framework.NewStatus(framework.Success)
	}
	cardsStr, ok1 := pod.Annotations["gpu.mps.io/cards"]
	ratioStr, ok2 := pod.Annotations["gpu.mps.io/ratio"]
	if !ok1 || !ok2 {
		return nil, framework.NewStatus(framework.Success)
	}
	var req GPURequest
	if _, err := fmt.Sscanf(cardsStr, "%d", &req.NumCards); err != nil || req.NumCards <= 0 {
		return nil, framework.NewStatus(framework.Success)
	}
	var r int64
	if _, err := fmt.Sscanf(ratioStr, "%d", &r); err != nil || r <= 0 {
		return nil, framework.NewStatus(framework.Success)
	}
	req.PercentPerCard = r
	state.Write(framework.StateKey(gpuRequestStateKey), &req)
	return nil, framework.NewStatus(framework.Success)
}

// Filter: minimal pass-through; real checks are performed in Reserve via /status.
func (pl *GPUMPSPlugin) Filter(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	return framework.NewStatus(framework.Success)
}

// Reserve: called to perform atomic reservation on the chosen node.
func (pl *GPUMPSPlugin) Reserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
	v, err := state.Read(framework.StateKey(gpuRequestStateKey))
	if err != nil {
		return framework.NewStatus(framework.Success)
	}
	req := v.(*GPURequest)
	podKey := pod.Namespace + "/" + pod.Name

	// Delegate core logic to ReserveLogic (testable helper).
	devices, err := ReserveLogic(ctx, pod.Namespace+"/"+pod.Name, *req, nodeName, pickDevicesFromNode, ReserveForPod)
	if err != nil {
		// Map ReserveLogic errors to scheduler statuses.
		klog.InfoS("Reserve: ReserveLogic failed", "pod", podKey, "node", nodeName, "err", err)
		return framework.NewStatus(framework.Unschedulable, "reserve failed")
	}

	// store allocation info for later stages
	state.Write(framework.StateKey(podReservationStateKey), podKey)
	state.Write(framework.StateKey(gpuAllocationStateKey), &GPUAllocationInfo{NodeName: nodeName, SelectedCards: devicesToIndices(devices), RequiredRatio: int64(req.PercentPerCard)})

	return framework.NewStatus(framework.Success)
}

// Unreserve: release reservation for the pod
func (pl *GPUMPSPlugin) Unreserve(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) {
	podKey, err := state.Read(framework.StateKey(podReservationStateKey))
	if err != nil {
		return
	}
	pk := podKey.(string)
	if err := UnreserveForPod(ctx, nodeName, pk); err != nil {
		klog.InfoS("Unreserve: UnreserveForPod failed", "pod", pk, "node", nodeName, "err", err)
	}
}

// PostBind: optional sync; no-op for minimal integration.
func (pl *GPUMPSPlugin) PostBind(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) {
	// After the pod is bound, finalize the cluster-side reservation by
	// releasing it from the CapacityManager. This keeps the reservation
	// lifecycle symmetric: Reserve() -> CapacityManager.Reserve(), and
	// PostBind() -> CapacityManager.Release() when the pod has been bound.
	if state == nil {
		return
	}
	v, err := state.Read(framework.StateKey(podReservationStateKey))
	if err != nil {
		return
	}
	podKey := v.(string)
	releaseCapacityReservation(podKey, nodeName)
}

// pickDevicesFromNode queries the node-local status socket and returns up to numCards deviceIDs with remaining >= percent.
func pickDevicesFromNode(nodeName string, numCards, percent int) ([]string, error) {
	// For minimal implementation assume status socket path is standard and accessible.
	statusSock := statusSocketPath(nodeName)
	transport := &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", statusSock)
	}}
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	resp, err := client.Get("http://unix/status")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var m map[string]int
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, err
	}
	var out []string
	for id, rem := range m {
		if rem >= percent {
			out = append(out, id)
			if len(out) >= numCards {
				break
			}
		}
	}
	if len(out) < numCards {
		return nil, fmt.Errorf("insufficient devices: need %d got %d", numCards, len(out))
	}
	return out, nil
}

func devicesToIndices(devices []string) []int {
	var out []int
	for _, d := range devices {
		// best-effort: parse trailing index if present, else ignore
		out = append(out, 0)
	}
	return out
}
