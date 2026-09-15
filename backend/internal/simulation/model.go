package simulation

import (
	"errors"
	"math/big"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/fleet"
)

// ErrStateUnusable reports a stored state this model cannot account for: a profile that names a source
// the vehicle does not carry, or a reserve list that does not match the names. It is a defect of what
// was written rather than a condition a client caused, so it is refused rather than repaired.
var ErrStateUnusable = errors.New("the stored model state does not describe a vehicle this model can move")

// ErrTimeReversed reports a journey that would end before the moment the model last accounted for.
// Playing such a journey would subtract time from what the vehicle has already spent and report a
// reserve it never had, so the model refuses instead: the two sides disagree about what time it is,
// and only a fresh reading of the server clock settles that.
var ErrTimeReversed = errors.New(
	"the target moment is earlier than the last processed moment; the model and the clock disagree")

// Mode is what a vehicle is doing during one window of a journey, which is what decides what it
// spends. The driving and paused values are the ones the ride lifecycle moves a rental between.
type Mode string

const (
	Driving Mode = "driving"
	Paused  Mode = "paused"
	Free    Mode = "free"
)

// Span is one stretch of a journey during which the vehicle stays in one mode. A journey alternates
// because a ride alternates; the spans of one journey meet end to end, so the vehicle is never in two
// modes at once and never in none.
type Span struct {
	Mode Mode
	From time.Time
	To   time.Time
}

// State is everything the model needs about a vehicle to move it forward: where it stands, what it
// holds, and the part of a millionth each source has been charged but not yet spent.
type State struct {
	// RouteID names the route the vehicle travels, and Parameters the capacities it is simulated with.
	// Both are stored with the ride that uses them, so a renewed fleet does not recompute the past of a
	// ride that has already begun.
	RouteID    RouteID
	Parameters Parameters

	// Sources are the inventories the vehicle carries, in the order its profile uses them. Each one
	// carries what its windows have charged, so a journey split into many short steps costs exactly what
	// the same journey costs in one call.
	Sources []Source

	// Path is how far along the route the vehicle has travelled, and Position where it stands at the
	// moment below. A vehicle a demonstration command moved away from its route carries IsOffRoute
	// and, in JoinPath, the distance along the route of the point it is driving back to.
	Path       Path
	JoinPath   Path
	IsOffRoute bool
	Position   fleet.Position

	// Depleted reports a vehicle that has already run out. It stands where it stopped and spends
	// nothing more, however long the demonstration goes on: only an explicit refill moves it again.
	Depleted bool

	// ProcessedAt is the moment everything above describes.
	ProcessedAt time.Time
}

// Motion is what the model says about a vehicle over one journey: where it stands at the end, what it
// holds, which sources the journey used up, and when it could go no further.
type Motion struct {
	Position fleet.Position

	// Holding is what is left of each source of the vehicle, in the order its profile uses them.
	Holding []fleet.Amount

	// Exhausted names the sources the vehicle used up and moved off, in the order it moved off them.
	// A vehicle that reached the target with something left in every source names none of them.
	Exhausted []fleet.SourceKind

	// Depleted is the first moment at which no source could carry the vehicle any further, or the zero
	// moment when it reached the target with something left. A vehicle whose battery is empty beside a
	// usable reserve is not depleted: the reserve is one of its sources.
	Depleted time.Time
}

// DepletedBy reports when the model ran out, so a caller can compare it with a command's own moment
// without asking whether there was one at all.
func (m Motion) DepletedBy() (time.Time, bool) { return m.Depleted, !m.Depleted.IsZero() }

// Holding is what is left of each source of a state, in the order the vehicle carries them.
func (s State) Holding() []fleet.Amount {
	amounts := make([]fleet.Amount, 0, len(s.Sources))
	for _, source := range s.Sources {
		amounts = append(amounts, source.Remaining())
	}
	return amounts
}

// Journey plays the spans out and advances the state to them. It refuses a journey that ends before
// the moment the state describes, or one whose spans do not meet end to end at it.
func (s State) Journey(spans []Span, target time.Time) (Motion, State, error) {
	if target.Before(s.ProcessedAt) {
		return Motion{}, State{}, ErrTimeReversed
	}
	if s.Depleted {
		// A vehicle that has run out stays where it stopped and spends nothing more, however long the
		// demonstration goes on: only an explicit refill moves it again. The moment it is asked about
		// still moves forward, so the next tick is not a journey into the past.
		stopped := s.clone()
		stopped.ProcessedAt = target
		return Motion{Position: stopped.Position, Holding: stopped.Holding()}, stopped, nil
	}
	if err := checkSpans(s, spans, target); err != nil {
		return Motion{}, State{}, err
	}
	if err := checkState(s); err != nil {
		return Motion{}, State{}, err
	}

	moved := s.clone()
	exhausted, depleted, charged := moved.burn(spans)
	// A journey that ran out ended where the reserve did, so the distance is read from the time the
	// sources were charged for rather than from the windows the caller named: a vehicle that stopped two
	// minutes into a five-minute window stands two minutes along, not five.
	moved.Path = s.Path + travelled(charged)
	moved.IsOffRoute = s.IsOffRoute && moved.Path < s.JoinPath
	moved.Position = moved.at()
	if depleted.IsZero() {
		moved.ProcessedAt = target
	} else {
		// The journey ended where the reserve did rather than where the caller asked about it, so the
		// state describes the moment the vehicle stopped and not a later one.
		moved.Depleted = true
		moved.ProcessedAt = depleted
	}
	return Motion{
		Position:  moved.Position,
		Holding:   moved.Holding(),
		Exhausted: exhausted,
		Depleted:  depleted,
	}, moved, nil
}

