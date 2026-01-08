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
	// Before selecting each candidate, first sort the candidate list using the
	// replicas map above. After sorting, the first element in the list will
	// contain the device with the MOST difference between total and available
	// replications (based on what's already been allocated) - PACK strategy.
	// This ensures we fill up GPUs completely before moving to the next one.
	// Add this device to the list of devices to allocate, remove it from the
	// candidate list, down its available count in the replicas map, and repeat.
	var devices []string
	for i := 0; i < needed; i++ {
		sort.Slice(candidates, func(i, j int) bool {
			iid := AnnotatedID(candidates[i]).GetID()
			jid := AnnotatedID(candidates[j]).GetID()
			idiff := replicas[iid].total - replicas[iid].available
			jdiff := replicas[jid].total - replicas[jid].available
			// Pack strategy: prefer GPUs with MORE allocated replicas (higher diff)
			// This fills up GPUs sequentially instead of spreading across all GPUs
			if idiff != jdiff {
				return idiff > jdiff
			}
			// Tie-breaker: use GPU ID for stable sorting
			return iid < jid
		})
		id := AnnotatedID(candidates[0]).GetID()
		replicas[id].available--
		devices = append(devices, candidates[0])
		candidates = candidates[1:]
	}

	// Add the set of required devices to this list and return it.
	devices = append(required, devices...)

	return devices, nil
}

// capacityAwareAlloc allocates up to `size` devices (including `required`) while
// ensuring no single physical GPU (baseID) is assigned more than its capacity
// (Device.Replicas). Uses a minimum-fit / best-fit strategy to reduce fragmentation:
// 1) prefer a single base that can fully satisfy the remaining need with minimal leftover,
// 2) otherwise iteratively choose the base that, when allocated, leaves the smallest remaining capacity.
func (r *resourceManager) capacityAwareAlloc(available, required []string, size int) ([]string, error) {
	candidates := r.devices.Subset(available).Difference(r.devices.Subset(required)).GetIDs()
	needed := size - len(required)

	if needed <= 0 {
		return required, nil
	}
	if len(candidates) == 0 {
		return required, nil
	}

	// Group available annotated IDs by baseID.
	groups := make(map[string][]string)
	for _, c := range candidates {
		base := AnnotatedID(c).GetID()
		groups[base] = append(groups[base], c)
	}

	// Compute per-base capacity (Device.Replicas when present, default 1).
	capacity := make(map[string]int)
	for base := range groups {
		capacity[base] = 1
		for id, dev := range r.devices {
			if AnnotatedID(id).GetID() == base {
				if dev.Replicas > 1 {
					capacity[base] = dev.Replicas
				} else {
					capacity[base] = 1
				}
				break
			}
		}
	}

	// Count already required per base (so we don't exceed capacity including required).
	reqCount := make(map[string]int)
	for _, id := range required {
		base := AnnotatedID(id).GetID()
		reqCount[base]++
	}

	// Compute total allocatable slots across all bases; if zero, nothing can be
	// allocated (even partially) and we should return an error to prevent
	// kubelet from accepting a useless suggestion.
	totalAllocatable := 0
	for base, list := range groups {
		remCap := capacity[base] - reqCount[base]
		if remCap <= 0 {
			continue
		}
		alloc := len(list)
		if alloc > remCap {
			alloc = remCap
		}
		if alloc > 0 {
			totalAllocatable += alloc
		}
	}
	if totalAllocatable == 0 {
		return nil, fmt.Errorf("unable to allocate any devices to satisfy request")
	}

	// 1) Try to find a single base that can fully satisfy `needed` with minimal leftover.
	var bestBase string
	var bestLeftover int = -1
	for base, list := range groups {
		avail := len(list)
		remainingCap := capacity[base] - reqCount[base]
		if remainingCap <= 0 {
			continue
		}
		maxAlloc := avail
		if maxAlloc > remainingCap {
			maxAlloc = remainingCap
		}
		if maxAlloc >= needed {
			leftover := maxAlloc - needed
			if bestLeftover == -1 || leftover < bestLeftover || (leftover == bestLeftover && base < bestBase) {
				bestLeftover = leftover
				bestBase = base
			}
		}
	}
	if bestBase != "" {
		selected := groups[bestBase][:needed]
		return append(required, selected...), nil
	}

	// 2) Iterative best-fit: pick base that minimizes leftover capacity after allocating
	remaining := needed
	selected := []string{}

	// ensure deterministic order within each group's candidate list
	for base := range groups {
		sort.Strings(groups[base])
	}

	used := make(map[string]int)
	for b, v := range reqCount {
		used[b] = v
	}

	for remaining > 0 {
		type cand struct {
			base           string
			alloc          int
			leftoverAfter  int
			availableSlots int
		}
		var cands []cand
		for base, list := range groups {
			avail := len(list)
			if avail == 0 {
				continue
			}
			remCap := capacity[base] - used[base]
			if remCap <= 0 {
				continue
			}
			alloc := avail
			if alloc > remCap {
				alloc = remCap
			}
			if alloc > remaining {
				alloc = remaining
			}
			if alloc <= 0 {
				continue
			}
			leftover := (capacity[base] - used[base]) - alloc
			cands = append(cands, cand{base: base, alloc: alloc, leftoverAfter: leftover, availableSlots: avail})
		}
		if len(cands) == 0 {
			// nothing more allocatable -> best-effort return
			break
		}
		// choose candidate with minimal leftoverAfter; tie-break by larger alloc, then baseID
		sort.Slice(cands, func(i, j int) bool {
			if cands[i].leftoverAfter != cands[j].leftoverAfter {
				return cands[i].leftoverAfter < cands[j].leftoverAfter
			}
			if cands[i].alloc != cands[j].alloc {
				return cands[i].alloc > cands[j].alloc
			}
			return cands[i].base < cands[j].base
		})
		chosen := cands[0]
		take := chosen.alloc
		// append from groups[base] the first `take` annotated IDs
		selected = append(selected, groups[chosen.base][:take]...)
		// remove taken IDs from group's front
		groups[chosen.base] = groups[chosen.base][take:]
		used[chosen.base] += take
		remaining -= take
	}

	return append(required, selected...), nil
}
