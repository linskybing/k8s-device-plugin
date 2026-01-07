package controller

import (
	"fmt"
	"testing"

	"github.com/NVIDIA/k8s-device-plugin/internal/scheduler"
)

func makeNodeWithDevices(count int, reserved int) NodeReservation {
	n := NodeReservation{}
	n.Spec.NodeName = "nodeA"
	for i := 0; i < count; i++ {
		n.Status.Devices = append(n.Status.Devices, DeviceStatus{
			ID:                   fmt.Sprintf("GPU-%d", i),
			Reservations:         nil,
			TotalReservedPercent: reserved,
		})
	}
	return n
}

func TestMergeReservation_Success(t *testing.T) {
	node := makeNodeWithDevices(4, 10)
	res := scheduler.Reservation{}
	res.Spec = scheduler.ReservationSpec{PodKey: "ns/p", NodeName: "nodeA", NumCards: 2, PercentPerCard: 20}

	updated, err := MergeReservationIntoNodeState(node, res)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Expect two devices to have the reservation appended
	got := 0
	for _, d := range updated.Status.Devices {
		for _, r := range d.Reservations {
			if r.PodKey == res.Spec.PodKey && r.Percent == res.Spec.PercentPerCard {
				got++
			}
		}
	}
	if got != 2 {
		t.Fatalf("expected 2 reservations applied, got %d", got)
	}
}

func TestMergeReservation_Insufficient(t *testing.T) {
	// devices already near capacity
	node := makeNodeWithDevices(2, 90)
	res := scheduler.Reservation{}
	res.Spec = scheduler.ReservationSpec{PodKey: "ns/p", NodeName: "nodeA", NumCards: 2, PercentPerCard: 20}

	_, err := MergeReservationIntoNodeState(node, res)
	if err == nil {
		t.Fatalf("expected insufficient capacity error")
	}
}

func TestRemoveReservation(t *testing.T) {
	node := makeNodeWithDevices(3, 0)
	res := scheduler.Reservation{}
	res.Spec = scheduler.ReservationSpec{PodKey: "ns/p", NodeName: "nodeA", NumCards: 2, PercentPerCard: 30}

	updated, err := MergeReservationIntoNodeState(node, res)
	if err != nil {
		t.Fatalf("unexpected merge error: %v", err)
	}

	after, err := RemoveReservationFromNodeState(updated, res)
	if err != nil {
		t.Fatalf("unexpected remove error: %v", err)
	}

	// Ensure no reservations remain for the pod
	for _, d := range after.Status.Devices {
		for _, r := range d.Reservations {
			if r.PodKey == res.Spec.PodKey {
				t.Fatalf("found reservation still present after removal")
			}
		}
	}
}
