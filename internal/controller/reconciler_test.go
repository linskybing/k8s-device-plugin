//go:build controller
// +build controller

package controller

import (
	"context"
	"testing"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/NVIDIA/k8s-device-plugin/internal/scheduler"
)

func TestReconciler_MergeSuccess(t *testing.T) {
	// Build reservation unstructured
	resObj := &unstructured.Unstructured{}
	resObj.SetGroupVersionKind(schema.GroupVersionKind{Group: "mps.nvidia.com", Version: "v1", Kind: "Reservation"})
	resObj.SetName("res1")
	resObj.SetNamespace("ns")
	_ = unstructured.SetNestedField(resObj.Object, "ns/p", "spec", "podKey")
	_ = unstructured.SetNestedField(resObj.Object, "nodeA", "spec", "nodeName")
	_ = unstructured.SetNestedField(resObj.Object, int64(1), "spec", "numCards")
	_ = unstructured.SetNestedField(resObj.Object, int64(30), "spec", "percentPerCard")

	// Build NodeReservation unstructured with one device
	schedNR := scheduler.NodeReservation{}
	schedNR.Spec.NodeName = "nodeA"
	schedNR.Status.Devices = []scheduler.DeviceStatus{{ID: "GPU-0", Reservations: nil, TotalReservedPercent: 0}}
	nrObj := &unstructured.Unstructured{}
	nrObj.SetGroupVersionKind(schema.GroupVersionKind{Group: "mps.nvidia.com", Version: "v1", Kind: "NodeReservation"})
	nrObj.Object = map[string]interface{}{
		"apiVersion": "mps.nvidia.com/v1",
		"kind":       "NodeReservation",
		"metadata": map[string]interface{}{
			"name": "node-nodeA",
		},
		"status": map[string]interface{}{
			"devices": []interface{}{
				map[string]interface{}{
					"id":                   "GPU-0",
					"reservations":         []interface{}{},
					"totalReservedPercent": int64(0),
				},
			},
		},
	}

	cl := fake.NewClientBuilder().WithObjects(resObj, nrObj).Build()
	// Sanity-check that fake client contains NodeReservation
	chk := &unstructured.Unstructured{}
	chk.SetGroupVersionKind(schema.GroupVersionKind{Group: "mps.nvidia.com", Version: "v1", Kind: "NodeReservation"})
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "node-nodeA"}, chk); err != nil {
		t.Fatalf("setup failed: fake client missing NodeReservation: %v", err)
	}
	r := &NodeReservationReconciler{Client: cl}

	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: "ns", Name: "res1"}})
	if err != nil {
		t.Fatalf("reconcile returned error: %v", err)
	}

	// Reload NodeReservation and check status
	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(schema.GroupVersionKind{Group: "mps.nvidia.com", Version: "v1", Kind: "NodeReservation"})
	if err := cl.Get(context.Background(), types.NamespacedName{Name: "node-nodeA"}, got); err != nil {
		t.Fatalf("failed to get node reservation: %v", err)
	}

	// Inspect status.devices[0].totalReservedPercent
	devices, found, _ := unstructured.NestedSlice(got.Object, "status", "devices")
	if !found || len(devices) == 0 {
		t.Fatalf("no devices found in node reservation status")
	}
	dev0 := devices[0].(map[string]interface{})
	var trp int
	switch v := dev0["totalReservedPercent"].(type) {
	case float64:
		trp = int(v)
	case int64:
		trp = int(v)
	default:
		t.Fatalf("unexpected type for totalReservedPercent: %T", v)
	}
	if trp != 30 {
		t.Fatalf("expected totalReservedPercent 30, got %d", trp)
	}
}
