package scheduler

// NodeReservation types shared between scheduler and controller.
type NodeReservationSpec struct {
	NodeName string `json:"nodeName,omitempty"`
}

type DeviceReservation struct {
	PodKey  string `json:"podKey,omitempty"`
	Percent int    `json:"percent,omitempty"`
}

type DeviceStatus struct {
	ID                   string              `json:"id,omitempty"`
	Reservations         []DeviceReservation `json:"reservations,omitempty"`
	TotalReservedPercent int                 `json:"totalReservedPercent,omitempty"`
}

type NodeReservationStatus struct {
	Devices     []DeviceStatus `json:"devices,omitempty"`
	LastUpdated string         `json:"lastUpdated,omitempty"`
}

type NodeReservation struct {
	Spec   NodeReservationSpec   `json:"spec,omitempty"`
	Status NodeReservationStatus `json:"status,omitempty"`
}
