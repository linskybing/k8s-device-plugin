package scheduler

import (
	"context"
	"fmt"

	"k8s.io/klog/v2"
)

// ReserveHook is a minimal wrapper intended for the scheduler plugin Reserve()
// hook to call. It accepts pod namespace/name and the devices+percent to reserve.
// This file is an integration example and does not import the Kubernetes
// scheduler framework directly to keep changes minimal.
func ReserveHook(ctx context.Context, nodeName, podNamespace, podName string, devices []string, percent int) error {
	podKey := fmt.Sprintf("%s/%s", podNamespace, podName)
	klog.InfoS("ReserveHook: reserving devices for pod", "pod", podKey, "node", nodeName, "devices", devices, "percent", percent)
	return ReserveForPod(ctx, nodeName, podKey, devices, percent)
}

// UnreserveHook releases a previous reservation for the pod.
func UnreserveHook(ctx context.Context, nodeName, podNamespace, podName string) error {
	podKey := fmt.Sprintf("%s/%s", podNamespace, podName)
	klog.InfoS("UnreserveHook: releasing reservation for pod", "pod", podKey, "node", nodeName)
	return UnreserveForPod(ctx, nodeName, podKey)
}
