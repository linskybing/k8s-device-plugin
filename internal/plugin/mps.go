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
	enabled        bool
	memoryLimiting bool
	resourceName   spec.ResourceName
	daemon         *mps.Daemon
	hostRoot       mps.Root
}

// getMPSOptions returns the MPS options specified for the resource manager.
// If MPS is not configured and empty set of options is returned.
func (o *options) getMPSOptions(resourceManager rm.ResourceManager) (mpsOptions, error) {
	if o.config.Sharing.SharingStrategy() != spec.SharingStrategyMPS {
		return mpsOptions{}, nil
	}

	// TODO: It might make sense to pull this logic into a resource manager.
	for _, device := range resourceManager.Devices() {
		if device.IsMigDevice() {
			return mpsOptions{}, errors.New("sharing using MPS is not supported for MIG devices")
		}
	}

	m := mpsOptions{
		enabled:        true,
		memoryLimiting: o.config.Flags.MpsMemoryLimiting != nil && *o.config.Flags.MpsMemoryLimiting,
		resourceName:   resourceManager.Resource(),
		daemon:         mps.NewDaemon(resourceManager, mps.ContainerRoot, o.config.Flags.MpsMemoryLimiting != nil && *o.config.Flags.MpsMemoryLimiting),
		hostRoot:       mps.Root(*o.config.Flags.MpsRoot),
	}
	return m, nil
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

func (m *mpsOptions) updateResponse(response *pluginapi.ContainerAllocateResponse, requestIDs []string) {
	if m == nil || !m.enabled {
		return
	}

	// Count total requested replicas and number of physical GPUs.
	totalRequested := len(requestIDs)
	uniqueGPUs := make(map[string]bool)
	for _, id := range requestIDs {
		uuid := rm.AnnotatedID(id).GetID()
		uniqueGPUs[uuid] = true
	}
	numGPUs := len(uniqueGPUs)

	if numGPUs > 0 {
		// Assume all GPUs have the same number of replicas and total memory.
		// We get these values from the first device we find in the managed set.
		var replicas int
		var totalMemory uint64

		for _, d := range m.daemon.Devices() {
			if d.Replicas > 0 {
				replicas = d.Replicas
				totalMemory = d.TotalMemory
				break
			}
		}

		if replicas > 0 {
			// Calculate average percentage per allocated GPU (ceiling division to avoid underallocation).
			denom := numGPUs * replicas
			percentage := (totalRequested*100 + denom - 1) / denom
			if percentage > 100 {
				percentage = 100
			}
			if percentage > 0 {
				response.Envs["CUDA_MPS_ACTIVE_THREAD_PERCENTAGE"] = fmt.Sprintf("%d", percentage)
			}

			// If memory limiting is enabled, set the memory limit environment variable.
			if m.memoryLimiting && totalMemory > 0 {
				// Calculate memory limit: (TotalMemory * totalRequested) / (numGPUs * replicas)
				// This gives the memory limit per MPS client (container).
				memLimitBytes := (totalMemory * uint64(totalRequested)) / uint64(numGPUs*replicas)
				response.Envs["CUDA_MPS_PINNED_DEVICE_MEM_LIMIT"] = fmt.Sprintf("%dM", memLimitBytes/1024/1024)
			}
		}
	}

	// TODO: We should check that the deviceIDs are shared using MPS.
	response.Envs["CUDA_MPS_PIPE_DIRECTORY"] = m.daemon.PipeDir()

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
}
