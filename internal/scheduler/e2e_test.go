package scheduler

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestE2E_SimulatedStatusSocket starts a unix-domain HTTP server that
// implements /status, /reserve, /unreserve and exercises Score + ReserveLogic
// against it.
func TestE2E_SimulatedStatusSocket(t *testing.T) {
	// create temp socket path
	dir := os.TempDir()
	sockPath := filepath.Join(dir, "test-nvidia.sock.status")
	_ = os.Remove(sockPath)

	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to listen on unix socket: %v", err)
	}
	defer func() {
		_ = l.Close()
		_ = os.Remove(sockPath)
	}()

	// simple in-memory state
	deviceMap := map[string]int{"gpu0": 100, "gpu1": 80}

	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(deviceMap)
	})
	mux.HandleFunc("/reserve", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &payload)
		// naive: reduce each listed device by percent
		if devs, ok := payload["devices"].([]interface{}); ok {
			p := int(payload["percent"].(float64))
			for _, d := range devs {
				id := d.(string)
				deviceMap[id] -= p
				if deviceMap[id] < 0 {
					deviceMap[id] = 0
				}
			}
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/unreserve", func(w http.ResponseWriter, r *http.Request) {
		// For simplicity, reset to full
		deviceMap["gpu0"] = 100
		deviceMap["gpu1"] = 80
		w.WriteHeader(http.StatusOK)
	})

	// start server
	srv := &http.Server{Handler: mux}
	go func() {
		_ = srv.Serve(l)
	}()
	defer func() { _ = srv.Close() }()

	// override statusSocketPath to point to our socket
	oldPath := statusSocketPath
	statusSocketPath = func(nodeName string) string { return sockPath }
	defer func() { statusSocketPath = oldPath }()

	// small delay to ensure server started
	time.Sleep(50 * time.Millisecond)

	// Score helper should return top-1 average (gpu0=100)
	sc, err := ScoreNodeTopNAverage("nodeA", 1)
	if err != nil {
		t.Fatalf("ScoreNodeTopNAverage error: %v", err)
	}
	if sc != 100 {
		t.Fatalf("expected score 100, got %d", sc)
	}

	// exercise ReserveLogic: reserve 1 card at 30%
	oldCap := capacityMgr
	f := &fakeCapMgr{}
	capacityMgr = f
	defer func() { capacityMgr = oldCap }()

	req := GPURequest{NumCards: 1, PercentPerCard: 30}
	devices, err := ReserveLogic(context.Background(), "ns/pod", req, "nodeA", pickDevicesFromSocket, ReserveForPod)
	if err != nil {
		t.Fatalf("ReserveLogic failed: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device reserved, got %d", len(devices))
	}

	// after reserve, device value should have decreased
	m, err := GetDeviceRemaining("nodeA")
	if err != nil {
		t.Fatalf("GetDeviceRemaining failed: %v", err)
	}
	// deviceMap initial values were gpu0=100, gpu1=80. After reserving 30% the
	// remaining should be original-30 (either 70 or 50 depending which device
	// was selected by the pick logic).
	expected := 70
	if devices[0] == "gpu1" {
		expected = 50
	}
	if m[devices[0]] != expected {
		t.Fatalf("expected remaining %d on %s, got %d", expected, devices[0], m[devices[0]])
	}

	// unreserve
	if err := UnreserveOnNode(context.Background(), "nodeA", "ns/pod"); err != nil {
		t.Fatalf("UnreserveOnNode failed: %v", err)
	}

	// confirm restored
	m2, err := GetDeviceRemaining("nodeA")
	if err != nil {
		t.Fatalf("GetDeviceRemaining failed: %v", err)
	}
	if m2[devices[0]] == 70 {
		t.Fatalf("expected device to be restored after unreserve")
	}
}
