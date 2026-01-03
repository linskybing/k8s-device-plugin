package plugin

import (
	"testing"

	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	spec "github.com/NVIDIA/k8s-device-plugin/api/config/v1"
	"github.com/NVIDIA/k8s-device-plugin/cmd/mps-control-daemon/mps"
	"github.com/NVIDIA/k8s-device-plugin/internal/rm"
)

func TestUpdateResponseForMPS_MultiGPUMapping(t *testing.T) {
	// Two physical GPUs with 25 replicas each; request 3 replicas on GPU0 and 2 on GPU1
	devices := rm.Devices{
		"GPU-AAAA": {Device: pluginapi.Device{ID: "GPU-AAAA"}, Index: "0"},
		"GPU-BBBB": {Device: pluginapi.Device{ID: "GPU-BBBB"}, Index: "1"},
	}

	rmMock := &rm.ResourceManagerMock{
		DevicesFunc: func() rm.Devices {
			return devices
		},
		ResourceFunc: func() spec.ResourceName {
			return spec.ResourceName("nvidia.com/gpu-mps")
		},
	}

	opts := mpsOptions{
		enabled:      true,
		resourceName: spec.ResourceName("nvidia.com/gpu-mps"),
		daemon:       mps.NewDaemon(rmMock, mps.ContainerRoot, &spec.GPUMPSConfig{Replicas: 25}),
		hostRoot:     mps.Root("/run/nvidia/mps"),
		mpsConfig:    &spec.GPUMPSConfig{Replicas: 25},
	}

	reqIDs := []string{"GPU-AAAA::0", "GPU-AAAA::1", "GPU-AAAA::2", "GPU-BBBB::0", "GPU-BBBB::1"}
	resp := &pluginapi.ContainerAllocateResponse{Envs: make(map[string]string)}

	opts.updateReponse(resp, reqIDs)

	if got := resp.Envs["NVIDIA_VISIBLE_DEVICES"]; got != "0,1" {
		t.Fatalf("expected visible devices '0,1', got %q", got)
	}
	if got := resp.Envs["CUDA_VISIBLE_DEVICES"]; got != "0,1" {
		t.Fatalf("expected CUDA_VISIBLE_DEVICES '0,1', got %q", got)
	}
	if got := resp.Envs["GPU_DEVICE_MAP"]; got != "0:GPU-AAAA:3;1:GPU-BBBB:2" {
		t.Fatalf("unexpected GPU_DEVICE_MAP: %q", got)
	}
	if got := resp.Annotations["mps.nvidia.com/assigned-gpus"]; got != "0,1" {
		t.Fatalf("unexpected assigned-gpus annotation: %q", got)
	}
	if got := resp.Annotations["mps.nvidia.com/gpu-device-map"]; got != "0:GPU-AAAA:3;1:GPU-BBBB:2" {
		t.Fatalf("unexpected gpu-device-map annotation: %q", got)
	}
}
