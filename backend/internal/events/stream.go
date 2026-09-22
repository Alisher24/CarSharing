package events

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
)

// Reasons a stream ends. The first three are ordinary ends of a connection; a stream that ends for
// any other reason is a failure of this server and is reported as one.
var (
	// ErrClientGone reports a connection that is no longer there: the browser closed it, or a frame
	// was not accepted within the write bound.
	ErrClientGone = errors.New("the client is gone")

	// ErrSessionEnded reports a private stream whose session was revoked or expired, whose browser now
	// carries another account, or whose session cannot be checked at all. A session that cannot be
	// proven live is not treated as one that is.
	ErrSessionEnded = errors.New("private session is no longer live")

	// ErrWriteTimeout reports a frame a client did not accept within the write bound, which is how a
	// client that has stopped reading is recognized.
	ErrWriteTimeout = errors.New("the client did not accept a frame in time")

	// ErrIncompleteStream reports a stream that was not told what it needs. It is refused where the
	// stream is built rather than discovered by the first private change.
	ErrIncompleteStream = errors.New("the stream is missing what it must be told")
)

// SessionState is what a check of a private stream's session reports: the account the session now
// belongs to, and whether it is live at all.
type SessionState struct {
	Owner uuid.UUID
	Live  bool
}

// SessionCheck proves that a private stream may still be served. A check that cannot answer is
// reported as an error, and the stream ends.
type SessionCheck func(ctx context.Context) (SessionState, error)

// StreamOptions is what the transport tells a stream before it starts.
type StreamOptions struct {
	Timing StreamTiming

	// Audience is who this stream may hear about.
	Audience Audience

	// Session proves a private stream's session. It is required for a private audience and unused for
	// a public one, which has nothing to prove.
	Session SessionCheck

	// EstablishedAt is the authoritative instant the handshake announces.
	EstablishedAt time.Time
}

func (o StreamOptions) validate() error {
	if err := o.Timing.Validate(); err != nil {
		return err
	}
	if o.Audience.Owner != uuid.Nil && o.Session == nil {
		return fmt.Errorf("%w: private stream session check", ErrIncompleteStream)
	}
	return nil
}

// Serve writes one SSE connection: the handshake, a comment while nothing changes, and a frame for
// every signal the subscription is offered. It returns when the connection ends — the client is
// gone, the stream fell behind, the session no longer holds it, or this process is stopping — and
// states which of those it was, because none of them is a stream that simply ended. A client
// recovers from a missing signal by reading the snapshot again rather than by waiting for a replay
// that does not exist.
func (h *Hub) Serve(ctx context.Context, w io.Writer, options StreamOptions) error {
	if err := options.validate(); err != nil {
		return err
	}
	subscription := h.Subscribe(options.Audience)
	defer h.forget(subscription)
	defer subscription.Close()

	if err := writeFrame(w, readyFrame(options.EstablishedAt), options.Timing.WriteTimeout); err != nil {
		return fmt.Errorf("%w: %w", ErrClientGone, err)
	}

	keepalive := time.NewTicker(options.Timing.KeepaliveInterval)
	defer keepalive.Stop()
	sessionChecks, stopSessionChecks := sessionTicker(options)
	defer stopSessionChecks()

	for {
		select {
		case <-ctx.Done():
			return ErrClientGone
		case signal, open := <-subscription.Signals():
			if !open {
				return subscriptionEnding(subscription)
			}
			if err := proven(ctx, options); err != nil {
				return err
			}
			if err := writeFrame(w, signal.frame(), options.Timing.WriteTimeout); err != nil {
				return fmt.Errorf("%w: %w", ErrClientGone, err)
			}
		case <-keepalive.C:
			if err := writeFrame(w, keepaliveFrame, options.Timing.WriteTimeout); err != nil {
				return fmt.Errorf("%w: %w", ErrClientGone, err)
			}
		case <-sessionChecks:
			if err := proven(ctx, options); err != nil {
				return err
			}
		}
	}
}

// subscriptionEnding names why a closed subscription ended: falling behind is an end the client has
// to repair by reading the snapshot again, and any other close was deliberate.
func subscriptionEnding(subscription *Subscription) error {
	if reason := subscription.Reason(); reason != nil {
		return reason
	}
	return ErrClientGone
}

// sessionTicker schedules the repeated session check of a private stream. A public stream gets no
// channel at all, so its select never wakes for a check it does not need.
func sessionTicker(options StreamOptions) (<-chan time.Time, func()) {
	if options.Audience.Owner == uuid.Nil {
		return nil, func() {}
	}
	ticker := time.NewTicker(options.Timing.SessionCheckInterval)
	return ticker.C, ticker.Stop
}

// proven checks that a private stream may still be served. A public stream has nothing to prove.
func proven(ctx context.Context, options StreamOptions) error {
	if options.Audience.Owner == uuid.Nil {
		return nil
	}
	state, err := options.Session(ctx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSessionEnded, err)
	}
	if !state.Live {
		return ErrSessionEnded
	}
	if state.Owner != options.Audience.Owner {
		// The browser carries another account's session now: continuing would hand this account's
		// changes to that one.
		return ErrSessionEnded
	}
	return nil
}

// writeFrame writes one frame within the stream's write bound. The write runs in its own goroutine
// because a client that has stopped reading blocks it: the bound is what turns such a client into a
// closed stream instead of a connection held open for as long as it likes.
func writeFrame(w io.Writer, frame []byte, timeout time.Duration) error {
	written := make(chan error, 1)
	go func() {
		_, err := w.Write(frame)
		written <- err
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-written:
		return err
	case <-timer.C:
		return ErrWriteTimeout
	}
}
