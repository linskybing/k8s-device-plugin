package scheduler

import (
	"errors"
)

// CRDCapacityManager is a scaffold intended to be replaced by a real
// implementation that persists reservations as Kubernetes CRs. The methods
// currently return a not-implemented error so callers can be wired incrementally.
type CRDCapacityManager struct{}

// compile-time check
var _ CapacityManager = &CRDCapacityManager{}

// NewCRDCapacityManager constructs a new CRD-backed CapacityManager.
// TODO: accept client/configuration to talk to API server.
func NewCRDCapacityManager() *CRDCapacityManager {
	return &CRDCapacityManager{}
}

// Reserve will create or update a reservation CR for podKey on nodeName.
// Currently unimplemented.
func (c *CRDCapacityManager) Reserve(podKey, nodeName string, numCards, percent int) error {
	return errors.New("CRD-backed CapacityManager Reserve not implemented")
}

// Release will remove or update the reservation CR for podKey on nodeName.
// Currently unimplemented.
func (c *CRDCapacityManager) Release(podKey, nodeName string) error {
	return errors.New("CRD-backed CapacityManager Release not implemented")
}
