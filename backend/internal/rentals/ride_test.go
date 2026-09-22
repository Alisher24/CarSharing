package rentals

import (
	"testing"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/rentals/stage"
)

// The whole of which command applies where: start applies to a reservation, pausing to a ride that is
// driving and continuing to one that is standing still. Every other stage is answered with the code
// that stage deserves rather than with a transition, so a repeated command cannot open a second
// interval.
func TestRideTransitionRule(t *testing.T) {
	for _, command := range []struct {
		action    RentalAction
		appliesTo stage.Stage
		reaches   stage.Stage
	}{
		{StartRental, stage.Reserved, stage.Active},
		{PauseRental, stage.Active, stage.Paused},
		{ResumeRental, stage.Paused, stage.Active},
	} {
		transition := rideTransitions[command.action]
		if transition.from != command.appliesTo || transition.to != command.reaches {
			t.Fatalf("%s moves %s to %s, want %s to %s",
				command.action, transition.from, transition.to, command.appliesTo, command.reaches)
		}

		for _, other := range []stage.Stage{
			stage.Reserved,
			stage.Active,
			stage.Paused,
			stage.Cancelled,
			stage.Expired,
			stage.Completed,
		} {
			refusal := rideRefusal(Rental{Stage: other}, transition)
			want := refusalForStage(other, command.appliesTo)
			switch {
			case want == nil && refusal != nil:
				t.Errorf("%s of a %s rental refused with %s", command.action, other, refusal.Kind)
			case want == nil:
			case refusal == nil:
				t.Fatalf("%s of a %s rental was allowed", command.action, other)
			case refusal.Kind != want.Kind:
				t.Errorf("%s of a %s rental refused with %s, want %s",
					command.action, other, refusal.Kind, want.Kind)
			}
		}
	}
}

// refusalForStage is the answer a stage deserves, or nil when the command applies to it. It is stated
// independently of the rule under test, as the table of the specification states it.
func refusalForStage(target, appliesTo stage.Stage) *Refusal {
	if target == appliesTo {
		return nil
	}
	switch target {
	case stage.Cancelled, stage.Expired:
		return &Refusal{Kind: ReservationExpired}
	case stage.Completed:
		return &Refusal{Kind: RentalCompleted}
	default:
		return &Refusal{Kind: InvalidRentalState}
	}
}

// Every command knows the stage it applies to and the mode it opens, so the interval a transition
// writes is decided by the same declaration that decided the move.
func TestRideTransitionsOpenTheModeOfTheirStage(t *testing.T) {
	for kind, transition := range rideTransitions {
		want := Driving
		if transition.to == stage.Paused {
			want = Paused
		}
		if transition.mode != want {
			t.Errorf("%s opens %s, want %s", kind, transition.mode, want)
		}
	}
}

// A rental that has not begun a ride is not riding, whatever stage it is in: a reservation has no mode
// moment, and neither has a rental that has released its vehicle.
func TestRidingRequiresAStartedRide(t *testing.T) {
	began := time.Date(2026, time.September, 14, 10, 0, 0, 0, time.UTC)
	for _, rental := range []struct {
		name   string
		stage  stage.Stage
		moment bool
		riding bool
	}{
		{"reserved", stage.Reserved, false, false},
		{"active without a mode moment", stage.Active, false, false},
		{"paused without a mode moment", stage.Paused, false, false},
		{"active", stage.Active, true, true},
		{"paused", stage.Paused, true, true},
		{"cancelled with a stray mode moment", stage.Cancelled, true, false},
		{"completed with a stray mode moment", stage.Completed, true, false},
	} {
		target := Rental{Stage: rental.stage}
		if rental.moment {
			target.ModeStartedAt = &began
		}
		if got := target.Riding(); got != rental.riding {
			t.Errorf("a %s rental is riding %t, want %t", rental.name, got, rental.riding)
		}
	}
}
