package simulation

import (
	"math"
	"math/big"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
)

// RouteID names one declared route. A vehicle is given one of them, and the stored state names it, so
// a vehicle only ever travels a route this build declares.
type RouteID string

// Path is how far along a route a vehicle has travelled from the route's first point, in millimetres.
// Counting in whole millimetres is what makes a journey split into many short steps end at exactly
// the place the same journey covers in one step: nothing is rounded away between two steps.
type Path int64

// The speed every vehicle moves at while it is driving: twenty-five thirds of a millimetre a
// microsecond, which is thirty kilometres an hour exactly. The distance of a window is counted from
// nanoseconds, the resolution a moment has, so a journey split into many short steps covers what the
// same journey covers in one call.
const (
	speedNumerator            = 25
	speedDenominator          = 3
	nanosecondsPerMicrosecond = 1_000

	metresPerKilometre      = 1_000
	millimetresPerMetre     = 1_000
	millimetresPerKilometre = metresPerKilometre * millimetresPerMetre
)

// travelled is how far a vehicle moves in one window at the declared speed, in whole millimetres. The
// last millimetre is the one the vehicle is nearest to, so a long window and the short windows that
// make it up agree rather than each losing the part of a millimetre they end in.
func travelled(window time.Duration) Path {
	// The speed over one nanosecond is the fraction of a millimetre the numerator and the denominator
	// state, so the window moves the vehicle that many times its nanoseconds, rounded to the nearest.
	covered := new(big.Int).Mul(big.NewInt(window.Nanoseconds()), big.NewInt(speedNumerator))
	perMillimetre := big.NewInt(speedDenominator * nanosecondsPerMicrosecond)
	covered.Add(covered, new(big.Int).Quo(perMillimetre, big.NewInt(2)))
	return Path(new(big.Int).Quo(covered, perMillimetre).Int64())
}

// ShortestSideMetres is the shortest side a declared ring may have. A ring is followed by eye on a
// real map, and a side shorter than a block is not a street: it is a corner cut across one, which is
// what a ring assembled from guessed coordinates looks like.
const ShortestSideMetres = 300.0

// crossing is a place where two named streets of the centre actually meet, taken from the map rather
// than placed by hand. A ring is a list of these, so every vertex of every ring is a corner a person
// can stand on and every side is a street that is really there.
//
// Both names travel with the point because that is what makes it a corner: a coordinate on one street
// is only a crossing if the other one reaches it, and keeping the two names together is what stops a
// ring from being named after a street it never reached. The name that runs north is held in `north`
// and the one that runs east in `east`, whichever way round the declaration wrote them, so the field
// a name sits in says which way it runs.
type crossing struct {
	north string
	east  string
	at    fleet.Position
}

// axis is the direction a street runs in where the rings are drawn.
type axis int

const (
	runsEastWest axis = iota
	runsNorthSouth
)

// Route is one fixed closed trajectory along named streets. It is a loop, so a vehicle that reaches
// its end continues from the beginning in the same direction and a demonstration never runs out of
// road.
type Route struct {
	ID RouteID

	// Streets are the streets the sides follow, in order. The name of the ring is these, so a person
	// reading the declaration can find the ring on a map and check it by eye.
	Streets []string

	// Points are the vertices in order, longitude first. The first point is repeated at the end, so
	// the loop is closed by the declaration rather than by a rule about the last vertex.
	Points []fleet.Position
}

// Start is where a vehicle given this route stands: the first vertex of the ring, which is a corner of
// two named streets rather than a coordinate kept beside the route.
func (r Route) Start() fleet.Position {
	return r.Points[0]
}

// Name is what the ring is, written as the streets it follows. The demonstration is checked by eye
// against these names, so the ring carries them rather than only the code that drew it.
func (r Route) Name() string {
	return strings.Join(r.Streets, " — ")
}

// The streets of the centre, named once. Every crossing below is a pair of them.
const (
	zhibekZholu   = "улица Жибек Жолу"
	frunze        = "улица Фрунзе"
	moskovskaya   = "улица Московская"
	manasa        = "проспект Манаса"
	turusbekova   = "улица Турусбекова"
	isanova       = "улица Исанова"
	logvinenko    = "улица Логвиненко"
	panfilova     = "улица Панфилова"
	tynystanova   = "улица Тыныстанова"
	abdrakhmanova = "улица Абдрахманова"
	ibraimova     = "улица Ибраимова"
	gogol         = "улица Гоголя"
)

