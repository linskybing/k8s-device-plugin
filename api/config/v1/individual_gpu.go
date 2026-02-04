/*
 * Copyright (c) 2024, NVIDIA CORPORATION.  All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package v1

import "fmt"

// IndividualGPUConfig defines configuration for exposing GPUs as individual resources
type IndividualGPUConfig struct {
	// Enabled determines whether to expose GPUs individually instead of pooled
	Enabled bool `json:"enabled" yaml:"enabled"`
	// GPUConfigs contains per-GPU configuration mapped by GPU index
	GPUConfigs []GPUConfig `json:"gpuConfigs,omitempty" yaml:"gpuConfigs,omitempty"`
	// NamePattern defines the pattern for naming GPU resources (e.g., "gpu%d")
	// %d will be replaced with the GPU index
	NamePattern string `json:"namePattern,omitempty" yaml:"namePattern,omitempty"`
}

// GPUConfig defines configuration for a specific GPU
type GPUConfig struct {
	// Index is the GPU index (0, 1, 2, etc.)
	Index int `json:"index" yaml:"index"`
	// Name is the custom resource name for this GPU (e.g., "gpu0", "gpu1")
	// If empty, will use NamePattern from IndividualGPUConfig
	Name string `json:"name,omitempty" yaml:"name,omitempty"`
	// UUID optionally identifies the GPU by UUID instead of index
	UUID string `json:"uuid,omitempty" yaml:"uuid,omitempty"`
	// MPS configuration for this specific GPU
	MPS *GPUMPSConfig `json:"mps,omitempty" yaml:"mps,omitempty"`
}

// GPUMPSConfig defines MPS-specific configuration for a GPU
type GPUMPSConfig struct {
	// Enabled determines if MPS is enabled for this GPU
	Enabled bool `json:"enabled" yaml:"enabled"`
	// ActiveThreadLimit sets the maximum number of active CUDA contexts per client
	// This is enforced via CUDA_MPS_ACTIVE_THREAD_LIMIT
	ActiveThreadLimit int `json:"activeThreadLimit,omitempty" yaml:"activeThreadLimit,omitempty"`
	// ActiveThreadPercentage sets the percentage of active threads (alternative to ActiveThreadLimit)
	// This is enforced via set_default_active_thread_percentage
	ActiveThreadPercentage int `json:"activeThreadPercentage,omitempty" yaml:"activeThreadPercentage,omitempty"`
	// PinnedMemoryLimit sets the pinned memory limit in MB for each MPS client
	PinnedMemoryLimit int `json:"pinnedMemoryLimit,omitempty" yaml:"pinnedMemoryLimit,omitempty"`
	// Replicas defines how many replicas to create for MPS sharing
	// Default is 1 (no sharing)
	Replicas int `json:"replicas,omitempty" yaml:"replicas,omitempty"`
	// EnableMemoryLimit controls whether to enforce proportional memory allocation
	// When false (default): Each replica can use full GPU memory (no limit)
	// When true: Memory is divided proportionally based on replica count
	EnableMemoryLimit bool `json:"enableMemoryLimit,omitempty" yaml:"enableMemoryLimit,omitempty"`
}

// GetResourceName returns the resource name for the GPU config
func (g *GPUConfig) GetResourceName(pattern string) (ResourceName, error) {
	name := g.Name
	if name == "" {
		if pattern == "" {
			pattern = "gpu%d"
		}
		name = fmt.Sprintf(pattern, g.Index)
	}
	return NewResourceName(name)
}

// GetDefaultIndividualGPUConfig returns a default configuration with individual GPUs disabled
func GetDefaultIndividualGPUConfig() *IndividualGPUConfig {
	return &IndividualGPUConfig{
		Enabled:     false,
		NamePattern: "gpu%d",
	}
}