// clone is a state a journey may be played on without disturbing the one it was read from. The sources
// are copied because a journey that stops early leaves the reserve of a source it never reached exactly
// as it found it.
func (s State) clone() State {
	moved := s
	moved.Sources = make([]Source, len(s.Sources))
	for index, source := range s.Sources {
		moved.Sources[index] = source
		moved.Sources[index].Charged = new(big.Int).Set(source.Charged)
	}
	return moved
}

// burn spends the reserves of the vehicle over the whole journey, in order. It reports the sources the
// journey used up, the first moment at which no source could carry the vehicle any further, which is
// where a ride that runs out ends rather than where the command that noticed it arrived, and the time
// the sources were charged for while the vehicle was moving. The last of the three is the distance the
// vehicle really covered: a vehicle that ran out does not travel the part of the window after it.
func (s *State) burn(spans []Span) ([]fleet.SourceKind, time.Time, time.Duration) {
	var exhausted []fleet.SourceKind
	var inMotion time.Duration
	for _, span := range spans {
		if !spends(span.Mode) {
			continue
		}
		used, charged, ranOut := s.consume(span)
		exhausted = append(exhausted, used...)
		if span.Mode == Driving {
			inMotion += charged
		}
		if !ranOut.IsZero() {
			return exhausted, ranOut, inMotion
		}
	}
	return exhausted, time.Time{}, inMotion
}

// consume takes one window out of the sources the vehicle is moving on. It reports the sources the
// window used up, the time of the window that was charged for, and the moment the vehicle ran out, or
// the zero moment when it still has something to move on.
//
// The window is handed to a source as one stretch rather than as one subtraction per tick: what a
// stretch costs follows from its duration and the rate alone, so nothing depends on how often the model
// was asked to move. A source that runs out inside the window ends its stretch there, and the part of
// the window after it belongs to the next source, so none of the window is lost at the switch.
func (s *State) consume(span Span) ([]fleet.SourceKind, time.Duration, time.Time) {
	var exhausted []fleet.SourceKind
	var charged time.Duration
	from := span.From
	for from.Before(span.To) {
		position, found := s.current()
		if !found {
			// Nothing the vehicle carries can move it any further, so it stops where its last source
			// did rather than at the end of a window it never covered.
			return exhausted, charged, from
		}
		source := s.Sources[position]
		rate := source.DrivingRate()
		if span.Mode == Paused {
			rate = source.PausedRate()
		}
		until := source.spentAt(rate, source.Charged, from, span.To.Sub(from))
		source.consume(rate, until.Sub(from), source.Charged)
		s.Sources[position] = source
		charged += until.Sub(from)
		if until.Equal(span.To) {
			// The source covered the whole window, so the next window begins wherever the vehicle
			// carries on: a source ending exactly with a window is one of the sources the vehicle has
			// used up, not a vehicle that has run out.
			if source.Remaining() > 0 {
				return exhausted, charged, time.Time{}
			}
			exhausted = append(exhausted, source.Kind)
			if _, continues := s.current(); continues {
				return exhausted, charged, time.Time{}
			}
			return exhausted, charged, span.To
		}
		// The source ran out inside the window, and the part of the window after it belongs to whatever
		// the vehicle carries next.
		exhausted = append(exhausted, source.Kind)
		from = until
	}
	return exhausted, charged, time.Time{}
}

// spends reports whether a vehicle in this mode uses any of its reserve at all. A free or reserved
// vehicle neither moves nor spends: it stands where it was left until somebody books it.
func spends(mode Mode) bool { return mode == Driving || mode == Paused }

// current is the source the vehicle moves on: the first one it carries, in the order its profile uses
// them, that still holds something. A source with nothing in it is not a reserve, and a vehicle whose
// battery is empty beside a full tank is moving on the tank.
func (s State) current() (int, bool) {
	for position, source := range s.Sources {
		if source.Remaining() > 0 {
			return position, true
		}
	}
	return 0, false
}

// checkState refuses a state this model cannot account for rather than guessing what was meant.
func checkState(s State) error {
	if len(s.Sources) == 0 {
		return ErrStateUnusable
	}
	if _, known := RouteOf(s.RouteID); !known {
		return ErrStateUnusable
	}
	for _, source := range s.Sources {
		if source.Kind == "" || source.Capacity <= 0 || source.Charged == nil {
			return ErrStateUnusable
		}
	}
	return nil
}

// checkSpans refuses a journey the model cannot account for: one that begins before the moment the
// state describes, that reaches past the target, or whose windows do not meet end to end.
func checkSpans(s State, spans []Span, target time.Time) error {
	if len(spans) == 0 {
		return ErrTimeReversed
	}
	previous := s.ProcessedAt
	for _, span := range spans {
		if !span.From.Equal(previous) || span.To.Before(span.From) || span.To.After(target) {
			return ErrTimeReversed
		}
		previous = span.To
	}
	if !previous.Equal(target) {
		return ErrTimeReversed
	}
	return nil
}

// at is where the vehicle stands: on its route, or on the straight line between the place a
// demonstration command moved it to and the point of the route it is driving back to. The connecting
// stretch is driven at the ordinary speed, so it is charged as ordinary movement.
func (s State) at() fleet.Position {
	route, known := RouteOf(s.RouteID)
	if !known {
		return s.Position
	}
	if !s.IsOffRoute {
		return route.PositionAt(s.Path)
	}
	if s.JoinPath <= 0 {
		return route.PositionAt(0)
	}
	return along(s.Position, route.PositionAt(s.JoinPath), float64(s.Path)/float64(s.JoinPath))
}