// The corners the rings are built from: each one a crossing of two of the streets above. They are the
// whole vocabulary of the rings, so no ring is drawn at a place that is not one of them.
var (
	// Along улица Жибек Жолу, which runs east across the north of the centre: one crossing per street
	// that reaches it, west to east.
	zhibekAtManasa        = meets(zhibekZholu, manasa, 74.56250, 42.88460)
	zhibekAtTurusbekova   = meets(zhibekZholu, turusbekova, 74.58559, 42.88412)
	zhibekAtIsanova       = meets(zhibekZholu, isanova, 74.59238, 42.88379)
	zhibekAtLogvinenko    = meets(zhibekZholu, logvinenko, 74.59928, 42.88377)
	zhibekAtPanfilova     = meets(zhibekZholu, panfilova, 74.60179, 42.88397)
	zhibekAtTynystanova   = meets(zhibekZholu, tynystanova, 74.60993, 42.88458)
	zhibekAtAbdrakhmanova = meets(zhibekZholu, abdrakhmanova, 74.61175, 42.88496)
	zhibekAtIbraimova     = meets(zhibekZholu, ibraimova, 74.61661, 42.88508)
	zhibekAtGogol         = meets(zhibekZholu, gogol, 74.61874, 42.88523)

	// Along улица Фрунзе, which runs east a little south of it.
	frunzeAtTurusbekova   = meets(frunze, turusbekova, 74.58532, 42.88186)
	frunzeAtAbdrakhmanova = meets(frunze, abdrakhmanova, 74.61175, 42.88061)

	// Along улица Московская, which runs east south of the avenue.
	moskovskayaAtManasa        = meets(moskovskaya, manasa, 74.56250, 42.87030)
	moskovskayaAtTurusbekova   = meets(moskovskaya, turusbekova, 74.58428, 42.87043)
	moskovskayaAtIsanova       = meets(moskovskaya, isanova, 74.59109, 42.87009)
	moskovskayaAtLogvinenko    = meets(moskovskaya, logvinenko, 74.59880, 42.86976)
	moskovskayaAtPanfilova     = meets(moskovskaya, panfilova, 74.60054, 42.86968)
	moskovskayaAtTynystanova   = meets(moskovskaya, tynystanova, 74.60880, 42.86938)
	moskovskayaAtAbdrakhmanova = meets(moskovskaya, abdrakhmanova, 74.61134, 42.86929)
	moskovskayaAtIbraimova     = meets(moskovskaya, ibraimova, 74.61660, 42.86905)
	moskovskayaAtGogol         = meets(moskovskaya, gogol, 74.62065, 42.86893)

	// The two far corners of the scenario ring, which are north of the northern edge of the service
	// area at 42.90. They are stated here rather than taken from a street's crossings because the map
	// has no corner there: the ring leaves the city's grid on purpose, and its northern side is a
	// stretch of street between two of them rather than a crossing.
	manasaFarNorth  = meets(manasa, zhibekZholu, 74.56250, 42.90400)
	isanovaFarNorth = meets(isanova, zhibekZholu, 74.57200, 42.90400)
)

// streetOfCentre is one street of the centre where the rings are drawn: the name a person reads on
// the map, and the direction it runs in.
type streetOfCentre struct {
	name string
	runs axis
}

// streetsOfCentre are every street this build declares a crossing on, with the way each of them runs.
// The direction is taken from the map rather than guessed, by comparing the two ends of a long stretch
// of each street: the avenues of the centre run east and west, and the streets between them run south
// to north.
//
// This is the one declaration of the streets of the centre: NamedStreets answers these names, and
// meets refuses a crossing of a street that is not here or of two that run the same way.
var streetsOfCentre = [...]streetOfCentre{
	{zhibekZholu, runsEastWest},
	{frunze, runsEastWest},
	{moskovskaya, runsEastWest},
	{manasa, runsNorthSouth},
	{turusbekova, runsNorthSouth},
	{isanova, runsNorthSouth},
	{logvinenko, runsNorthSouth},
	{panfilova, runsNorthSouth},
	{tynystanova, runsNorthSouth},
	{abdrakhmanova, runsNorthSouth},
	{ibraimova, runsNorthSouth},
	{gogol, runsNorthSouth},
}

// wayOf answers the direction a street of the centre runs in, and reports whether this build declares
// that street at all.
func wayOf(street string) (axis, bool) {
	for _, declared := range streetsOfCentre {
		if declared.name == street {
			return declared.runs, true
		}
	}
	return 0, false
}

// meets declares the crossing of two streets at one place. A street this build knows no direction for,
// or two streets running the same way, is a crossing that cannot exist: two parallel streets do not
// meet, and one that crosses itself is not a corner.
func meets(one string, other string, longitude, latitude float64) crossing {
	oneWay, oneKnown := wayOf(one)
	otherWay, otherKnown := wayOf(other)
	if !oneKnown || !otherKnown || oneWay == otherWay {
		panic("a crossing is declared of streets that do not cross")
	}

	point := fleet.Position{Longitude: longitude, Latitude: latitude}
	if oneWay == runsNorthSouth {
		return crossing{north: one, east: other, at: point}
	}
	return crossing{north: other, east: one, at: point}
}

