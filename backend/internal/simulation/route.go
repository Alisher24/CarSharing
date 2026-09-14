package simulation

import (
	"math"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
)

// RouteID names one declared route. A vehicle is given one of them, and the stored state names it, so
// a vehicle only ever travels a route this build declares.
type RouteID string

// Path is how far along a route a vehicle has travelled from the route's first point, in millimetres.
// Counting in whole millimetres is what makes a journey split into many short steps end at exactly
// the place the same journey covers in one step: nothing is rounded away between two steps.
type Path int64

// Microseconds is a duration counted in the unit every rule here is stated in.
type Microseconds int64

// The speed every vehicle moves at while it is driving, as a fraction of a metre per microsecond.
// Thirty kilometres an hour is exactly twenty-five thirds of a metre a second; keeping the fraction
// rather than its decimal is what keeps the distance of a long journey exact.
const (
	speedNumerator   = 25
	speedDenominator = 3

	metresPerKilometre      = 1_000
	millimetresPerMetre     = 1_000
	millimetresPerKilometre = metresPerKilometre * millimetresPerMetre
)

// travelled is how far a vehicle moves in one window at the declared speed.
func travelled(duration Microseconds) Path {
	return Path(int64(duration) * speedNumerator / speedDenominator)
}

// Route is one fixed closed trajectory. It is a loop, so a vehicle that reaches its end continues
// from the beginning in the same direction and a demonstration never runs out of road.
type Route struct {
	ID RouteID

	// Points are the vertices in order, longitude first. The first point is repeated at the end, so
	// the loop is closed by the declaration rather than by a rule about the last vertex.
	Points []fleet.Position
}

// Routes are every trajectory this build declares. A vehicle names one of them by identifier, and
// this is the only place the geometry of any of them lives.
var Routes = []Route{
	memberRoute("electric-1", fleet.Position{Longitude: 74.5720, Latitude: 42.8590}),
	memberRoute("electric-2", fleet.Position{Longitude: 74.5865, Latitude: 42.8742}),
	memberRoute("electric-3", fleet.Position{Longitude: 74.5990, Latitude: 42.8663}),
	memberRoute("electric-4", fleet.Position{Longitude: 74.6120, Latitude: 42.8815}),
	memberRoute("electric-5", fleet.Position{Longitude: 74.6285, Latitude: 42.8574}),
	memberRoute("gasoline-1", fleet.Position{Longitude: 74.5638, Latitude: 42.8871}),
	memberRoute("gasoline-2", fleet.Position{Longitude: 74.5793, Latitude: 42.8486}),
	memberRoute("gasoline-3", fleet.Position{Longitude: 74.6046, Latitude: 42.8928}),
	memberRoute("gasoline-4", fleet.Position{Longitude: 74.6209, Latitude: 42.8701}),
	memberRoute("gasoline-5", fleet.Position{Longitude: 74.6371, Latitude: 42.8836}),
	memberRoute("diesel-1", fleet.Position{Longitude: 74.5561, Latitude: 42.8628}),
	memberRoute("diesel-2", fleet.Position{Longitude: 74.5904, Latitude: 42.8955}),
	memberRoute("diesel-3", fleet.Position{Longitude: 74.6158, Latitude: 42.8443}),
	memberRoute("diesel-4", fleet.Position{Longitude: 74.6432, Latitude: 42.8759}),
	memberRoute("diesel-5", fleet.Position{Longitude: 74.5682, Latitude: 42.8794}),
	memberRoute("hybrid-1", fleet.Position{Longitude: 74.5837, Latitude: 42.8617}),
	memberRoute("hybrid-2", fleet.Position{Longitude: 74.6091, Latitude: 42.8880}),
	memberRoute("hybrid-3", fleet.Position{Longitude: 74.6246, Latitude: 42.8521}),
	memberRoute("hybrid-4", fleet.Position{Longitude: 74.6398, Latitude: 42.8646}),
	memberRoute("hybrid-5", fleet.Position{Longitude: 74.5599, Latitude: 42.8912}),
	memberRoute("gas-1", fleet.Position{Longitude: 74.5751, Latitude: 42.8703}),
	memberRoute("gas-2", fleet.Position{Longitude: 74.6014, Latitude: 42.8558}),
	memberRoute("gas-3", fleet.Position{Longitude: 74.6177, Latitude: 42.8967}),
	memberRoute("gas-4", fleet.Position{Longitude: 74.6320, Latitude: 42.8688}),
	memberRoute("gas-5", fleet.Position{Longitude: 74.5926, Latitude: 42.8461}),
}

// ScenarioRouteID names the one trajectory that leaves the demonstration service area: the vehicle on
// it drives out of the zone and back in, which is what an ending beyond the boundary is shown with.
const ScenarioRouteID RouteID = "scenario-1"

// The corners of the circuit every ordinary demonstration vehicle keeps to, as offsets from the place
// it is stood at. The whole fleet stands inside the demonstration service area, and a rectangle this
// size stays inside it from every one of those places.
const (
	circuitLongitudeOffset = 0.0030
	circuitLatitudeOffset  = 0.0028
)

// memberRoute declares the circuit of one vehicle: a closed rectangle around the place the
// demonstration stands it, so the route passes through the position its model starts from.
func memberRoute(id RouteID, start fleet.Position) Route {
	return rectangle(id,
		fleet.Position{
			Longitude: start.Longitude - circuitLongitudeOffset,
			Latitude:  start.Latitude - circuitLatitudeOffset,
		},
		fleet.Position{
			Longitude: start.Longitude + circuitLongitudeOffset,
			Latitude:  start.Latitude + circuitLatitudeOffset,
		},
	)
}

