package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// ReserveOnNode calls the node-local device plugin status socket to reserve
// percent-based capacity for a pod on the specified devices.
// It expects the device plugin status socket to be available via a hostPath
// (e.g. mounted into the scheduler pod) at /var/lib/kubelet/device-plugins/nvidia-gpu.sock.status.
func ReserveOnNode(ctx context.Context, nodeName, podKey string, devices []string, percent int) error {
	statusSock := "/var/lib/kubelet/device-plugins/nvidia-gpu.sock.status"

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", statusSock)
		},
	}
	client := &http.Client{Transport: transport}

	payload := map[string]interface{}{
		"podKey":  podKey,
		"devices": devices,
		"percent": percent,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", "http://unix/reserve", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create reserve request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("reserve request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("reserve request returned status: %s", resp.Status)
	}
	return nil
}

// UnreserveOnNode releases a previous reservation for podKey on the node-local
// device plugin status socket.
func UnreserveOnNode(ctx context.Context, nodeName, podKey string) error {
	statusSock := "/var/lib/kubelet/device-plugins/nvidia-gpu.sock.status"
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "unix", statusSock)
		},
	}
	client := &http.Client{Transport: transport}

	payload := map[string]string{"podKey": podKey}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", "http://unix/unreserve", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create unreserve request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("unreserve request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unreserve request returned status: %s", resp.Status)
	}
	return nil
}