// The identifiers of the trajectories this build declares, one per demonstration vehicle. They are
// declared once because both the geometry below and the demonstration fleet that drives them name
// them, so a mistyped identifier is a build error rather than a vehicle standing nowhere.
const (
	ElectricRoute1 RouteID = "electric-1"
	ElectricRoute2 RouteID = "electric-2"
	ElectricRoute3 RouteID = "electric-3"
	ElectricRoute4 RouteID = "electric-4"
	ElectricRoute5 RouteID = "electric-5"

	GasolineRoute1 RouteID = "gasoline-1"
	GasolineRoute2 RouteID = "gasoline-2"
	GasolineRoute3 RouteID = "gasoline-3"
	GasolineRoute4 RouteID = "gasoline-4"
	GasolineRoute5 RouteID = "gasoline-5"

	DieselRoute2 RouteID = "diesel-2"
	DieselRoute3 RouteID = "diesel-3"
	DieselRoute4 RouteID = "diesel-4"
	DieselRoute5 RouteID = "diesel-5"

	HybridRoute1 RouteID = "hybrid-1"
	HybridRoute2 RouteID = "hybrid-2"
	HybridRoute3 RouteID = "hybrid-3"
	HybridRoute4 RouteID = "hybrid-4"
	HybridRoute5 RouteID = "hybrid-5"

	GasRoute1 RouteID = "gas-1"
	GasRoute2 RouteID = "gas-2"
	GasRoute3 RouteID = "gas-3"
	GasRoute4 RouteID = "gas-4"
	GasRoute5 RouteID = "gas-5"

	// ScenarioRouteID names the one trajectory that leaves the demonstration service area: the
	// vehicle on it drives out of the zone and back in, which is what an ending beyond the boundary
	// is shown with.
	ScenarioRouteID RouteID = "scenario-1"
)

// routes are every trajectory this build declares, one per demonstration vehicle. A vehicle names one
// of them by identifier, and this is the only place the geometry of any of them lives.
//
// Every ring lies inside the demonstration service area but the scenario one, and every one of them is
// four sides of the avenues the centre is laid out on, joined by two of the streets that cross them.
// The vehicles are therefore spread over the whole centre and are seen driving along the streets the
// city is built on, rather than around one block.
var routes = [...]Route{
	ring(ElectricRoute1, zhibekAtManasa, zhibekAtGogol, moskovskayaAtGogol, moskovskayaAtManasa),
	ring(ElectricRoute2, zhibekAtManasa, zhibekAtIbraimova, moskovskayaAtIbraimova, moskovskayaAtManasa),
	ring(ElectricRoute3, zhibekAtManasa, zhibekAtAbdrakhmanova, moskovskayaAtAbdrakhmanova, moskovskayaAtManasa),
	ring(ElectricRoute4, zhibekAtManasa, zhibekAtTynystanova, moskovskayaAtTynystanova, moskovskayaAtManasa),
	ring(ElectricRoute5, zhibekAtManasa, zhibekAtPanfilova, moskovskayaAtPanfilova, moskovskayaAtManasa),
	ring(GasolineRoute1, zhibekAtManasa, zhibekAtLogvinenko, moskovskayaAtLogvinenko, moskovskayaAtManasa),
	ring(GasolineRoute2, zhibekAtTurusbekova, zhibekAtGogol, moskovskayaAtGogol, moskovskayaAtTurusbekova),
	ring(GasolineRoute3, zhibekAtTurusbekova, zhibekAtIbraimova, moskovskayaAtIbraimova, moskovskayaAtTurusbekova),
	ring(GasolineRoute4, zhibekAtManasa, zhibekAtIsanova, moskovskayaAtIsanova, moskovskayaAtManasa),
	ring(GasolineRoute5, zhibekAtIsanova, zhibekAtGogol, moskovskayaAtGogol, moskovskayaAtIsanova),
	ScenarioRoute(),
	ring(DieselRoute2, zhibekAtTurusbekova, zhibekAtAbdrakhmanova, moskovskayaAtAbdrakhmanova, moskovskayaAtTurusbekova),
	ring(DieselRoute3, frunzeAtTurusbekova, frunzeAtAbdrakhmanova, moskovskayaAtAbdrakhmanova, moskovskayaAtTurusbekova),
	ring(DieselRoute4, zhibekAtIsanova, zhibekAtIbraimova, moskovskayaAtIbraimova, moskovskayaAtIsanova),
	ring(DieselRoute5, zhibekAtTurusbekova, zhibekAtTynystanova, moskovskayaAtTynystanova, moskovskayaAtTurusbekova),
	ring(HybridRoute1, zhibekAtManasa, zhibekAtTurusbekova, moskovskayaAtTurusbekova, moskovskayaAtManasa),
	ring(HybridRoute2, zhibekAtLogvinenko, zhibekAtGogol, moskovskayaAtGogol, moskovskayaAtLogvinenko),
	ring(HybridRoute3, zhibekAtPanfilova, zhibekAtGogol, moskovskayaAtGogol, moskovskayaAtPanfilova),
	ring(HybridRoute4, zhibekAtTynystanova, zhibekAtGogol, moskovskayaAtGogol, moskovskayaAtTynystanova),
	ring(HybridRoute5, zhibekAtAbdrakhmanova, zhibekAtGogol, moskovskayaAtGogol, moskovskayaAtAbdrakhmanova),
	ring(GasRoute1, zhibekAtLogvinenko, zhibekAtIbraimova, moskovskayaAtIbraimova, moskovskayaAtLogvinenko),
	ring(GasRoute2, zhibekAtPanfilova, zhibekAtIbraimova, moskovskayaAtIbraimova, moskovskayaAtPanfilova),
	ring(GasRoute3, zhibekAtTynystanova, zhibekAtIbraimova, moskovskayaAtIbraimova, moskovskayaAtTynystanova),
	ring(GasRoute4, zhibekAtAbdrakhmanova, zhibekAtIbraimova, moskovskayaAtIbraimova, moskovskayaAtAbdrakhmanova),
	ring(GasRoute5, zhibekAtIsanova, zhibekAtAbdrakhmanova, moskovskayaAtAbdrakhmanova, moskovskayaAtIsanova),
}

