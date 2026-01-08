package rm

import (
	"testing"

	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
)

func makeAnnotatedIDs(base string, n int) []string {
	res := make([]string, 0, n)
	for i := 0; i < n; i++ {
		res = append(res, string(NewAnnotatedID(base, i)))
	}
	return res
}

func TestCapacityAware_SingleBasePartial(t *testing.T) {
	base := "gpuA"
	annot := makeAnnotatedIDs(base, 10)
	devices := make(Devices)
	// annotated entries represent replicated devices; set Replicas on each
	for _, a := range annot {
		devices[a] = &Device{Device: pluginapi.Device{ID: a}, Replicas: 10}
	}
	r := &resourceManager{devices: devices}

	res, err := r.capacityAwareAlloc(annot, nil, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 10 {
		t.Fatalf("expected 10 allocated devices (best-effort), got %d", len(res))
	}
	// ensure all returned map to base
	for _, id := range res {
		if AnnotatedID(id).GetID() != base {
			t.Fatalf("allocated id %s does not map to base %s", id, base)
		}
	}
}

func TestCapacityAware_PreferSingleBaseFull(t *testing.T) {
	base := "gpuB"
	annot := makeAnnotatedIDs(base, 20)
	devices := make(Devices)
	for _, a := range annot {
		devices[a] = &Device{Device: pluginapi.Device{ID: a}, Replicas: 20}
	}
	r := &resourceManager{devices: devices}

	res, err := r.capacityAwareAlloc(annot, nil, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 20 {
		t.Fatalf("expected 20 allocated devices, got %d", len(res))
	}
	for _, id := range res {
		if AnnotatedID(id).GetID() != base {
			t.Fatalf("allocated id %s does not map to base %s", id, base)
		}
	}
}

func TestCapacityAware_BestFit(t *testing.T) {
	baseA := "gpuC"
	baseB := "gpuD"
	annotA := makeAnnotatedIDs(baseA, 10)
	annotB := makeAnnotatedIDs(baseB, 10)
	devices := make(Devices)
	for _, a := range annotA {
		devices[a] = &Device{Device: pluginapi.Device{ID: a}, Replicas: 10}
	}
	for _, b := range annotB {
		devices[b] = &Device{Device: pluginapi.Device{ID: b}, Replicas: 10}
	}
	r := &resourceManager{devices: devices}

	available := append(annotA, annotB...)
	res, err := r.capacityAwareAlloc(available, nil, 12)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) != 12 {
		t.Fatalf("expected 12 allocated devices, got %d", len(res))
	}
	countA := 0
	countB := 0
	for _, id := range res {
		if AnnotatedID(id).GetID() == baseA {
			countA++
		} else if AnnotatedID(id).GetID() == baseB {
			countB++
		} else {
			t.Fatalf("unknown base in allocated id: %s", id)
		}
	}
	if countA > 10 || countB > 10 {
		t.Fatalf("allocated over capacity: A=%d B=%d", countA, countB)
	}
	if countA+countB != 12 {
		t.Fatalf("allocated total mismatch: %d", countA+countB)
	}
}

func TestCapacityAware_AtLeastOneCard(t *testing.T) {
	base := "gpuE"
	annot := makeAnnotatedIDs(base, 2)
	devices := make(Devices)
	// annotated entries with capacity 2
	for _, a := range annot {
		devices[a] = &Device{Device: pluginapi.Device{ID: a}, Replicas: 2}
	}
	r := &resourceManager{devices: devices}

	res, err := r.capacityAwareAlloc(annot, nil, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res) == 0 {
		t.Fatalf("expected at least one allocated device (best-effort), got 0")
	}
	for _, id := range res {
		if AnnotatedID(id).GetID() != base {
			t.Fatalf("allocated id %s does not map to base %s", id, base)
		}
	}
}

func TestCapacityAware_NoAllocError(t *testing.T) {
	base := "gpuNo"
	// create two annotated IDs but set base capacity equal to required count
	annot := makeAnnotatedIDs(base, 2)
	devices := make(Devices)
	// base device with capacity 1
	devices[base] = &Device{Device: pluginapi.Device{ID: base}, Replicas: 1}
	// annotated entries
	for _, a := range annot {
		devices[a] = &Device{Device: pluginapi.Device{ID: a}}
	}

	r := &resourceManager{devices: devices}

	// required already consumes the single capacity slot
	required := []string{annot[0]}
	available := []string{annot[1]}

	_, err := r.capacityAwareAlloc(available, required, 2)
	if err == nil {
		t.Fatalf("expected error when no allocatable slots are available, got nil")
	}
}
