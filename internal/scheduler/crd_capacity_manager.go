package scheduler

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

// CRDCapacityManager implements CapacityManager using a Reservation CRD via the
// dynamic client. This first-version client performs simple create/delete
// operations for Reservation objects in the pod's namespace.
type CRDCapacityManager struct {
	httpClient *http.Client
	baseURL    string
	token      string
}

// compile-time check
var _ CapacityManager = &CRDCapacityManager{}

var reservationGVR = schema.GroupVersionResource{Group: "mps.nvidia.com", Version: "v1", Resource: "reservations"}

// NewCRDCapacityManager constructs a CRD-backed manager using in-cluster
// configuration (or KUBECONFIG if set in the environment).
func NewCRDCapacityManager() (*CRDCapacityManager, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		// Not running in-cluster; return a client that is not configured.
		return &CRDCapacityManager{httpClient: nil}, nil
	}

	// Prepare TLS config using the CA file if present.
	var tlsConfig *tls.Config
	if cfg.TLSClientConfig.CAFile != "" {
		caFile := cfg.TLSClientConfig.CAFile
		caCert, err := os.ReadFile(filepath.Clean(caFile))
		if err == nil {
			pool := x509.NewCertPool()
			pool.AppendCertsFromPEM(caCert)
			tlsConfig = &tls.Config{RootCAs: pool}
		}
	}

	tr := &http.Transport{TLSClientConfig: tlsConfig}
	client := &http.Client{Transport: tr}

	return &CRDCapacityManager{httpClient: client, baseURL: strings.TrimRight(cfg.Host, "/"), token: cfg.BearerToken}, nil
}

func podKeyToNamespaceAndName(podKey string) (string, string) {
	parts := strings.SplitN(podKey, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "default", podKey
}

// Reserve creates or updates a Reservation CR in the pod's namespace.
func (c *CRDCapacityManager) Reserve(podKey, nodeName string, numCards, percent int) error {
	ns, name := podKeyToNamespaceAndName(podKey)
	// Use pod name as the Reservation name to keep resources human-friendly.
	resName := name
	if c.httpClient == nil {
		return fmt.Errorf("CRDCapacityManager not configured (not running in-cluster)")
	}

	url := fmt.Sprintf("%s/apis/mps.nvidia.com/v1/namespaces/%s/reservations", c.baseURL, ns)
	body := map[string]interface{}{
		"apiVersion": "mps.nvidia.com/v1",
		"kind":       "Reservation",
		"metadata": map[string]interface{}{
			"name": resName,
		},
		"spec": map[string]interface{}{
			"podKey":         podKey,
			"nodeName":       nodeName,
			"numCards":       numCards,
			"percentPerCard": percent,
		},
	}
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, _ := http.NewRequestWithContext(context.Background(), "POST", url, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		return nil
	}
	// Read body for debugging
	rb, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("reservation create failed: status=%d body=%s", resp.StatusCode, string(rb))
}

// Release deletes the Reservation CR for the pod in its namespace.
func (c *CRDCapacityManager) Release(podKey, nodeName string) error {
	ns, name := podKeyToNamespaceAndName(podKey)
	resName := name
	if c.httpClient == nil {
		return nil
	}
	url := fmt.Sprintf("%s/apis/mps.nvidia.com/v1/namespaces/%s/reservations/%s", c.baseURL, ns, resName)
	req, _ := http.NewRequestWithContext(context.Background(), "DELETE", url, nil)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	// ignore not found
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	rb, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("reservation delete failed: status=%d body=%s", resp.StatusCode, string(rb))
}
