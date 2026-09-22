package simulation_test

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"testing"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
	"github.com/Alisher24/CarSharing/backend/internal/simulation"
)

// The service area the demonstration installs, which every ring but the scenario one keeps to. This
// package cannot read it from the package that declares it without depending on the whole
// demonstration, so it is stated here as the rectangle the ending rule is applied against.
const (
	zoneWestLongitude = 74.55
	zoneEastLongitude = 74.65
	zoneSouthLatitude = 42.84
	zoneNorthLatitude = 42.90
)

// How far a degree of latitude covers where the fleet stands, which the checks below measure in with
// the same value the model measures distances with.
const metresPerDegreeLatitude = 111_190

func TestEveryRingIsClosed(t *testing.T) {
	for _, route := range simulation.Routes() {
		if len(route.Points) < 4 {
			t.Errorf("%s is drawn with %d points, and a ring has at least three sides", route.ID, len(route.Points))
			continue
		}
		first, last := route.Points[0], route.Points[len(route.Points)-1]
		if first != last {
			t.Errorf("%s starts at %v and ends at %v, so one lap of it is not a loop", route.ID, first, last)
		}
	}
}

// A vehicle is placed by the ring it drives rather than by a coordinate kept beside it, and the place
// it is placed at is the first vertex of that ring. A start point that drifted from the ring would
// show itself as a vehicle jumping to the nearest point of its circuit on its first movement, which
// is exactly what the declaration exists to prevent.
func TestEveryRingStartsWhereItsFirstSideDoes(t *testing.T) {
	for _, route := range simulation.Routes() {
		if len(route.Points) < 2 {
			t.Errorf("%s has no sides at all", route.ID)
			continue
		}
		if _, distance := route.DistanceFrom(route.Start()); distance > 1.0 {
			t.Errorf("%s starts at %v, which lies %v metres from it", route.ID, route.Start(), distance)
		}
		if path, _ := route.DistanceFrom(route.Start()); path != 0 {
			t.Errorf("%s starts at %v, which is %d millimetres along it rather than at its beginning",
				route.ID, route.Start(), path)
		}
	}
}

// A side shorter than a block is not a street: it is a corner cut across one, which is what a ring
// assembled from guessed geometry looks like when a person follows it on the map.
func TestNoRingHasASideShorterThanABlock(t *testing.T) {
	for _, route := range simulation.Routes() {
		for index, metres := range sideLengths(route) {
			if metres < simulation.ShortestSideMetres {
				t.Errorf("%s follows %s for only %.0f metres", route.ID, route.Streets[index], metres)
			}
		}
	}
}

// The whole fleet drives inside the demonstration area, so a vehicle on any of these rings is a
// vehicle a rental may be ended on. The scenario ring is the one exception, and it is checked for the
// crossing it exists to produce.
func TestEveryRingButTheScenarioOneKeepsToTheServiceArea(t *testing.T) {
	for _, route := range simulation.Routes() {
		if route.ID == simulation.ScenarioRouteID {
			continue
		}
		for _, point := range route.Points {
			if !insideServiceArea(point) {
				t.Errorf("%s passes through %v, %v: outside the service area",
					route.ID, point.Longitude, point.Latitude)
			}
		}
	}
}

func TestTheScenarioRingLeavesTheServiceAreaAndComesBack(t *testing.T) {
	scenario, declared := simulation.RouteOf(simulation.ScenarioRouteID)
	if !declared {
		t.Fatalf("this build declares no %s", simulation.ScenarioRouteID)
	}

	outside, inside := 0, 0
	for _, point := range scenario.Points {
		if insideServiceArea(point) {
			inside++
			continue
		}
		outside++
	}
	if outside == 0 {
		t.Error("the scenario ring never leaves the service area, so no ride can be ended beyond it")
	}
	if inside == 0 {
		t.Error("the scenario ring never returns to the service area, so its vehicle never comes back")
	}
}

// A ring is named by the streets it follows, which is how a person checks it on the map by eye: the
// name states what the sides are supposed to be, and a side named after a street this build knows no
// crossing of is a side nobody can look up.
func TestEveryRingIsNamedByTheStreetsItFollows(t *testing.T) {
	known := map[string]bool{}
	for _, street := range simulation.NamedStreets() {
		known[street] = true
	}

	for _, route := range simulation.Routes() {
		if len(route.Streets) != len(route.Points)-1 {
			t.Errorf("%s has %d sides and names %d streets", route.ID, len(route.Points)-1, len(route.Streets))
		}
		for index, street := range route.Streets {
			if !known[street] {
				t.Errorf("%s leaves side %d named %q, which is no street of the centre", route.ID, index+1, street)
			}
		}
		if route.Name() == "" {
			t.Errorf("%s has no name of its own", route.ID)
		}
	}
}

func TestEveryRingIdentifierIsDeclaredOnce(t *testing.T) {
	seen := map[simulation.RouteID]bool{}
	for _, route := range simulation.Routes() {
		if seen[route.ID] {
			t.Errorf("%s is declared twice", route.ID)
		}
		seen[route.ID] = true
		if found, declared := simulation.RouteOf(route.ID); !declared || found.ID != route.ID {
			t.Errorf("%s cannot be looked up by its own identifier", route.ID)
		}
	}
}

