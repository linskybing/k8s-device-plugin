/**
# Copyright 2024 NVIDIA CORPORATION
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
**/

package plugin

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"k8s.io/klog/v2"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	spec "github.com/NVIDIA/k8s-device-plugin/api/config/v1"
	"github.com/NVIDIA/k8s-device-plugin/cmd/mps-control-daemon/mps"
	"github.com/NVIDIA/k8s-device-plugin/internal/rm"
)

type mpsOptions struct {
	enabled      bool
	resourceName spec.ResourceName
	daemon       *mps.Daemon
	hostRoot     mps.Root
	mpsConfig    *spec.GPUMPSConfig
	rm           rm.ResourceManager
}

// getMPSOptions returns the MPS options specified for the resource manager.
// If MPS is not configured and empty set of options is returned.
func (o *options) getMPSOptions(resourceManager rm.ResourceManager) (mpsOptions, error) {
	// Determine if MPS is enabled globally (sharing strategy) or per-GPU
	sharingEnabled := o.config.Sharing.SharingStrategy() == spec.SharingStrategyMPS
	gpuConfig := o.getGPUConfigForResource(resourceManager.Resource())
	if !sharingEnabled {
		if gpuConfig == nil || gpuConfig.MPS == nil || !gpuConfig.MPS.Enabled {
			return mpsOptions{}, nil
		}
	}

	// TODO: It might make sense to pull this logic into a resource manager.
	for _, device := range resourceManager.Devices() {
		if device.IsMigDevice() {
			return mpsOptions{}, errors.New("sharing using MPS is not supported for MIG devices")
		}
	}

	var mpsCfg *spec.GPUMPSConfig
	if gpuConfig != nil {
		mpsCfg = gpuConfig.MPS
	}
	if mpsCfg != nil && !mpsCfg.Enabled {
		mpsCfg = nil
	}

	m := mpsOptions{
		enabled:      true,
		resourceName: resourceManager.Resource(),
		daemon:       mps.NewDaemon(resourceManager, mps.ContainerRoot, mpsCfg),
		hostRoot:     mps.Root(*o.config.Flags.MpsRoot),
		mpsConfig:    mpsCfg,
		rm:           resourceManager,
	}
	return m, nil
}

// getGPUConfigForResource returns the GPU config that matches the resource name, if any.
func (o *options) getGPUConfigForResource(resourceName spec.ResourceName) *spec.GPUConfig {
	if o.config == nil || o.config.IndividualGPU == nil {
		return nil
	}
	for i := range o.config.IndividualGPU.GPUConfigs {
		cfg := &o.config.IndividualGPU.GPUConfigs[i]
		name, err := cfg.GetResourceName(o.config.IndividualGPU.NamePattern)
		if err != nil {
			continue
		}
		if name == resourceName {
			return cfg
		}
	}
	return nil
}

func (m *mpsOptions) waitForDaemon() error {
	if m == nil || !m.enabled {
		return nil
	}
	// TODO: Check the .ready file here.
	// TODO: Have some retry strategy here.
	if err := m.daemon.AssertHealthy(); err != nil {
		return fmt.Errorf("error checking MPS daemon health: %w", err)
	}
	klog.InfoS("MPS daemon is healthy", "resource", m.resourceName)
	return nil
}

