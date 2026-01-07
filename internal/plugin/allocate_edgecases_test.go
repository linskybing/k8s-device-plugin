package plugin

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	spec "github.com/NVIDIA/k8s-device-plugin/api/config/v1"
)

func TestMultipleReservationsOverflow(t *testing.T) {
	var p nvidiaDevicePlugin
	p.socket = "/tmp/test-nvidia.sock"
	p.initialize()

	// ensure deviceListStrategies is set so Allocate path is exercised
	if s, err := spec.NewDeviceListStrategies([]string{spec.DeviceListStrategyEnvVar}); err == nil {
		p.deviceListStrategies = s
	}

	p.statusMux.Lock()
	p.deviceRemaining["dev0"] = 50
	p.statusMux.Unlock()

	// pod1 reserves 30 -> remaining 20
	r1 := map[string]interface{}{"podKey": "ns/p1", "devices": []string{"dev0"}, "percent": 30}
	b1, _ := json.Marshal(r1)
	req1 := httptest.NewRequest("POST", "/reserve", bytes.NewReader(b1))
	w1 := httptest.NewRecorder()
	p.reserveHandler(w1, req1)
	if w1.Code != 200 {
		t.Fatalf("reserve1 failed: %d", w1.Code)
	}

	// pod2 reserves 30 -> only 20 left, so pod2 gets 20 and remaining drops to 0
	r2 := map[string]interface{}{"podKey": "ns/p2", "devices": []string{"dev0"}, "percent": 30}
	b2, _ := json.Marshal(r2)
	req2 := httptest.NewRequest("POST", "/reserve", bytes.NewReader(b2))
	w2 := httptest.NewRecorder()
	p.reserveHandler(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("reserve2 failed: %d", w2.Code)
	}

	p.statusMux.RLock()
	rem := p.deviceRemaining["dev0"]
	p.statusMux.RUnlock()
	if rem != 0 {
		t.Fatalf("expected remaining 0 after overflow reserves, got %d", rem)
	}

	p.allocationsMux.RLock()
	a1 := p.allocations["ns/p1"]["dev0"]
	a2 := p.allocations["ns/p2"]["dev0"]
	p.allocationsMux.RUnlock()
	if a1 != 30 {
		t.Fatalf("expected p1 alloc 30, got %d", a1)
	}
	if a2 != 20 {
		t.Fatalf("expected p2 alloc 20, got %d", a2)
	}

	// unreserve p1 -> remaining should increase by 30
	ub, _ := json.Marshal(map[string]string{"podKey": "ns/p1"})
	ureq := httptest.NewRequest("POST", "/unreserve", bytes.NewReader(ub))
	uw := httptest.NewRecorder()
	p.unreserveHandler(uw, ureq)
	if uw.Code != 200 {
		t.Fatalf("unreserve p1 failed: %d", uw.Code)
	}

	p.statusMux.RLock()
	rem2 := p.deviceRemaining["dev0"]
	p.statusMux.RUnlock()
	if rem2 != 30 {
		t.Fatalf("expected remaining 30 after unreserve p1, got %d", rem2)
	}

	_ = p.Stop()
}

func TestPartialAllocateConsume(t *testing.T) {
	var p nvidiaDevicePlugin
	p.socket = "/tmp/test-nvidia.sock"
	p.initialize()

	if s, err := spec.NewDeviceListStrategies([]string{spec.DeviceListStrategyEnvVar}); err == nil {
		p.deviceListStrategies = s
	}

	// ensure DeviceIDStrategy is set so uniqueDeviceIDsFromAnnotatedDeviceIDs can run
	ds := spec.DeviceIDStrategyUUID
	p.config = &spec.Config{Flags: spec.Flags{CommandLineFlags: spec.CommandLineFlags{Plugin: &spec.PluginCommandLineFlags{DeviceIDStrategy: &ds}}}}

	p.statusMux.Lock()
	p.deviceRemaining["dev0"] = 100
	p.deviceRemaining["dev1"] = 100
	p.statusMux.Unlock()

	// reserve both devices for pod
	r := map[string]interface{}{"podKey": "ns/px", "devices": []string{"dev0", "dev1"}, "percent": 30}
	b, _ := json.Marshal(r)
	req := httptest.NewRequest("POST", "/reserve", bytes.NewReader(b))
	w := httptest.NewRecorder()
	p.reserveHandler(w, req)
	if w.Code != 200 {
		t.Fatalf("reserve failed: %d", w.Code)
	}

	// Allocate only dev0 -> should consume only dev0 entry
	if _, err := p.getAllocateResponse([]string{"dev0"}); err != nil {
		t.Fatalf("allocate failed: %v", err)
	}

	p.allocationsMux.RLock()
	allocMap := p.allocations["ns/px"]
	p.allocationsMux.RUnlock()
	if allocMap == nil {
		t.Fatalf("expected remaining allocation entries for pod")
	}
	if _, ok := allocMap["dev0"]; ok {
		t.Fatalf("expected dev0 entry consumed, still present")
	}
	if val, ok := allocMap["dev1"]; !ok || val != 30 {
		t.Fatalf("expected dev1 still reserved 30, got %d (ok=%v)", val, ok)
	}

	_ = p.Stop()
}

func TestDoubleReserveSamePod(t *testing.T) {
	var p nvidiaDevicePlugin
	p.socket = "/tmp/test-nvidia.sock"
	p.initialize()

	if s, err := spec.NewDeviceListStrategies([]string{spec.DeviceListStrategyEnvVar}); err == nil {
		p.deviceListStrategies = s
	}

	p.statusMux.Lock()
	p.deviceRemaining["dev0"] = 100
	p.statusMux.Unlock()

	// first reserve 20
	r1 := map[string]interface{}{"podKey": "ns/pdup", "devices": []string{"dev0"}, "percent": 20}
	b1, _ := json.Marshal(r1)
	req1 := httptest.NewRequest("POST", "/reserve", bytes.NewReader(b1))
	w1 := httptest.NewRecorder()
	p.reserveHandler(w1, req1)
	if w1.Code != 200 {
		t.Fatalf("reserve1 failed: %d", w1.Code)
	}

	// second reserve 15 by same pod -> should accumulate to 35
	r2 := map[string]interface{}{"podKey": "ns/pdup", "devices": []string{"dev0"}, "percent": 15}
	b2, _ := json.Marshal(r2)
	req2 := httptest.NewRequest("POST", "/reserve", bytes.NewReader(b2))
	w2 := httptest.NewRecorder()
	p.reserveHandler(w2, req2)
	if w2.Code != 200 {
		t.Fatalf("reserve2 failed: %d", w2.Code)
	}

	p.allocationsMux.RLock()
	alloc := p.allocations["ns/pdup"]["dev0"]
	p.allocationsMux.RUnlock()
	if alloc != 35 {
		t.Fatalf("expected accumulated alloc 35, got %d", alloc)
	}

	_ = p.Stop()
}
