/*
 * Copyright (c) 2022, NVIDIA CORPORATION.  All rights reserved.
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
	"sort"
)

// distributedAlloc returns a list of devices such that any replicated
// devices are distributed across all replicated GPUs equally. It takes into
// account already allocated replicas to ensure a proper balance across them.
func (r *resourceManager) distributedAlloc(available, required []string, size int) ([]string, error) {
	// Get the set of candidate devices as the difference between available and required.
	candidates := r.devices.Subset(available).Difference(r.devices.Subset(required)).GetIDs()
	needed := size - len(required)

	if len(candidates) < needed {
		return nil, fmt.Errorf("not enough available devices to satisfy allocation")
	}

	// For each candidate device, build a mapping of (stripped) device ID to
	// total / available replicas for that device.
	replicas := make(map[string]*struct{ total, available int })
	for _, c := range candidates {
		id := AnnotatedID(c).GetID()
		if _, exists := replicas[id]; !exists {
			replicas[id] = &struct{ total, available int }{}
		}
		replicas[id].available++
	}
	for d := range r.devices {
		id := AnnotatedID(d).GetID()
		if _, exists := replicas[id]; !exists {
			continue
		}
		replicas[id].total++
	}

	// Grab the set of 'needed' devices one-by-one from the candidates list.
	// Sort once before the loop; after each pick, do a single O(n) pass to
	// bubble up any candidates that share the same base device ID (their
	// allocated count just increased, so they rank higher now).
	lessFunc := func(a, b string) bool {
		aid := AnnotatedID(a).GetID()
		bid := AnnotatedID(b).GetID()
		adiff := replicas[aid].total - replicas[aid].available
		bdiff := replicas[bid].total - replicas[bid].available
		if adiff != bdiff {
			return adiff > bdiff
		}
		return aid < bid
	}
	sort.Slice(candidates, func(i, j int) bool {
		return lessFunc(candidates[i], candidates[j])
	})

	var devices []string
	for i := 0; i < needed; i++ {
		pick := candidates[0]
		pickedID := AnnotatedID(pick).GetID()
		replicas[pickedID].available--
		devices = append(devices, pick)
		candidates = candidates[1:]

		// Bubble up candidates whose base ID matches the picked device:
		// their idiff just increased, so they may need to move forward.
		for j := 0; j < len(candidates); j++ {
			if AnnotatedID(candidates[j]).GetID() != pickedID {
				continue
			}
			// Insertion: move candidates[j] forward while it beats its predecessor.
			for j > 0 && lessFunc(candidates[j], candidates[j-1]) {
				candidates[j], candidates[j-1] = candidates[j-1], candidates[j]
				j--
			}
		}
	}

	// Add the set of required devices to this list and return it.
	devices = append(required, devices...)

	return devices, nil
}
