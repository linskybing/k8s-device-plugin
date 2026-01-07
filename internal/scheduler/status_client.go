package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// pickDevicesFromSocket queries the node-local status socket and returns up to numCards deviceIDs
// with remaining >= percent. This function is provided in a non-build-tag file so tests
// can exercise status behavior.
func pickDevicesFromSocket(nodeName string, numCards, percent int) ([]string, error) {
	statusSock := statusSocketPath(nodeName)
	transport := &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "unix", statusSock)
	}}
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	resp, err := client.Get("http://unix/status")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var m map[string]int
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		return nil, err
	}
	var out []string
	for id, rem := range m {
		if rem >= percent {
			out = append(out, id)
			if len(out) >= numCards {
				break
			}
		}
	}
	if len(out) < numCards {
		return nil, fmt.Errorf("insufficient devices: need %d got %d", numCards, len(out))
	}
	return out, nil
}
