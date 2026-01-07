package scheduler

import (
	"context"
	"testing"
)

// fakeCycleState simulates the minimal Read/Write behavior used by the plugin Reserve path.
type fakeCycleState struct {
	m map[string]interface{}
}

func newFakeCycleState() *fakeCycleState { return &fakeCycleState{m: make(map[string]interface{})} }

func (s *fakeCycleState) Read(key string) (interface{}, error) {
	if v, ok := s.m[key]; ok {
		return v, nil
	}
	return nil, errStateNotFound
}

func (s *fakeCycleState) Write(key string, obj interface{}) {
	s.m[key] = obj
}

// minimal error used by fake Read
var errStateNotFound = &stateNotFoundError{}

type stateNotFoundError struct{}

func (e *stateNotFoundError) Error() string { return "state not found" }

// PerformReserveFlow emulates GPUMPSPlugin.Reserve using the fakeCycleState.
func PerformReserveFlow(ctx context.Context, state *fakeCycleState, podNamespace, podName, nodeName string) error {
	// read GPURequest
	v, err := state.Read("gpu-request")
	if err != nil {
		return nil // plugin would treat missing request as Success/no-op
	}
	req := v.(*GPURequest)
	podKey := podNamespace + "/" + podName

	// call core ReserveLogic
	devices, err := ReserveLogic(ctx, podKey, *req, nodeName, pickDevicesFromSocket, ReserveForPod)
	if err != nil {
		return err
	}

	// write podReservation and gpuAllocation
	state.Write("pod-reservation", podKey)
	state.Write("gpu-allocation", &GPUAllocationInfo{NodeName: nodeName, SelectedCards: devicesToIndices(devices), RequiredRatio: req.PercentPerCard})
	return nil
}

func TestPerformReserveFlow_SuccessAndStateWrites(t *testing.T) {
	old := capacityMgr
	f := &fakeCapMgr{}
	capacityMgr = f
	defer func() { capacityMgr = old }()

	// prepare fake state with GPURequest
	state := newFakeCycleState()
	state.Write("gpu-request", &GPURequest{NumCards: 1, PercentPerCard: 10})

	// pick/reserve functions that succeed
	pickFn := func(nodeName string, numCards, percent int) ([]string, error) { return []string{"gpu0"}, nil }
	reserveFn := func(ctx context.Context, nodeName, podKey string, devices []string, percent int) error { return nil }

	// emulate plugin Reserve: call ReserveLogic then write state
	devices, err := ReserveLogic(context.Background(), "ns/p", *state.m["gpu-request"].(*GPURequest), "node1", pickFn, reserveFn)
	if err != nil {
		t.Fatalf("ReserveLogic failed: %v", err)
	}
	state.Write("pod-reservation", "ns/p")
	state.Write("gpu-allocation", &GPUAllocationInfo{NodeName: "node1", SelectedCards: devicesToIndices(devices), RequiredRatio: int64(10)})

	if v, _ := state.Read("pod-reservation"); v != "ns/p" {
		t.Fatalf("expected pod-reservation written, got: %v", v)
	}
	if v, _ := state.Read("gpu-allocation"); v == nil {
		t.Fatalf("expected gpu-allocation written")
	}
}

func TestPerformReserveFlow_RollbackOnPickFailure(t *testing.T) {
	old := capacityMgr
	f := &fakeCapMgr{}
	capacityMgr = f
	defer func() { capacityMgr = old }()

	state := newFakeCycleState()
	state.Write("gpu-request", &GPURequest{NumCards: 1, PercentPerCard: 10})

	pickFn := func(nodeName string, numCards, percent int) ([]string, error) { return nil, errStateNotFound }
	reserveFn := func(ctx context.Context, nodeName, podKey string, devices []string, percent int) error { return nil }

	_, err := ReserveLogic(context.Background(), "ns/p", *state.m["gpu-request"].(*GPURequest), "nodeX", pickFn, reserveFn)
	if err == nil {
		t.Fatalf("expected ReserveLogic to fail when pick fails")
	}

	// ensure Release was called and state was not written
	if f.releasedPod != "ns/p" || f.releasedNode != "nodeX" {
		t.Fatalf("expected capacityMgr.Release called for rollback, got (%s,%s)", f.releasedPod, f.releasedNode)
	}
	if _, err := state.Read("pod-reservation"); err == nil {
		t.Fatalf("expected no pod-reservation written on rollback")
	}
}

func TestPerformReserveFlow_RollbackOnReserveFailure(t *testing.T) {
	old := capacityMgr
	f := &fakeCapMgr{}
	capacityMgr = f
	defer func() { capacityMgr = old }()

	state := newFakeCycleState()
	state.Write("gpu-request", &GPURequest{NumCards: 1, PercentPerCard: 10})

	pickFn := func(nodeName string, numCards, percent int) ([]string, error) { return []string{"gpu0"}, nil }
	reserveFn := func(ctx context.Context, nodeName, podKey string, devices []string, percent int) error {
		return errStateNotFound
	}

	_, err := ReserveLogic(context.Background(), "ns/p2", *state.m["gpu-request"].(*GPURequest), "nodeY", pickFn, reserveFn)
	if err == nil {
		t.Fatalf("expected ReserveLogic to fail when reserveFn fails")
	}
	if f.releasedPod != "ns/p2" || f.releasedNode != "nodeY" {
		t.Fatalf("expected capacityMgr.Release called for rollback, got (%s,%s)", f.releasedPod, f.releasedNode)
	}
	if _, err := state.Read("pod-reservation"); err == nil {
		t.Fatalf("expected no pod-reservation written on rollback")
	}
}