// ScenarioRoute is the trajectory that crosses the boundary of the demonstration area. It starts
// where the vehicle it belongs to stands, near the northern edge of the zone, and runs north beyond
// it, so part of every lap is driven outside the area.
func ScenarioRoute() Route {
	start := fleet.Position{Longitude: 74.5561, Latitude: 42.8628}
	return rectangle(ScenarioRouteID,
		fleet.Position{Longitude: start.Longitude - 0.0040, Latitude: start.Latitude - 0.0040},
		fleet.Position{Longitude: start.Longitude + 0.0040, Latitude: 42.9030},
	)
}

// rectangle declares a closed circuit over two opposite corners: east along the southern side, north,
// west along the northern side and south back to where it started.
func rectangle(id RouteID, southWest, northEast fleet.Position) Route {
	return Route{
		ID: id,
		Points: []fleet.Position{
			southWest,
			{Longitude: northEast.Longitude, Latitude: southWest.Latitude},
			northEast,
			{Longitude: southWest.Longitude, Latitude: northEast.Latitude},
			southWest,
		},
	}
}

// RouteOf answers the route a vehicle was given, and reports whether this build declares it.
func RouteOf(id RouteID) (Route, bool) {
	if id == ScenarioRouteID {
		return ScenarioRoute(), true
	}
	for _, route := range Routes {
		if route.ID == id {
			return route, true
		}
	}
	return Route{}, false
}

// segments are the lengths of the route's sides, worked out once for a reading of the whole route.
func (r Route) segments() []float64 {
	lengths := make([]float64, 0, len(r.Points)-1)
	for index := 0; index+1 < len(r.Points); index++ {
		lengths = append(lengths, distance(local(r.Points[index]), local(r.Points[index+1])))
	}
	return lengths
}

// Length is how long one lap of a route is.
func (r Route) Length() Path {
	return toPath(0, sum(r.segments()))
}

// PositionAt is where a vehicle stands after travelling a distance along the route. A distance past
// the end of the lap continues from the beginning, so a journey wraps as many times as it needs.
func (r Route) PositionAt(path Path) fleet.Position {
	lengths := r.segments()
	lap := sum(lengths)
	if lap <= 0 {
		return r.Points[0]
	}
	covered := float64(path) / millimetresPerKilometre
	covered = math.Mod(math.Mod(covered, lap)+lap, lap)
	for index, length := range lengths {
		if covered <= length || index+1 == len(lengths) {
			return along(r.Points[index], r.Points[index+1], covered/length)
		}
		covered -= length
	}
	return r.Points[len(r.Points)-1]
}

// DistanceFrom is how far a place lies from the route, and where along the route the point nearest to
// it falls. A vehicle a demonstration command has moved away from its route drives back to that point
// and follows the route from there.
func (r Route) DistanceFrom(at fleet.Position) (Path, float64) {
	lengths := r.segments()
	best := Path(0)
	nearest := math.Inf(1)
	covered := 0.0
	for index, length := range lengths {
		fraction, offset := ontoSegment(local(at), local(r.Points[index]), local(r.Points[index+1]))
		if offset < nearest {
			nearest = offset
			best = toPath(covered, fraction*length)
		}
		covered += length
	}
	return best, nearest * metresPerKilometre
}

// local is a position in the flat plane the distances of a route are measured in.
type localPoint struct{ x, y float64 }

// The size of a degree where the fleet stands, in the flat approximation every route distance is
// measured in. A degree of longitude is shorter than a degree of latitude by the cosine of the
// parallel, and every vehicle stands within a few kilometres of the same one, so the error of the
// approximation stays far below the width of a lane.
const (
	kilometresPerDegreeLatitude  = 111.19
	kilometresPerDegreeLongitude = kilometresPerDegreeLatitude * 0.7330
)

func local(at fleet.Position) localPoint {
	return localPoint{
		x: at.Longitude * kilometresPerDegreeLongitude,
		y: at.Latitude * kilometresPerDegreeLatitude,
	}
}

// coordinate renders a place of the plane back as a WGS84 position.
func (p localPoint) coordinate() fleet.Position {
	return fleet.Position{
		Longitude: p.x / kilometresPerDegreeLongitude,
		Latitude:  p.y / kilometresPerDegreeLatitude,
	}
}

func sum(lengths []float64) float64 {
	total := 0.0
	for _, length := range lengths {
		total += length
	}
	return total
}

// toPath renders a distance in kilometres, counted from the beginning of the route, as a whole number
// of millimetres.
func toPath(coveredKilometres, withinSegmentKilometres float64) Path {
	return Path(math.Round((coveredKilometres + withinSegmentKilometres) * millimetresPerKilometre))
}

func distance(from, to localPoint) float64 {
	return math.Hypot(to.x-from.x, to.y-from.y)
}

// along is the place a fraction of the way from one position to another.
func along(from, to fleet.Position, fraction float64) fleet.Position {
	start, end := local(from), local(to)
	return localPoint{
		x: start.x + (end.x-start.x)*fraction,
		y: start.y + (end.y-start.y)*fraction,
	}.coordinate()
}

// ontoSegment is where a place projects onto a side of the route, as a fraction of that side's length,
// and how far from the side it lies. A place beyond either end projects onto that end.
func ontoSegment(at, from, to localPoint) (float64, float64) {
	side := localPoint{x: to.x - from.x, y: to.y - from.y}
	squared := side.x*side.x + side.y*side.y
	if squared == 0 {
		return 0, distance(at, from)
	}
	fraction := ((at.x-from.x)*side.x + (at.y-from.y)*side.y) / squared
	switch {
	case fraction < 0:
		fraction = 0
	case fraction > 1:
		fraction = 1
	}
	nearest := localPoint{x: from.x + side.x*fraction, y: from.y + side.y*fraction}
	return fraction, distance(at, nearest)
}
