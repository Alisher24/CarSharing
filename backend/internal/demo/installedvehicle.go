package demo

import "github.com/Alisher24/CarSharing/backend/internal/fleet"

func (v Vehicle) installed() fleet.InstalledVehicle {
	return fleet.InstalledVehicle{
		ID:             v.ID,
		Model:          v.Model,
		PowertrainType: v.PowertrainType,
		Connected:      v.Connected,
		Reporting:      v.Reporting,
		RouteID:        string(v.RouteID),
		Position:       v.Position,
		Sources:        v.Sources,
		ConfirmedAgo:   v.confirmedAgo(),
	}
}
