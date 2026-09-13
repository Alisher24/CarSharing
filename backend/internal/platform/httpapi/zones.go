package httpapi

import (
	"context"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/zones"
)

// ZoneReader is the read model the zone operation answers from.
type ZoneReader interface {
	Current(ctx context.Context) ([]zones.Zone, error)
}

// serviceZones answers the zone operation. A zone that could not be read is reported as a failure
// rather than replaced by a boundary this process made up.
type serviceZones struct{ reader ZoneReader }

func (z serviceZones) GetZones(
	ctx context.Context, _ servedapi.GetZonesRequestObject,
) (servedapi.GetZonesResponseObject, error) {
	current, err := z.reader.Current(ctx)
	if err != nil {
		return servedapi.GetZones503JSONResponse{Body: serviceUnavailable(ctx)}, nil
	}
	collection := servedapi.ZoneCollection{Items: make([]servedapi.Zone, 0, len(current))}
	for _, zone := range current {
		published, err := publishedZone(zone)
		if err != nil {
			return nil, err
		}
		collection.Items = append(collection.Items, published)
	}
	return servedapi.GetZones200JSONResponse{Body: collection}, nil
}

func publishedZone(zone zones.Zone) (servedapi.Zone, error) {
	var geometry servedapi.Geometry
	if err := geometry.UnmarshalJSON(zone.Area); err != nil {
		return servedapi.Zone{}, err
	}
	return servedapi.Zone{
		Id:       zone.ID,
		Name:     zone.Name,
		Geometry: geometry,
		Version:  exactInteger(zone.Version),
	}, nil
}
