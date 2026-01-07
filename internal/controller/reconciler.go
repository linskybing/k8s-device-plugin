//go:build controller
// +build controller

package controller

import (
	"context"
	"encoding/json"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	"github.com/NVIDIA/k8s-device-plugin/internal/scheduler"
)

// NodeReservationReconciler reconciles Reservation -> NodeReservation aggregation.
type NodeReservationReconciler struct {
	client.Client
}

func NewReconciler(mgr ctrl.Manager) error {
	// wire up watches for Reservation and NodeReservation
	return nil
}

func (r *NodeReservationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	// Load Reservation (unstructured)
	reservationGvk := schema.GroupVersionKind{Group: "mps.nvidia.com", Version: "v1", Kind: "Reservation"}
	resObj := &unstructured.Unstructured{}
	resObj.SetGroupVersionKind(reservationGvk)
	if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, resObj); err != nil {
		// NotFound or other errors will be returned to let controller-runtime handle retries
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Convert Reservation to scheduler.Reservation
	resJSON, err := json.Marshal(resObj.Object)
	if err != nil {
		return ctrl.Result{}, err
	}
	var schedRes scheduler.Reservation
	if err := json.Unmarshal(resJSON, &schedRes); err != nil {
		// best-effort: update status with parse error
		_ = unstructured.SetNestedField(resObj.Object, fmt.Sprintf("failed to parse reservation: %v", err), "status", "message")
		_ = unstructured.SetNestedField(resObj.Object, "Rejected", "status", "phase")
		_ = r.Update(ctx, resObj)
		return ctrl.Result{}, nil
	}

	nodeName := schedRes.Spec.NodeName
	if nodeName == "" {
		// nothing to do
		return ctrl.Result{}, nil
	}

	// Load or create NodeReservation (cluster-scoped)
	nrGvk := schema.GroupVersionKind{Group: "mps.nvidia.com", Version: "v1", Kind: "NodeReservation"}
	nrName := fmt.Sprintf("node-%s", nodeName)
	nrObj := &unstructured.Unstructured{}
	nrObj.SetGroupVersionKind(nrGvk)
	err = r.Get(ctx, types.NamespacedName{Name: nrName}, nrObj)
	if err != nil {
		// If not found, attempt to create a baseline NodeReservation and apply the reservation
		if client.IgnoreNotFound(err) == nil {
			// NotFound -> construct a basic NodeReservation with at least NumCards devices
			base := scheduler.NodeReservation{}
			base.Spec.NodeName = nodeName
			devCount := schedRes.Spec.NumCards
			if devCount <= 0 {
				devCount = 1
			}
			for i := 0; i < devCount; i++ {
				base.Status.Devices = append(base.Status.Devices, scheduler.DeviceStatus{ID: fmt.Sprintf("GPU-%d", i), Reservations: nil, TotalReservedPercent: 0})
			}

			updatedBase, mergeErr := scheduler.MergeReservationIntoNodeState(base, schedRes)
			if mergeErr != nil {
				_ = unstructured.SetNestedField(resObj.Object, mergeErr.Error(), "status", "message")
				_ = unstructured.SetNestedField(resObj.Object, "Rejected", "status", "phase")
				_ = r.Status().Update(ctx, resObj)
				return ctrl.Result{}, nil
			}

			// marshal updatedBase into an unstructured and create
			var nrMap map[string]interface{}
			if b, err := json.Marshal(updatedBase); err == nil {
				_ = json.Unmarshal(b, &nrMap)
			} else {
				return ctrl.Result{}, err
			}
			// ensure metadata and apiVersion/kind
			nrMap["apiVersion"] = "mps.nvidia.com/v1"
			nrMap["kind"] = "NodeReservation"
			meta, _ := nrMap["metadata"].(map[string]interface{})
			if meta == nil {
				meta = map[string]interface{}{}
				nrMap["metadata"] = meta
			}
			meta["name"] = nrName

			nrObj.Object = nrMap
			nrObj.SetGroupVersionKind(nrGvk)
			if err := r.Create(ctx, nrObj); err != nil {
				return ctrl.Result{}, err
			}
		} else {
			// Other errors: set Pending and return
			_ = unstructured.SetNestedField(resObj.Object, "nodereservation get error", "status", "message")
			_ = unstructured.SetNestedField(resObj.Object, "Pending", "status", "phase")
			_ = r.Status().Update(ctx, resObj)
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	// Convert NodeReservation to scheduler.NodeReservation
	nrJSON, err := json.Marshal(nrObj.Object)
	if err != nil {
		return ctrl.Result{}, err
	}
	var schedNR scheduler.NodeReservation
	if err := json.Unmarshal(nrJSON, &schedNR); err != nil {
		return ctrl.Result{}, err
	}

	// Attempt merge
	updatedNR, mergeErr := scheduler.MergeReservationIntoNodeState(schedNR, schedRes)
	if mergeErr != nil {
		// update reservation status
		_ = unstructured.SetNestedField(resObj.Object, mergeErr.Error(), "status", "message")
		_ = unstructured.SetNestedField(resObj.Object, "Rejected", "status", "phase")
		_ = r.Status().Update(ctx, resObj)
		return ctrl.Result{}, nil
	}

	// Write back NodeReservation status
	var nrMap map[string]interface{}
	if b, err := json.Marshal(updatedNR); err == nil {
		_ = json.Unmarshal(b, &nrMap)
		// set the status subfield of the existing object
		_ = unstructured.SetNestedField(nrObj.Object, nrMap["status"], "status")
		if err := r.Update(ctx, nrObj); err != nil {
			return ctrl.Result{}, err
		}
	}

	// update reservation status to Accepted
	_ = unstructured.SetNestedField(resObj.Object, "Accepted", "status", "phase")
	_ = unstructured.SetNestedField(resObj.Object, "bound to node", "status", "message")
	if err := r.Update(ctx, resObj); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}
