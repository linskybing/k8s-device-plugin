package scheduler

import "testing"

type fakeCapacityMgr struct {
	releasedPod  string
	releasedNode string
}

func (f *fakeCapacityMgr) Reserve(podKey, nodeName string, numCards, percent int) error { return nil }
func (f *fakeCapacityMgr) Release(podKey, nodeName string) error {
	f.releasedPod = podKey
	f.releasedNode = nodeName
	return nil
}

// TestReleaseCapacityReservation verifies the helper releases via CapacityManager.
func TestReleaseCapacityReservation(t *testing.T) {
	old := capacityMgr
	f := &fakeCapacityMgr{}
	capacityMgr = f
	defer func() { capacityMgr = old }()

	releaseCapacityReservation("ns/testpod", "nodeX")

	if f.releasedPod != "ns/testpod" || f.releasedNode != "nodeX" {
		t.Fatalf("expected Release called with (ns/testpod, nodeX), got (%s, %s)", f.releasedPod, f.releasedNode)
	}
}
