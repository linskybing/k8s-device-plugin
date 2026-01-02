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

func (m *mpsOptions) updateReponse(response *pluginapi.ContainerAllocateResponse, replicaCount int) {
	if m == nil || !m.enabled {
		return
	}
	// Inject MPS pipe and log directories
	for k, v := range m.daemon.EnvVars() {
		response.Envs[k] = v
	}

	// Auto-calculate and inject CUDA_MPS_ACTIVE_THREAD_PERCENTAGE based on replica count
	// This prevents pods from tampering with the value while allowing per-pod customization
	if replicaCount > 0 && m.mpsConfig != nil && m.mpsConfig.Replicas > 0 {
		// Thread percentage = (requested replicas / total replicas) * 100
		threadPercentage := (replicaCount * 100) / int(m.mpsConfig.Replicas)
		if threadPercentage > 100 {
			threadPercentage = 100
		}
		if threadPercentage > 0 {
			response.Envs["CUDA_MPS_ACTIVE_THREAD_PERCENTAGE"] = fmt.Sprintf("%d", threadPercentage)
			klog.InfoS("Injecting MPS thread percentage based on replica request",
				"resource", m.resourceName,
				"requested", replicaCount,
				"total", m.mpsConfig.Replicas,
				"percentage", threadPercentage)
		}
	}

	// CRITICAL: Set NVIDIA_VISIBLE_DEVICES to enable GPU access in container
	// Without this, nvidia-container-runtime sets it to "void" which disables GPU access
	// We use the device index (e.g., "1") to match CUDA_VISIBLE_DEVICES
	if cvd := m.daemon.VisibleDevices(); cvd != "" {
		response.Envs["NVIDIA_VISIBLE_DEVICES"] = cvd
		klog.InfoS("Setting NVIDIA_VISIBLE_DEVICES to enable GPU access",
			"resource", m.resourceName,
			"value", cvd)
	}

	response.Mounts = append(response.Mounts,
		&pluginapi.Mount{
			ContainerPath: m.daemon.PipeDir(),
			HostPath:      m.hostRoot.PipeDir(m.resourceName),
		},
		&pluginapi.Mount{
			ContainerPath: m.daemon.ShmDir(),
			HostPath:      m.hostRoot.ShmDir(m.resourceName),
		},
	)

	// Add GPU device files so container runtime can access the hardware
	// The device index is extracted from CUDA_VISIBLE_DEVICES
	if cvd := m.daemon.VisibleDevices(); cvd != "" {
		// Add main GPU device (e.g., /dev/nvidia1)
		gpuDevice := "/dev/nvidia" + cvd
		response.Devices = append(response.Devices, &pluginapi.DeviceSpec{
			ContainerPath: gpuDevice,
			HostPath:      gpuDevice,
			Permissions:   "rw",
		})

		// Add required nvidia control devices
		controlDevices := []string{
			"/dev/nvidiactl",
			"/dev/nvidia-uvm",
			"/dev/nvidia-uvm-tools",
			"/dev/nvidia-modeset",
		}
		for _, dev := range controlDevices {
			response.Devices = append(response.Devices, &pluginapi.DeviceSpec{
				ContainerPath: dev,
				HostPath:      dev,
				Permissions:   "rw",
			})
		}

		klog.InfoS("Added GPU device specs for MPS",
			"resource", m.resourceName,
			"gpuDevice", gpuDevice,
			"controlDevices", len(controlDevices))
	}
}
