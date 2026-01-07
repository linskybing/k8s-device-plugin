package scheduler

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// helper to create a unix socket HTTP server that responds according to handler
func serveUnixHTTP(t *testing.T, sockPath string, handler http.Handler) (func(), string) {
	// ensure directory exists
	dir := filepath.Dir(sockPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// remove any existing socket
	_ = os.Remove(sockPath)
	l, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	srv := &http.Server{Handler: handler}
	go srv.Serve(l)
	cleanup := func() {
		srv.Close()
		l.Close()
		_ = os.Remove(sockPath)
	}
	return cleanup, sockPath
}

// Test ReserveForPod retries: server fails first attempts then succeeds.
func TestReserveForPod_Retries(t *testing.T) {
	// create a temp dir for socket
	sock := filepath.Join(os.TempDir(), "ndp-test-retry.sock")

	// handler that fails twice then succeeds
	attempts := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if r.URL.Path == "/reserve" {
			if attempts < 3 {
				http.Error(w, "transient", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
			return
		}
		http.NotFound(w, r)
	})

	cleanup, _ := serveUnixHTTP(t, sock, handler)
	defer cleanup()

	// point the package hook to our socket
	old := statusSocketPath
	statusSocketPath = func(nodeName string) string { return sock }
	defer func() { statusSocketPath = old }()

	// reduce backoff to speed test
	oldAttempts := ReserveRetryAttempts
	ReserveRetryAttempts = 3
	defer func() { ReserveRetryAttempts = oldAttempts }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := ReserveForPod(ctx, "nodeA", "ns/pod", []string{"gpu0"}, 50)
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
}

// Test ReserveForPod fails after retries when server always errors.
func TestReserveForPod_Failures(t *testing.T) {
	sock := filepath.Join(os.TempDir(), "ndp-test-fail.sock")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "perm", http.StatusInternalServerError)
	})
	cleanup, _ := serveUnixHTTP(t, sock, handler)
	defer cleanup()

	old := statusSocketPath
	statusSocketPath = func(nodeName string) string { return sock }
	defer func() { statusSocketPath = old }()

	oldAttempts := ReserveRetryAttempts
	ReserveRetryAttempts = 2
	defer func() { ReserveRetryAttempts = oldAttempts }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := ReserveForPod(ctx, "nodeB", "ns/pod2", []string{"gpu0"}, 20); err == nil {
		t.Fatalf("expected error after retries")
	}
}

// Test pickDevicesFromNode reads status and picks required devices.
func TestPickDevicesFromNode(t *testing.T) {
	sock := filepath.Join(os.TempDir(), "ndp-test-status.sock")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			_ = json.NewEncoder(w).Encode(map[string]int{"gpu-a": 80, "gpu-b": 30, "gpu-c": 90})
			return
		}
		http.NotFound(w, r)
	})
	cleanup, _ := serveUnixHTTP(t, sock, handler)
	defer cleanup()

	old := statusSocketPath
	statusSocketPath = func(nodeName string) string { return sock }
	defer func() { statusSocketPath = old }()

	devs, err := pickDevicesFromSocket("nodeX", 2, 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devs) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devs))
	}
}
