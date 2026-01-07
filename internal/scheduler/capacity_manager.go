package scheduler

import (
	"sync"
)

// CapacityManager provides an abstraction for cluster-wide reservation
// operations. This minimal in-memory implementation is a noop-style
// manager suitable for local testing and for evolving to a CRD-backed
// implementation later.
type CapacityManager interface {
	// Reserve attempts to create a reservation for podKey on nodeName for the requested
	// number of cards with the given percent per-card. Returns error on rejection.
	Reserve(podKey, nodeName string, numCards, percent int) error
	// Release removes a previous reservation.
	Release(podKey, nodeName string) error
}

// InMemoryCapacityManager is a trivial in-memory implementation that
// records reservations but performs no capacity enforcement.
type InMemoryCapacityManager struct {
	mu           sync.Mutex
	reservations map[string]map[string]reservation // nodeName -> podKey -> reservation
}

type reservation struct {
	NumCards int
	Percent  int
}

// NewInMemoryCapacityManager constructs a new in-memory manager.
func NewInMemoryCapacityManager() *InMemoryCapacityManager {
	return &InMemoryCapacityManager{reservations: make(map[string]map[string]reservation)}
}

func (m *InMemoryCapacityManager) Reserve(podKey, nodeName string, numCards, percent int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.reservations[nodeName]; !ok {
		m.reservations[nodeName] = make(map[string]reservation)
	}
	m.reservations[nodeName][podKey] = reservation{NumCards: numCards, Percent: percent}
	return nil
}

func (m *InMemoryCapacityManager) Release(podKey, nodeName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if nm, ok := m.reservations[nodeName]; ok {
		delete(nm, podKey)
		if len(nm) == 0 {
			delete(m.reservations, nodeName)
		}
	}
	return nil
}
