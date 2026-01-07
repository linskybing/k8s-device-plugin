package scheduler

import (
	"context"
	"time"

	"k8s.io/klog/v2"
)

// Configurable retry attempts for reserve/unreserve calls. Can be adjusted in tests.
var ReserveRetryAttempts = 3

// ReserveForPod attempts to reserve percent capacity for a pod on the given
// node and devices. It retries a few times on transient errors.
func ReserveForPod(ctx context.Context, nodeName, podKey string, devices []string, percent int) error {
	var lastErr error
	for i := 0; i < ReserveRetryAttempts; i++ {
		if err := ReserveOnNode(ctx, nodeName, podKey, devices, percent); err != nil {
			lastErr = err
			klog.InfoS("ReserveOnNode attempt failed", "pod", podKey, "node", nodeName, "err", err, "attempt", i+1)
			select {
			case <-time.After(time.Duration(100*(i+1)) * time.Millisecond):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		} else {
			klog.InfoS("ReserveOnNode succeeded", "pod", podKey, "node", nodeName)
			return nil
		}
	}
	klog.ErrorS(lastErr, "ReserveForPod failed after retries", "pod", podKey, "node", nodeName)
	return lastErr
}

// UnreserveForPod attempts to release a previous reservation for podKey.
func UnreserveForPod(ctx context.Context, nodeName, podKey string) error {
	var lastErr error
	for i := 0; i < ReserveRetryAttempts; i++ {
		if err := UnreserveOnNode(ctx, nodeName, podKey); err != nil {
			lastErr = err
			klog.InfoS("UnreserveOnNode attempt failed", "pod", podKey, "node", nodeName, "err", err, "attempt", i+1)
			select {
			case <-time.After(time.Duration(100*(i+1)) * time.Millisecond):
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		} else {
			klog.InfoS("UnreserveOnNode succeeded", "pod", podKey, "node", nodeName)
			return nil
		}
	}
	klog.ErrorS(lastErr, "UnreserveForPod failed after retries", "pod", podKey, "node", nodeName)
	return lastErr
}
