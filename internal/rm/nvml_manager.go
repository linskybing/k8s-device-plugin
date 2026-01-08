/*
 * Copyright (c) 2019-2022, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY Type, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package rm

import (
	"fmt"

	"github.com/NVIDIA/go-gpuallocator/gpuallocator"
	"github.com/NVIDIA/go-nvlib/pkg/nvlib/device"
	"github.com/NVIDIA/go-nvlib/pkg/nvlib/info"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"k8s.io/klog/v2"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"

	spec "github.com/NVIDIA/k8s-device-plugin/api/config/v1"
)

type nvmlResourceManager struct {
	resourceManager
	nvml nvml.Interface
}

var _ ResourceManager = (*nvmlResourceManager)(nil)

// NewNVMLResourceManagers returns a set of ResourceManagers, one for each NVML resource in 'config'.
func NewNVMLResourceManagers(infolib info.Interface, nvmllib nvml.Interface, devicelib device.Interface, config *spec.Config) ([]ResourceManager, error) {
	ret := nvmllib.Init()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to initialize NVML: %v", ret)
	}
	defer func() {
		ret := nvmllib.Shutdown()
		if ret != nvml.SUCCESS {
			klog.Infof("Error shutting down NVML: %v", ret)
		}
	}()

	// If IndividualGPU mode is enabled, create a separate resource manager for each GPU
	if config.IndividualGPU != nil && config.IndividualGPU.Enabled {
		return newIndividualGPUResourceManagers(infolib, nvmllib, devicelib, config)
	}

	deviceMap, err := NewDeviceMap(infolib, devicelib, config)
	if err != nil {
		return nil, fmt.Errorf("error building device map: %v", err)
	}

	var rms []ResourceManager
	for resourceName, devices := range deviceMap {
		if len(devices) == 0 {
			continue
		}
		r := &nvmlResourceManager{
			resourceManager: resourceManager{
				config:   config,
				resource: resourceName,
				devices:  devices,
			},
			nvml: nvmllib,
		}
		rms = append(rms, r)
	}

	return rms, nil
}

// GetPreferredAllocation runs an allocation algorithm over the inputs.
// The algorithm chosen is based both on the incoming set of available devices and various config settings.
func (r *nvmlResourceManager) GetPreferredAllocation(available, required []string, size int) ([]string, error) {
	return r.getPreferredAllocation(available, required, size)
}

// GetDevicePaths returns the required and optional device nodes for the requested resources
func (r *nvmlResourceManager) GetDevicePaths(ids []string) []string {
	paths := []string{
		"/dev/nvidiactl",
		"/dev/nvidia-uvm",
		"/dev/nvidia-uvm-tools",
		"/dev/nvidia-modeset",
	}

	return append(paths, r.Devices().Subset(ids).GetPaths()...)
}

// CheckHealth performs health checks on a set of devices, writing to the 'unhealthy' channel with any unhealthy devices
func (r *nvmlResourceManager) CheckHealth(stop <-chan interface{}, unhealthy chan<- *Device) error {
	return r.checkHealth(stop, r.devices, unhealthy)
}

// getPreferredAllocation runs an allocation algorithm over the inputs.
// The algorithm chosen is based both on the incoming set of available devices and various config settings.
func (r *nvmlResourceManager) getPreferredAllocation(available, required []string, size int) ([]string, error) {
	// If all of the available devices are full GPUs without replicas, then
	// calculate an aligned allocation across those devices.
	if r.Devices().AlignedAllocationSupported() && !AnnotatedIDs(available).AnyHasAnnotations() {
		return r.alignedAlloc(available, required, size)
	}

	// Otherwise, use capacity-aware allocation to prefer single-card allocations
	// and minimize fragmentation. Falls back to best-effort if full request
	// cannot be satisfied.
	return r.capacityAwareAlloc(available, required, size)
}

// alignedAlloc shells out to the alignedAllocationPolicy that is set in
// order to calculate the preferred allocation.
func (r *nvmlResourceManager) alignedAlloc(available, required []string, size int) ([]string, error) {
	var devices []string

	linkedDevices, err := gpuallocator.NewDevices(
		gpuallocator.WithNvmlLib(r.nvml),
	)
	if err != nil {
		return nil, fmt.Errorf("unable to get device link information: %w", err)
	}

	availableDevices, err := linkedDevices.Filter(available)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve list of available devices: %v", err)
	}

	requiredDevices, err := linkedDevices.Filter(required)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve list of required devices: %v", err)
	}

	allocatedDevices := gpuallocator.NewBestEffortPolicy().Allocate(availableDevices, requiredDevices, size)
	for _, device := range allocatedDevices {
		devices = append(devices, device.UUID)
	}

	return devices, nil
}

// newIndividualGPUResourceManagers creates a separate ResourceManager for each GPU
func newIndividualGPUResourceManagers(infolib info.Interface, nvmllib nvml.Interface, devicelib device.Interface, config *spec.Config) ([]ResourceManager, error) {
	var rms []ResourceManager

	// Build a deviceMapBuilder to enumerate GPUs
	b := deviceMapBuilder{
		Interface:   devicelib,
		migStrategy: config.Flags.MigStrategy,
		resources:   &config.Resources,
	}

	if infolib.ResolvePlatform() == info.PlatformWSL {
		b.newGPUDevice = newWslGPUDevice
	} else {
		b.newGPUDevice = newNvmlGPUDevice
	}

	// Enumerate all GPUs and create a ResourceManager for each
	err := b.VisitDevices(func(i int, gpu device.Device) error {
		// Skip MIG-enabled GPUs if MIG strategy is not none
		migEnabled, err := gpu.IsMigEnabled()
		if err != nil {
			return fmt.Errorf("error checking if MIG is enabled on GPU %d: %v", i, err)
		}
		if migEnabled && *b.migStrategy != spec.MigStrategyNone {
			return nil
		}

		// Get GPU config for this index
		var gpuConfig *spec.GPUConfig
		gpuUUID, _ := gpu.GetUUID()
		for j := range config.IndividualGPU.GPUConfigs {
			candidate := &config.IndividualGPU.GPUConfigs[j]
			if candidate.Index == i {
				gpuConfig = candidate
				break
			}
			if candidate.UUID != "" && gpuUUID == candidate.UUID {
				gpuConfig = candidate
				break
			}
		}

		// If no specific config, create a default one
		if gpuConfig == nil {
			gpuConfig = &spec.GPUConfig{
				Index: i,
			}
		}

		// Get resource name for this GPU
		resourceName, err := gpuConfig.GetResourceName(config.IndividualGPU.NamePattern)
		if err != nil {
			return fmt.Errorf("error getting resource name for GPU %d: %v", i, err)
		}

		// Build device info
		index, deviceInfo := b.newGPUDevice(i, gpu)
		dev, err := BuildDevice(index, deviceInfo)
		if err != nil {
			return fmt.Errorf("error building device for GPU %d: %v", i, err)
		}

		// Create devices map with this single GPU
		devices := make(Devices)
		devices[dev.ID] = dev

		// Handle MPS replication if enabled for this GPU
		replicas := 1
		if gpuConfig.MPS != nil && gpuConfig.MPS.Enabled {
			replicas = gpuConfig.MPS.Replicas
			if replicas == 0 {
				replicas = 1
			}
		}
		if replicas > 1 {
			replicatedDevices := make(Devices)
			for replica := 0; replica < replicas; replica++ {
				annotatedID := string(NewAnnotatedID(dev.ID, replica))
				replicatedDevice := Device{
					Device: pluginapi.Device{
						ID:       annotatedID,
						Health:   dev.Health,
						Topology: dev.Topology,
					},
					Paths:             dev.Paths,
					Index:             dev.Index,
					TotalMemory:       dev.TotalMemory,
					ComputeCapability: dev.ComputeCapability,
					Replicas:          replicas,
				}
				replicatedDevices[annotatedID] = &replicatedDevice
			}
			devices = replicatedDevices
		}

		// Create ResourceManager for this GPU
		rm := &nvmlResourceManager{
			resourceManager: resourceManager{
				config:   config,
				resource: resourceName,
				devices:  devices,
			},
			nvml: nvmllib,
		}
		rms = append(rms, rm)

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("error creating individual GPU resource managers: %v", err)
	}

	return rms, nil
}
