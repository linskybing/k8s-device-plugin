package scheduler

import (
	"context"
	"errors"
	"testing"
)

type fakeCapMgr struct {
	reservedPod  string
	reservedNode string
	reserveErr   error
	releasedPod  string
	releasedNode string
}

func (f *fakeCapMgr) Reserve(podKey, nodeName string, numCards, percent int) error {
	f.reservedPod = podKey
	f.reservedNode = nodeName
	return f.reserveErr
}
func (f *fakeCapMgr) Release(podKey, nodeName string) error {
	f.releasedPod = podKey
	f.releasedNode = nodeName
	return nil
}

func TestReserveLogic_RollbackOnPickFailure(t *testing.T) {
	old := capacityMgr
	f := &fakeCapMgr{}
	capacityMgr = f
	defer func() { capacityMgr = old }()

	req := GPURequest{NumCards: 1, PercentPerCard: 50}

	pickFn := func(nodeName string, numCards, percent int) ([]string, error) {
		return nil, errors.New("no devices")
	}
	reserveFn := func(ctx context.Context, nodeName, podKey string, devices []string, percent int) error {
		return nil
	}

	_, err := ReserveLogic(context.Background(), "ns/pod", req, "nodeA", pickFn, reserveFn)
	if err == nil {
		t.Fatalf("expected error from ReserveLogic when pick fails")
	}
	if f.releasedPod != "ns/pod" || f.releasedNode != "nodeA" {
		t.Fatalf("expected capacityMgr.Release called for rollback, got (%s,%s)", f.releasedPod, f.releasedNode)
	}
}

func TestReserveLogic_RollbackOnReserveFnFailure(t *testing.T) {
	old := capacityMgr
	f := &fakeCapMgr{}
	capacityMgr = f
	defer func() { capacityMgr = old }()

	req := GPURequest{NumCards: 1, PercentPerCard: 20}

	pickFn := func(nodeName string, numCards, percent int) ([]string, error) {
		return []string{"gpu0"}, nil
	}
	reserveFn := func(ctx context.Context, nodeName, podKey string, devices []string, percent int) error {
		return errors.New("reserve failed")
	}

	_, err := ReserveLogic(context.Background(), "ns/pod2", req, "nodeB", pickFn, reserveFn)
	if err == nil {
		t.Fatalf("expected error from ReserveLogic when reserveFn fails")
	}
	if f.releasedPod != "ns/pod2" || f.releasedNode != "nodeB" {
		t.Fatalf("expected capacityMgr.Release called for rollback, got (%s,%s)", f.releasedPod, f.releasedNode)
	}
}

func TestReserveLogic_Success(t *testing.T) {
	old := capacityMgr
	f := &fakeCapMgr{}
	capacityMgr = f
	defer func() { capacityMgr = old }()

	req := GPURequest{NumCards: 1, PercentPerCard: 10}

	pickFn := func(nodeName string, numCards, percent int) ([]string, error) {
		return []string{"gpu0"}, nil
	}
	reserveFn := func(ctx context.Context, nodeName, podKey string, devices []string, percent int) error {
		return nil
	}

	devs, err := ReserveLogic(context.Background(), "ns/pod3", req, "nodeC", pickFn, reserveFn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devs) != 1 || devs[0] != "gpu0" {
		t.Fatalf("unexpected devices: %v", devs)
	}
	if f.releasedPod != "" {
		t.Fatalf("did not expect Release to be called on success")
	}
}
