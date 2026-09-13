package httpapi

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	servedapi "github.com/Alisher24/CarSharing/backend/internal/contracts/servedapi"
	"github.com/Alisher24/CarSharing/backend/internal/events"
	"github.com/Alisher24/CarSharing/backend/internal/platform/sessions"
)

// EventStream is what the streaming operations need from the signals feature: a subscription that
// writes its frames to a connection, and the authoritative time a handshake announces. Serve reports
// why the stream ended, or nil for a stream that ended without anything worth reporting. The router
// over a process that holds no signals is given an implementation that refuses instead.
type EventStream interface {
	Serve(ctx context.Context, w io.Writer, options events.StreamOptions) error
	ServerTime(ctx context.Context) (time.Time, error)
}

// streams answers the two operations whose response does not end before the handler returns. Both are
// the same stream over a different audience: what a connection may hear, and — for the private one —
// the session it must keep proving, are the whole difference between them.
type streams struct {
	hub EventStream

	// sessions proves the session of a private stream. A router that serves no streams has none, and
	// reaches that field only after the session boundary has already refused the request.
	sessions *sessions.Manager
}

// newStreams builds the streaming operations, or names the dependency that is missing.
func newStreams(hub EventStream, sessionManager *sessions.Manager) (streams, error) {
	if hub == nil {
		return streams{}, fmt.Errorf("%w: event streams", ErrIncompleteApplication)
	}
	return streams{hub: hub, sessions: sessionManager}, nil
}

// unservedStreams answers the streaming operations on a router that holds no signals: the isolated
// contract router and the container's own health check serve no long-lived connection. The operations
// exist there, so they are answered as unavailable rather than as unknown resources.
type unservedStreams struct{}

// ErrNoEventStreams reports a streaming operation reached on a router that does not serve streams.
var ErrNoEventStreams = errors.New("this router serves no event streams")

func (unservedStreams) Serve(context.Context, io.Writer, events.StreamOptions) error {
	return ErrNoEventStreams
}

func (unservedStreams) ServerTime(context.Context) (time.Time, error) {
	return time.Time{}, ErrNoEventStreams
}

func (s streams) GetPublicEvents(
	ctx context.Context, _ servedapi.GetPublicEventsRequestObject,
) (servedapi.GetPublicEventsResponseObject, error) {
	body, err := s.open(ctx, events.StreamOptions{Audience: events.Public()})
	if err != nil {
		return servedapi.GetPublicEvents503JSONResponse{Body: serviceUnavailable(ctx)}, nil
	}
	return servedapi.GetPublicEvents200TexteventStreamResponse{Body: body}, nil
}

// GetPrivateEvents opens the stream of one account. The session is resolved by the boundary before
// this runs, and is proven again for as long as the stream lives, because a request that was
// authenticated once is not authenticated for the next change it is sent.
func (s streams) GetPrivateEvents(
	ctx context.Context, _ servedapi.GetPrivateEventsRequestObject,
) (servedapi.GetPrivateEventsResponseObject, error) {
	snapshot, live, err := sessionOf(ctx).resolve(ctx)
	if err != nil {
		return servedapi.GetPrivateEvents503JSONResponse{Body: serviceUnavailable(ctx)}, nil
	}
	if !live {
		return servedapi.GetPrivateEvents401JSONResponse{
			Body: apiErrorBody(ctx, codeAuthenticationRequired, messageAuthenticationRequired),
		}, nil
	}
	body, err := s.open(ctx, events.StreamOptions{
		Audience: events.Private(snapshot.UserID),
		Session:  s.sessionCheck(sessionToken(ctx)),
	})
	if err != nil {
		return servedapi.GetPrivateEvents503JSONResponse{Body: serviceUnavailable(ctx)}, nil
	}
	return servedapi.GetPrivateEvents200TexteventStreamResponse{Body: body}, nil
}

// sessionCheck proves, for as long as a private stream lives, that the session it was opened with
// still belongs to the account it was opened for. Revocation, expiry and a store that cannot answer
// all end the stream: a session that was not proven live is not treated as one.
func (s streams) sessionCheck(token string) events.SessionCheck {
	return func(ctx context.Context) (events.SessionState, error) {
		snapshot, live, err := s.sessions.Restore(ctx, token)
		if err != nil {
			return events.SessionState{}, err
		}
		return events.SessionState{Owner: snapshot.UserID, Live: live}, nil
	}
}

// open starts a stream and returns the body the generated response writes from. The frames are
// written by their own goroutine because the generated streaming response reads the body until it
// ends, which is exactly as long as the connection lives.
//
// The reader is always closed cleanly: an end of a stream is a normal end of this response, and the
// client recovers by reading the snapshot rather than by reading an error out of a stream that has
// already been answered.
func (s streams) open(ctx context.Context, options events.StreamOptions) (io.Reader, error) {
	establishedAt, err := s.hub.ServerTime(ctx)
	if err != nil {
		return nil, err
	}
	options.EstablishedAt = establishedAt

	reader, writer := io.Pipe()
	go func() {
		ended := s.hub.Serve(ctx, writer, options)
		s.reportEnd(options.Audience, ended)
		_ = writer.Close()
	}()
	return reader, nil
}

// reportEnd states how one stream ended. An ordinary end is information; anything else is a failure
// of this process and is reported as one.
func (s streams) reportEnd(audience events.Audience, ended error) {
	attributes := []any{"audience", audience.String(), "reason", endReason(ended)}
	if ordinaryStreamEnd(ended) {
		slog.Info("event stream ended", attributes...)
		return
	}
	slog.Error("event stream failed", attributes...)
}

// ordinaryStreamEnd reports the ends that are a part of serving a stream rather than a failure of
// this process: a client that went away, a subscription that was dropped for falling behind, and a
// session that no longer holds the stream.
func ordinaryStreamEnd(ended error) bool {
	return ended == nil ||
		errors.Is(ended, events.ErrClientGone) ||
		errors.Is(ended, events.ErrFellBehind) ||
		errors.Is(ended, events.ErrSessionEnded)
}

// endReason states why a stream ended for a log line. A stream that ended without a reason is
// reported as closed rather than as a failure.
func endReason(ended error) string {
	if ended == nil {
		return "closed"
	}
	return ended.Error()
}