// The declaration is answered as a copy of its own, streets and points included, so a caller that
// rewrites any part of what it was given cannot change the geometry the next reader of the fleet is
// placed by.
func TestARewrittenRouteDoesNotReachTheDeclaration(t *testing.T) {
	declared := simulation.Routes()
	if len(declared) == 0 {
		t.Fatal("this build declares no routes at all")
	}
	handed := simulation.Routes()
	handed[0] = simulation.Route{ID: "rewritten"}
	handed[1].Streets[0] = "rewritten"
	handed[1].Points[0] = fleet.Position{Longitude: 0, Latitude: 0}

	answered := simulation.Routes()
	if answered[0].ID != declared[0].ID {
		t.Errorf("the declaration now answers %q where it declared %q", answered[0].ID, declared[0].ID)
	}
	if answered[1].Streets[0] != declared[1].Streets[0] {
		t.Errorf("the declaration now names the side %q, want %q",
			answered[1].Streets[0], declared[1].Streets[0])
	}
	if answered[1].Points[0] != declared[1].Points[0] {
		t.Errorf("the declaration now stands at %+v, want %+v",
			answered[1].Points[0], declared[1].Points[0])
	}
}

// A position on the ring is answered by the distance along it, and a place beside the ring is
// answered with how far beside it lies — which is what the model uses to decide whether a
// demonstration command placed a vehicle on its route or away from it.
func TestAPlaceOnTheRingIsNoDistanceFromIt(t *testing.T) {
	for _, route := range simulation.Routes() {
		halfway := route.PositionAt(route.Length() / 2)
		if _, metres := route.DistanceFrom(halfway); metres > 1.0 {
			t.Errorf("the middle of %s lies %v metres from it", route.ID, metres)
		}
	}

	route, declared := simulation.RouteOf(simulation.GasRoute1)
	if !declared {
		t.Fatal("this build declares no gas-1")
	}
	// The middle of a ring is inside it and the middle of its bounding box is inside it too, because
	// the box is one ring's own; between the two lies a place that is on neither the ring nor the
	// line the model measures along it.
	inside := route.PositionAt(route.Length() / 2)
	middle := between(inside, centreOf(boundsOf(route)))
	if _, metres := route.DistanceFrom(middle); metres < 1.0 {
		t.Errorf("a place inside %s is read as %v metres from it", route.ID, metres)
	}
}

// between is the place halfway between two places, which the checks use to step off a ring.
func between(from, to fleet.Position) fleet.Position {
	return fleet.Position{
		Longitude: (from.Longitude + to.Longitude) / 2,
		Latitude:  (from.Latitude + to.Latitude) / 2,
	}
}

// boundsOf is the box a ring spans, as its south-western corner and its north-eastern one.
func boundsOf(route simulation.Route) (southWest, northEast fleet.Position) {
	southWest, northEast = route.Points[0], route.Points[0]
	for _, point := range route.Points {
		southWest.Longitude = min(southWest.Longitude, point.Longitude)
		southWest.Latitude = min(southWest.Latitude, point.Latitude)
		northEast.Longitude = max(northEast.Longitude, point.Longitude)
		northEast.Latitude = max(northEast.Latitude, point.Latitude)
	}
	return southWest, northEast
}

// centreOf is the middle of a box.
func centreOf(southWest, northEast fleet.Position) fleet.Position {
	return between(southWest, northEast)
}

// sideLengths is the length of each side of a ring, in metres, measured the way the model measures a
// route: a degree of latitude and a degree of longitude at the parallel the fleet stands on.
func sideLengths(route simulation.Route) []float64 {
	const metresPerDegreeLongitude = metresPerDegreeLatitude * 0.733
	lengths := make([]float64, 0, len(route.Points)-1)
	for index := 0; index+1 < len(route.Points); index++ {
		across := (route.Points[index+1].Longitude - route.Points[index].Longitude) * metresPerDegreeLongitude
		up := (route.Points[index+1].Latitude - route.Points[index].Latitude) * metresPerDegreeLatitude
		lengths = append(lengths, math.Hypot(across, up))
	}
	return lengths
}

func insideServiceArea(point fleet.Position) bool {
	return point.Longitude >= zoneWestLongitude && point.Longitude <= zoneEastLongitude &&
		point.Latitude >= zoneSouthLatitude && point.Latitude <= zoneNorthLatitude
}

// Two vehicles driving identical rings would look like one vehicle drawn twice, and the demonstration
// would be showing fewer circuits than it declares. Rings may share sides — the centre has four
// avenues and a dozen streets, so they do — but not all four of them.
func TestNoTwoRingsAreTheSameCircuit(t *testing.T) {
	seen := map[string]simulation.RouteID{}
	for _, route := range simulation.Routes() {
		key := circuitOf(route)
		if previous, taken := seen[key]; taken {
			t.Errorf("%s drives the same circuit as %s", route.ID, previous)
		}
		seen[key] = route.ID
	}
}

// circuitOf is a ring reduced to the corners it passes through, in an order that does not depend on
// where the vehicle starts or which way round it goes.
func circuitOf(route simulation.Route) string {
	corners := make([]string, 0, len(route.Points)-1)
	for _, point := range route.Points[:len(route.Points)-1] {
		corners = append(corners, fmt.Sprintf("%.5f,%.5f", point.Longitude, point.Latitude))
	}
	sort.Strings(corners)
	return strings.Join(corners, ";")
}
