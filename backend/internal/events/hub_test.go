package events

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

// The fan-out offers a signal to every stream whose audience may hear it, and to no other.
func TestASignalReachesExactlyTheStreamsThatMayHearIt(t *testing.T) {
	hub := NewHub(nil)
	public := hub.Subscribe(Public())
	owner, other := uuid.New(), uuid.New()
	private := hub.Subscribe(Private(owner))
	stranger := hub.Subscribe(Private(other))

	hub.publish(Signal{Kind: VehicleChanged, ResourceID: testVehicle, Version: 1})
	hub.publish(Signal{Kind: RentalChanged, ResourceID: testVehicle, Version: 2, Recipient: owner})

	if got := len(public.Signals()); got != 1 {
		t.Errorf("the public stream holds %d signals, want 1", got)
	}
	if got := len(private.Signals()); got != 1 {
		t.Errorf("the owner's stream holds %d signals, want 1", got)
	}
	if got := len(stranger.Signals()); got != 0 {
		t.Errorf("another account's stream holds %d signals, want none", got)
	}
}

// A reader that cannot keep up is closed rather than allowed to hold an unbounded backlog, and the
// fan-out never waits for it: this is what keeps one slow client from delaying the others.
func TestAStreamThatFallsBehindIsClosedWithoutHoldingUpTheOthers(t *testing.T) {
	hub := NewHub(nil)
	behind := hub.Subscribe(Public())
	keeping := hub.Subscribe(Public())

	for version := range queueSize {
		hub.publish(Signal{Kind: VehicleChanged, ResourceID: testVehicle, Version: int64(version)})
	}
	if reason := behind.Reason(); reason != nil {
		t.Fatalf("a stream that is merely full was closed: %v", reason)
	}

	// The reader of the other stream drains as the signals arrive, which is what a client that is
	// keeping up does.
	drained := make(chan int, queueSize+1)
	go func() {
		for range keeping.Signals() {
			drained <- 1
		}
	}()
	hub.publish(Signal{Kind: VehicleChanged, ResourceID: testVehicle, Version: int64(queueSize)})
	drained <- 1

	if reason := behind.Reason(); !errors.Is(reason, ErrFellBehind) {
		t.Fatalf("a stream that fell behind ended with %v", reason)
	}
	// What the stream had already accepted stays readable, so a reader that is merely slow still
	// receives it; the end arrives after those frames rather than instead of them.
	accepted := 0
	for range behind.Signals() {
		accepted++
	}
	if accepted != queueSize {
		t.Fatalf("the closed stream held %d signals, want %d", accepted, queueSize)
	}
}

// A stream that has ended stops being offered signals, so a long-lived process does not accumulate
// subscriptions for connections that are gone.
func TestAStreamThatHasEndedIsNoLongerOfferedSignals(t *testing.T) {
	hub := NewHub(nil)
	stream := hub.Subscribe(Public())
	if hub.Subscribers() != 1 {
		t.Fatalf("the hub holds %d streams, want 1", hub.Subscribers())
	}
	hub.forget(stream)
	stream.Close()

	if hub.Subscribers() != 0 {
		t.Fatalf("the hub still holds %d streams", hub.Subscribers())
	}
	hub.publish(Signal{Kind: VehicleChanged, ResourceID: testVehicle, Version: 1})
	if got := len(stream.Signals()); got != 0 {
		t.Fatalf("an ended stream received %d signals", got)
	}
}
