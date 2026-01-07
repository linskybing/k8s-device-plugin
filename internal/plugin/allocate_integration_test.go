package plugin

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	spec "github.com/NVIDIA/k8s-device-plugin/api/config/v1"
)

// TestReserveAllocateUnreserve verifies the lifecycle where the scheduler
// reserves capacity, Allocate() finalizes (consumes) the reservation, and
// Unreserve becomes a no-op for already-consumed reservations.
func TestReserveAllocateUnreserve(t *testing.T) {
	var p nvidiaDevicePlugin
	p.socket = "/tmp/test-nvidia.sock"
	p.initialize()

	// ensure deviceListStrategies is not empty so Allocate does not early-return
	if s, err := spec.NewDeviceListStrategies([]string{spec.DeviceListStrategyEnvVar}); err == nil {
		p.deviceListStrategies = s
	}

	// minimal config so getAllocateResponse chooses UUID strategy
	ds := spec.DeviceIDStrategyUUID
	p.config = &spec.Config{Flags: spec.Flags{CommandLineFlags: spec.CommandLineFlags{Plugin: &spec.PluginCommandLineFlags{DeviceIDStrategy: &ds}}}}

	// ensure initial device state
	p.statusMux.Lock()
	p.deviceRemaining["dev0"] = 100
	p.statusMux.Unlock()

	// Reserve 30%
	reqBody := map[string]interface{}{"podKey": "ns/pod1", "devices": []string{"dev0"}, "percent": 30}
	b, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/reserve", bytes.NewReader(b))
	w := httptest.NewRecorder()
	p.reserveHandler(w, req)
	if w.Code != 200 {
		t.Fatalf("reserve handler returned non-200: %d", w.Code)
	}

	// verify remaining and allocation recorded
	p.statusMux.RLock()
	rem := p.deviceRemaining["dev0"]
	p.statusMux.RUnlock()
	if rem != 70 {
		t.Fatalf("expected remaining 70 after reserve, got %d", rem)
	}

	p.allocationsMux.RLock()
	alloc, ok := p.allocations["ns/pod1"]
	p.allocationsMux.RUnlock()
	if !ok || alloc["dev0"] != 30 {
		t.Fatalf("expected allocation 30 for dev0, got %#v", alloc)
	}

	// Call Allocate (via helper) which should consume the reservation
	if _, err := p.getAllocateResponse([]string{"dev0"}); err != nil {
		t.Fatalf("getAllocateResponse failed: %v", err)
	}

	// allocations entry for dev0 should be removed (consumed)
	p.allocationsMux.RLock()
	alloc2, ok2 := p.allocations["ns/pod1"]
	p.allocationsMux.RUnlock()
	if ok2 {
		if val := alloc2["dev0"]; val != 0 {
			t.Fatalf("expected dev0 reservation consumed, still present: %d", val)
		}
	}

	// Calling Unreserve should be a no-op and not increase remaining (since allocation was finalized)
	ub, _ := json.Marshal(map[string]string{"podKey": "ns/pod1"})
	ureq := httptest.NewRequest("POST", "/unreserve", bytes.NewReader(ub))
	uw := httptest.NewRecorder()
	p.unreserveHandler(uw, ureq)
	if uw.Code != 200 {
		t.Fatalf("unreserve handler returned non-200: %d", uw.Code)
	}

	p.statusMux.RLock()
	rem3 := p.deviceRemaining["dev0"]
	p.statusMux.RUnlock()
	if rem3 != 70 {
		t.Fatalf("expected remaining 70 after allocate+unreserve, got %d", rem3)
	}

	_ = p.Stop()
}