func (m *mpsOptions) updateReponse(response *pluginapi.ContainerAllocateResponse, requestIds []string) {
	if m == nil || !m.enabled {
		return
	}

	// Build per-GPU allocation counts from annotated IDs (base ID + replica index)
	perGPUCount := make(map[string]int)
	for _, id := range requestIds {
		annotated := rm.AnnotatedID(id)
		base := annotated.GetID()
		perGPUCount[base]++
	}

	// Collect unique GPU indices and UUID mapping keyed by base IDs (strip replica suffixes)
	seenIndex := make(map[string]bool)
	seenBase := make(map[string]bool)
	indexQuota := make(map[string]int)
	var indices []string
	var gpuMapParts []string
	for _, reqID := range requestIds {
		annotated := rm.AnnotatedID(reqID)
		baseID := annotated.GetID()

		// Prefer daemon device map; fall back to resource manager devices.
		var dev *rm.Device
		if m.daemon != nil && m.daemon.Devices() != nil {
			dev = m.daemon.Devices().GetByID(reqID)
			if dev == nil {
				dev = m.daemon.Devices().GetByID(baseID)
			}
		}
		if dev == nil && m.rm != nil {
			dev = m.rm.Devices().GetByID(baseID)
		}
		if dev == nil {
			klog.InfoS("Skipping unknown requested GPU", "id", reqID, "baseID", baseID)
			continue
		}
		indexQuota[dev.Index]++
		if !seenIndex[dev.Index] {
			seenIndex[dev.Index] = true
			indices = append(indices, dev.Index)
		}
		if !seenBase[baseID] {
			seenBase[baseID] = true
			gpuMapParts = append(gpuMapParts, fmt.Sprintf("%s:%s:%d", dev.Index, baseID, perGPUCount[baseID]))
		}
	}

	sort.Strings(indices)

	// Inject MPS pipe and log directories
	if response.Envs == nil {
		response.Envs = make(map[string]string)
	}
	for k, v := range m.daemon.EnvVars() {
		response.Envs[k] = v
	}

	// Compute thread percentage based on number of distinct GPUs allocated
	if len(indices) > 0 {
		perGPUPercentage := 100 / len(indices)
		if perGPUPercentage < 1 {
			perGPUPercentage = 1
		}
		response.Envs["CUDA_MPS_ACTIVE_THREAD_PERCENTAGE"] = fmt.Sprintf("%d", perGPUPercentage)
		klog.InfoS("Injecting MPS thread percentage for multi-GPU",
			"resource", m.resourceName,
			"gpuCount", len(indices),
			"percentage", perGPUPercentage)
	}

	// Visible devices: convert global indices to relative indices (0, 1, 2, ...)
	// CUDA expects relative positions within the visible set, not absolute GPU indices
	var relativeIndices []string
	for i := range indices {
		relativeIndices = append(relativeIndices, fmt.Sprintf("%d", i))
	}
	relativeMerged := strings.Join(relativeIndices, ",")
	if relativeMerged != "" {
		response.Envs["NVIDIA_VISIBLE_DEVICES"] = relativeMerged
		response.Envs["CUDA_VISIBLE_DEVICES"] = relativeMerged
		klog.InfoS("Setting merged visible devices (relative indices)",
			"resource", m.resourceName,
			"value", relativeMerged,
			"globalIndices", strings.Join(indices, ","))
	}

	if len(indexQuota) > 0 {
		var quotaParts []string
		for _, idx := range indices {
			quotaParts = append(quotaParts, fmt.Sprintf("%s:%d", idx, indexQuota[idx]))
		}
		sort.Strings(quotaParts)
		response.Envs["MPS_GPU_QUOTA"] = strings.Join(quotaParts, ";")
		klog.InfoS("Setting user-visible GPU quota map", "resource", m.resourceName, "value", response.Envs["MPS_GPU_QUOTA"])
	}

	// Expose mapping and per-GPU token usage
	if len(gpuMapParts) > 0 {
		sort.Strings(gpuMapParts)
		response.Envs["GPU_DEVICE_MAP"] = strings.Join(gpuMapParts, ";")
		// Also surface as annotation for external schedulers if supported
		if response.Annotations == nil {
			response.Annotations = make(map[string]string)
		}
		response.Annotations["mps.nvidia.com/assigned-gpus"] = strings.Join(indices, ",")
		response.Annotations["mps.nvidia.com/gpu-device-map"] = strings.Join(gpuMapParts, ";")
	}

	// Mounts: pipe and shm (shared across all GPUs in this resource)
	response.Mounts = append(response.Mounts,
		&pluginapi.Mount{ContainerPath: m.daemon.PipeDir(), HostPath: m.hostRoot.PipeDir(m.resourceName)},
		&pluginapi.Mount{ContainerPath: m.daemon.ShmDir(), HostPath: m.hostRoot.ShmDir(m.resourceName)},
	)

	// Add GPU device files for all selected indices
	controlDevices := []string{"/dev/nvidiactl", "/dev/nvidia-uvm", "/dev/nvidia-uvm-tools", "/dev/nvidia-modeset"}
	for _, idx := range indices {
		gpuDevice := "/dev/nvidia" + idx
		response.Devices = append(response.Devices, &pluginapi.DeviceSpec{ContainerPath: gpuDevice, HostPath: gpuDevice, Permissions: "rw"})
	}
	if len(indices) == 0 {
		klog.InfoS("No GPU indices resolved for MPS request", "resource", m.resourceName, "requestIds", requestIds, "perGPUCount", perGPUCount)
	}
	for _, dev := range controlDevices {
		response.Devices = append(response.Devices, &pluginapi.DeviceSpec{ContainerPath: dev, HostPath: dev, Permissions: "rw"})
	}

	klog.InfoS("Added GPU devices for MPS",
		"resource", m.resourceName,
		"gpuCount", len(indices),
		"totalDevices", len(response.Devices))
}
