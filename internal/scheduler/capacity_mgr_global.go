package scheduler

import "k8s.io/klog/v2"

// capacityMgr is the package-global manager used by the example plugin and tests.
var capacityMgr CapacityManager = NewInMemoryCapacityManager()

// releaseCapacityReservation is a thin helper that calls CapacityManager.Release.
// Extracted into a non-build-tag file so tests can access and override capacityMgr.
func releaseCapacityReservation(podKey, nodeName string) {
	if err := capacityMgr.Release(podKey, nodeName); err != nil {
		klog.InfoS("releaseCapacityReservation: capacityMgr.Release failed", "pod", podKey, "node", nodeName, "err", err)
	}
}
