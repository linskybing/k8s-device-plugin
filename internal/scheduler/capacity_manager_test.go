package scheduler

import "testing"

func TestInMemoryCapacityManager_ReserveRelease(t *testing.T) {
	m := NewInMemoryCapacityManager()

	podKey := "ns/pod1"
	node := "nodeA"

	if err := m.Reserve(podKey, node, 2, 50); err != nil {
		t.Fatalf("Reserve failed: %v", err)
	}

	// verify internal state was recorded
	m.mu.Lock()
	nm, ok := m.reservations[node]
	if !ok {
		m.mu.Unlock()
		t.Fatalf("expected reservations for node")
	}
	r, ok2 := nm[podKey]
	m.mu.Unlock()
	if !ok2 {
		t.Fatalf("expected reservation entry for pod")
	}
	if r.NumCards != 2 || r.Percent != 50 {
		t.Fatalf("unexpected reservation values: %#v", r)
	}

	// release and verify removal
	if err := m.Release(podKey, node); err != nil {
		t.Fatalf("Release failed: %v", err)
	}
	m.mu.Lock()
	nm2, ok3 := m.reservations[node]
	m.mu.Unlock()
	if ok3 && len(nm2) != 0 {
		t.Fatalf("expected reservations removed for node, still present: %#v", nm2)
	}
}