// Routes answers every trajectory this build declares, as a copy of its own: the corpus, the streets of
// each ring and the points of each ring are copied, so nothing outside this package can rewrite the
// geometry a vehicle is driven along, add a circuit or drop one.
func Routes() []Route {
	declared := make([]Route, 0, len(routes))
	for _, route := range routes {
		route.Streets = slices.Clone(route.Streets)
		route.Points = slices.Clone(route.Points)
		declared = append(declared, route)
	}
	return declared
}

// ScenarioRoute is the trajectory that crosses the boundary of the demonstration area. It starts at a
// corner inside the zone and runs north beyond it, so part of every lap is driven outside the area and
// a ride ended there is an ending beyond the service boundary.
func ScenarioRoute() Route {
	return ring(ScenarioRouteID, zhibekAtIsanova, zhibekAtManasa, manasaFarNorth, isanovaFarNorth)
}

// ring declares a closed circuit over the corners it passes through: side n runs from corner n-1 to
// corner n, and the last side ends where the first began, so the ring closes by its own declaration
// rather than by a rule about the last vertex. The first corner is where a vehicle given this route
// stands.
func ring(id RouteID, corners ...crossing) Route {
	streets := make([]string, 0, len(corners))
	points := make([]fleet.Position, 0, len(corners)+1)
	for index, corner := range corners {
		streets = append(streets, streetBetween(corners[(index+len(corners)-1)%len(corners)], corner))
		points = append(points, corner.at)
	}
	points = append(points, points[0])
	return Route{ID: id, Streets: streets, Points: points}
}

// streetBetween is the street two corners share, which is the street the side between them follows.
// Two corners with nothing in common are two places on different streets, and two corners that share
// only a street running the other way are not on one line: the side between them would cross the city
// rather than follow it.
//
// Which name they have in common also says which way the side runs: two corners sharing the street
// that runs north are on an avenue, and two sharing the one that runs east are on a street.
func streetBetween(from, to crossing) string {
	if from.north == to.north {
		return from.north
	}
	if from.east == to.east {
		return from.east
	}
	panic(notOneStreet(from, to))
}

// notOneStreet says which two corners of a ring do not meet along a street, because a declaration
// that is wrong about its own geometry has to say where.
func notOneStreet(from, to crossing) string {
	return "the corners " + from.north + "×" + from.east + " and " + to.north + "×" + to.east +
		" are not on one street, so the side between them is not a road"
}

// NamedStreets are every street this build declares a crossing on. A side of a ring is named after
// one of these, so a name that is not here is a side nobody can look up on a map.
func NamedStreets() []string {
	named := make([]string, 0, len(streetsOfCentre))
	for _, street := range streetsOfCentre {
		named = append(named, street.name)
	}
	sort.Strings(named)
	return named
}

// RouteOf answers the route a vehicle was given, and reports whether this build declares it.
func RouteOf(id RouteID) (Route, bool) {
	for _, route := range routes {
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

// kilometresPerDegreeLatitude and kilometresPerDegreeLongitude are the size of a degree where the
// fleet stands, in the flat approximation every route distance is measured in. A degree of longitude
// is shorter than a degree of latitude by the cosine of the parallel, and every ring stands within a
// few kilometres of the same one, so the error of the approximation stays far below the width of a
// lane.
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
