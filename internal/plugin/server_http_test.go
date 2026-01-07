package plugin

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestReserveUnreserveHandlers(t *testing.T) {
	var p nvidiaDevicePlugin
	p.socket = "/tmp/test-nvidia.sock"
	p.initialize()

	// ensure initial state
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

	// verify remaining and allocation
	p.statusMux.RLock()
	rem := p.deviceRemaining["dev0"]
	p.statusMux.RUnlock()
	if rem != 70 {
		t.Fatalf("expected remaining 70 after reserve, got %d", rem)
	}

	p.allocationsMux.RLock()
	alloc, ok := p.allocations["ns/pod1"]
	p.allocationsMux.RUnlock()
	if !ok {
		t.Fatalf("expected allocations entry for pod")
	}
	if alloc["dev0"] != 30 {
		t.Fatalf("expected allocation 30 for dev0, got %d", alloc["dev0"])
	}

	// Unreserve
	ub, _ := json.Marshal(map[string]string{"podKey": "ns/pod1"})
	ureq := httptest.NewRequest("POST", "/unreserve", bytes.NewReader(ub))
	uw := httptest.NewRecorder()
	p.unreserveHandler(uw, ureq)
	if uw.Code != 200 {
		t.Fatalf("unreserve handler returned non-200: %d", uw.Code)
	}

	// verify restored
	p.statusMux.RLock()
	rem2 := p.deviceRemaining["dev0"]
	p.statusMux.RUnlock()
	if rem2 != 100 {
		t.Fatalf("expected remaining 100 after unreserve, got %d", rem2)
	}

	p.allocationsMux.RLock()
	_, ok2 := p.allocations["ns/pod1"]
	p.allocationsMux.RUnlock()
	if ok2 {
		t.Fatalf("expected allocations entry removed after unreserve")
	}

	// status handler should report the map
	sreq := httptest.NewRequest("GET", "/status", nil)
	sw := httptest.NewRecorder()
	p.statusHandler(sw, sreq)
	if sw.Code != 200 {
		t.Fatalf("status handler returned non-200: %d", sw.Code)
	}
	var resp map[string]int
	if err := json.NewDecoder(sw.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode status response: %v", err)
	}
	if resp["dev0"] != 100 {
		t.Fatalf("expected status dev0=100, got %d", resp["dev0"])
	}

	// cleanup
	_ = p.Stop()
}
