package events

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// frames is what one stream wrote, in the order it wrote it. A pipe hands each write to the reader
// as it is made, so one item of the channel is one frame.
type frames chan string

// serve runs one stream over a pipe and returns what it writes, so a test asserts about the frames a
// client would receive.
func serve(t *testing.T, hub *Hub, options StreamOptions) (frames, <-chan error, func()) {
	t.Helper()
	reader, writer := io.Pipe()
	written := make(frames, 128)
	go func() {
		defer close(written)
		buffer := make([]byte, 4096)
		for {
			read, err := reader.Read(buffer)
			if read > 0 {
				written <- string(buffer[:read])
			}
			if err != nil {
				return
			}
		}
	}()

	ended := make(chan error, 1)
	go func() { ended <- hub.Serve(context.Background(), writer, options) }()
	return written, ended, func() {
		_ = writer.Close()
		_ = reader.Close()
	}
}

// next reads one frame, or reports the stream as silent.
func next(t *testing.T, written frames, patience time.Duration) string {
	t.Helper()
	select {
	case frame, open := <-written:
		if !open {
			t.Fatal("the stream ended before it wrote the frame the test waits for")
		}
		return frame
	case <-time.After(patience):
		t.Fatal("the stream wrote nothing")
		return ""
	}
}

// A stream announces itself before it announces anything else: a client that has not been told its
// subscription is established has no reason to read a snapshot yet.
func TestAStreamAnnouncesItselfBeforeAnyChange(t *testing.T) {
	hub := NewHub(nil)
	established := time.Date(2026, time.September, 12, 7, 15, 30, 0, time.UTC)
	written, _, stop := serve(t, hub, StreamOptions{Audience: Public(), EstablishedAt: established})
	defer stop()

	handshake := next(t, written, time.Second)
	if !strings.HasPrefix(handshake, "retry: 3000\nevent: ready\n") {
		t.Fatalf("the first frame was %q", handshake)
	}

	hub.publish(Signal{Kind: VehicleChanged, ResourceID: testVehicle, Version: 42})
	change := next(t, written, time.Second)
	want := "event: vehicle.changed\ndata: {\"id\":\"" + testVehicle + "\",\"version\":\"42\"}\n\n"
	if change != want {
		t.Fatalf("the change frame was %q, want %q", change, want)
	}
}

// A private stream proves its session while nothing happens, and ends when that session is gone: a
// person who is signed out must stop receiving what their account is told.
func TestAPrivateStreamEndsWhenItsSessionIsNoLongerLive(t *testing.T) {
	hub := NewHub(nil)
	owner := uuid.New()
	live := true
	session := func(context.Context) (SessionState, error) {
		return SessionState{Owner: owner, Live: live}, nil
	}
	started := time.Now()
	_, ended, stop := serve(t, hub, StreamOptions{Audience: Private(owner), Session: session})
	defer stop()

	live = false
	select {
	case err := <-ended:
		if !errors.Is(err, ErrSessionEnded) {
			t.Fatalf("the stream ended with %v", err)
		}
		if waited := time.Since(started); waited < sessionCheckInterval {
			t.Fatalf("the session was proven again after %s, inside its interval", waited)
		}
	case <-time.After(sessionCheckInterval + 2*time.Second):
		t.Fatal("a private stream outlived the session it was opened with")
	}
}

// A session that cannot be checked is not a session that was proven live.
func TestAPrivateStreamEndsWhenItsSessionCannotBeChecked(t *testing.T) {
	hub := NewHub(nil)
	session := func(context.Context) (SessionState, error) { return SessionState{}, errSessionStoreDown }
	_, ended, stop := serve(t, hub, StreamOptions{Audience: Private(uuid.New()), Session: session})
	defer stop()

	select {
	case err := <-ended:
		if !errors.Is(err, ErrSessionEnded) || !errors.Is(err, errSessionStoreDown) {
			t.Fatalf("the stream ended with %v", err)
		}
	case <-time.After(sessionCheckInterval + 2*time.Second):
		t.Fatal("a stream whose session could not be checked stayed open")
	}
}

var errSessionStoreDown = errors.New("session store did not answer")

// A client that has stopped reading is closed rather than held open: the write bound is what turns a
// client that accepts nothing into a stream that ends.
func TestAStreamWhoseClientStopsReadingEndsWithinTheWriteBound(t *testing.T) {
	hub := NewHub(nil)
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close() }()

	started := time.Now()
	// Nobody reads this pipe, so even the handshake cannot be written.
	err := hub.Serve(context.Background(), writer, StreamOptions{Audience: Public()})
	if !errors.Is(err, ErrClientGone) || !errors.Is(err, ErrWriteTimeout) {
		t.Fatalf("the stream ended with %v", err)
	}
	if waited := time.Since(started); waited < writeTimeout {
		t.Fatalf("the stream gave up after %s, inside its write bound", waited)
	}
}

// A private stream is refused where it is built when nothing can prove its session, rather than
// discovered at the first change it is offered.
func TestAPrivateStreamWithoutASessionCheckIsRefused(t *testing.T) {
	hub := NewHub(nil)
	err := hub.Serve(context.Background(), io.Discard, StreamOptions{Audience: Private(uuid.New())})
	if !errors.Is(err, ErrIncompleteStream) {
		t.Fatalf("the stream was opened with %v", err)
	}
}
