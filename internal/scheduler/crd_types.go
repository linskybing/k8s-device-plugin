package scheduler

// Lightweight local representations of the Reservation CRD used by the
// CRD-backed CapacityManager. These types avoid importing controller-runtime
// or k8s codegen so they are safe for incremental development and testing.

// ReservationSpec represents the desired reservation fields.
type ReservationSpec struct {
    PodKey        string `json:"podKey,omitempty"`
    NodeName      string `json:"nodeName,omitempty"`
    NumCards      int    `json:"numCards,omitempty"`
    PercentPerCard int   `json:"percentPerCard,omitempty"`
}

// ReservationStatus represents the observed state of a Reservation.
type ReservationStatus struct {
    Phase          string `json:"phase,omitempty"`
    Message        string `json:"message,omitempty"`
    LastUpdateTime string `json:"lastUpdateTime,omitempty"`
}

// Reservation is a minimal in-repo representation of the CR.
type Reservation struct {
    // TypeMeta / ObjectMeta omitted for simplicity in this scaffold.
    Spec   ReservationSpec   `json:"spec,omitempty"`
    Status ReservationStatus `json:"status,omitempty"`
}

// ReservationList is a minimal list wrapper.
type ReservationList struct {
    Items []Reservation `json:"items"`
}
