package demo

import (
	"strconv"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
)

// positionDecimals is how precisely a demonstration position is written out. Five places is about
// a metre, which is as exact as a placed demonstration vehicle can meaningfully be.
const positionDecimals = 5

// formatPosition writes a coordinate pair the way GeoJSON does: longitude first.
func formatPosition(longitude, latitude float64) string {
	return "[" + formatDegrees(longitude) + "," + formatDegrees(latitude) + "]"
}

func formatDegrees(degrees float64) string {
	return strconv.FormatFloat(degrees, 'f', positionDecimals, 64)
}

// standingPositions is where the demonstration vehicles are parked, spread across the service area
// along the streets the offline map draws so the fleet does not appear on an empty lattice. Every
// position is inside the rectangle the zone declares.
var standingPositions = []fleet.Position{
	{Longitude: 74.5720, Latitude: 42.8590},
	{Longitude: 74.5865, Latitude: 42.8742},
	{Longitude: 74.5990, Latitude: 42.8663},
	{Longitude: 74.6120, Latitude: 42.8815},
	{Longitude: 74.6285, Latitude: 42.8574},
	{Longitude: 74.5638, Latitude: 42.8871},
	{Longitude: 74.5793, Latitude: 42.8486},
	{Longitude: 74.6046, Latitude: 42.8928},
	{Longitude: 74.6209, Latitude: 42.8701},
	{Longitude: 74.6371, Latitude: 42.8836},
	{Longitude: 74.5561, Latitude: 42.8628},
	{Longitude: 74.5904, Latitude: 42.8955},
	{Longitude: 74.6158, Latitude: 42.8443},
	{Longitude: 74.6432, Latitude: 42.8759},
	{Longitude: 74.5682, Latitude: 42.8794},
	{Longitude: 74.5837, Latitude: 42.8617},
	{Longitude: 74.6091, Latitude: 42.8880},
	{Longitude: 74.6246, Latitude: 42.8521},
	{Longitude: 74.6398, Latitude: 42.8646},
	{Longitude: 74.5599, Latitude: 42.8912},
	{Longitude: 74.5751, Latitude: 42.8703},
	{Longitude: 74.6014, Latitude: 42.8558},
	{Longitude: 74.6177, Latitude: 42.8967},
	{Longitude: 74.6320, Latitude: 42.8688},
	{Longitude: 74.5926, Latitude: 42.8461},
}
