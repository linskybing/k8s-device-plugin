package controller

// Minimal in-repo types for NodeReservation CRD used by the scaffold.
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
