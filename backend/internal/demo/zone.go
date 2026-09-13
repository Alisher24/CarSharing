package demo

import "github.com/Alisher24/CarSharing/backend/internal/zones"

// ZoneName says in the interface's own language that this boundary is a demonstration rectangle
// rather than the administrative border of the city.
const ZoneName = "Демонстрационная зона Бишкека"

// The corners of the demonstration rectangle in WGS84. It is an operator's artificial service
// area, not an official city boundary.
const (
	zoneWestLongitude  = 74.55
	zoneEastLongitude  = 74.65
	zoneSouthLatitude  = 42.84
	zoneNorthLatitude  = 42.90
	zoneInitialVersion = 1
)

// Zone is the service area the demonstration installs. The ring is closed and wound so that the
// database stores a valid polygon, and the coordinates are written longitude first.
func Zone() zones.Zone {
	return zones.Zone{
		ID:      resourceID(zoneFamily, 1),
		Name:    ZoneName,
		Version: zoneInitialVersion,
		Area:    []byte(zoneGeoJSON()),
	}
}

func zoneGeoJSON() string {
	corners := [][2]float64{
		{zoneWestLongitude, zoneSouthLatitude},
		{zoneEastLongitude, zoneSouthLatitude},
		{zoneEastLongitude, zoneNorthLatitude},
		{zoneWestLongitude, zoneNorthLatitude},
		{zoneWestLongitude, zoneSouthLatitude},
	}
	ring := ""
	for index, corner := range corners {
		if index > 0 {
			ring += ","
		}
		ring += formatPosition(corner[0], corner[1])
	}
	return `{"type":"Polygon","coordinates":[[` + ring + `]]}`
}
