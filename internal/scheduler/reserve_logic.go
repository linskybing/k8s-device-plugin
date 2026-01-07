package scheduler

import (
	"context"
	"fmt"

	"k8s.io/klog/v2"
)

// ReserveLogic performs the core reservation flow used by the scheduler plugin.
// It is intentionally placed in a non-build-tag file so unit tests can exercise
// the logic by injecting dependencies.
//
// Parameters:
// - podKey: "ns/name" identifier for the pod
// - req: GPURequest describing NumCards and PercentPerCard
// - nodeName: target node
// - pickDevicesFn: function that returns candidate device IDs on the node
// - reserveFn: function that issues the node-local /reserve call (e.g. ReserveForPod)
// Returns the selected devices on success or an error.
func ReserveLogic(ctx context.Context, podKey string, req GPURequest, nodeName string,
	pickDevicesFn func(nodeName string, numCards, percent int) ([]string, error),
	reserveFn func(ctx context.Context, nodeName, podKey string, devices []string, percent int) error,
) ([]string, error) {
	// Reserve in cluster manager first
	if err := capacityMgr.Reserve(podKey, nodeName, int(req.NumCards), int(req.PercentPerCard)); err != nil {
		klog.InfoS("ReserveLogic: capacityMgr.Reserve failed", "pod", podKey, "node", nodeName, "err", err)
		return nil, fmt.Errorf("capacity manager rejected reservation: %w", err)
	}

	// pick devices from node-local status
	devices, err := pickDevicesFn(nodeName, int(req.NumCards), int(req.PercentPerCard))
	if err != nil {
		klog.InfoS("ReserveLogic: pickDevicesFn failed, rolling back capacity reservation", "pod", podKey, "node", nodeName, "err", err)
		_ = capacityMgr.Release(podKey, nodeName)
		return nil, err
	}

	// call node-local reserve
	if err := reserveFn(ctx, nodeName, podKey, devices, int(req.PercentPerCard)); err != nil {
		klog.InfoS("ReserveLogic: reserveFn failed, rolling back capacity reservation", "pod", podKey, "node", nodeName, "err", err)
		_ = capacityMgr.Release(podKey, nodeName)
		return nil, err
	}

	return devices, nil
}
