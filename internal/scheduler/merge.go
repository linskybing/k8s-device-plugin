package scheduler

import "fmt"

// MergeReservationIntoNodeState attempts to apply a reservation to a NodeReservation
// and returns the updated NodeReservation. It will return an error if there
// is insufficient per-device capacity to satisfy the reservation.
func MergeReservationIntoNodeState(node NodeReservation, res Reservation) (NodeReservation, error) {
	spec := res.Spec
	if node.Spec.NodeName != "" && spec.NodeName != "" && node.Spec.NodeName != spec.NodeName {
		return node, fmt.Errorf("node mismatch: node reservation for %q vs reservation for %q", node.Spec.NodeName, spec.NodeName)
	}

	candidates := make([]int, 0, len(node.Status.Devices))
	for i, d := range node.Status.Devices {
		if d.TotalReservedPercent+spec.PercentPerCard <= 100 {
			candidates = append(candidates, i)
		}
	}

	if len(candidates) < spec.NumCards {
		return node, fmt.Errorf("insufficient capacity: need %d devices, have %d candidates", spec.NumCards, len(candidates))
	}

	pick := candidates[:spec.NumCards]
	for _, idx := range pick {
		node.Status.Devices[idx].Reservations = append(node.Status.Devices[idx].Reservations, DeviceReservation{
			PodKey:  spec.PodKey,
			Percent: spec.PercentPerCard,
		})
		node.Status.Devices[idx].TotalReservedPercent += spec.PercentPerCard
	}

	return node, nil
}

// RemoveReservationFromNodeState removes a reservation's entries from a NodeReservation.
func RemoveReservationFromNodeState(node NodeReservation, res Reservation) (NodeReservation, error) {
	spec := res.Spec
	for i := range node.Status.Devices {
		newRes := node.Status.Devices[i].Reservations[:0]
		for _, r := range node.Status.Devices[i].Reservations {
			if r.PodKey == spec.PodKey {
				node.Status.Devices[i].TotalReservedPercent -= r.Percent
				continue
			}
			newRes = append(newRes, r)
		}
		node.Status.Devices[i].Reservations = newRes
		if node.Status.Devices[i].TotalReservedPercent < 0 {
			node.Status.Devices[i].TotalReservedPercent = 0
		}
	}
	return node, nil
}
