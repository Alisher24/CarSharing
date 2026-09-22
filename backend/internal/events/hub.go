package events

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// queueSize is how many signals one stream may have waiting to be written. A reader that falls
// further behind than this is closed instead of being allowed to hold a backlog it will never catch
// up with: it recovers by reading the snapshot again, which is cheaper than replaying the signals.
const queueSize = 64

// ErrFellBehind reports a subscription that was closed because its reader could not keep up. The
// client is expected to recover through the snapshot rather than through the signals it missed.
var ErrFellBehind = errors.New("stream fell behind its queue")

// Audience is who a stream may hear about: every reader, or the one account a private stream belongs
// to. The two are exclusive, so a private stream is never sent the public changes as well and a
// public stream never receives a change that belongs to somebody.
type Audience struct {
	// Owner is the account a private stream reads for, empty for the public stream.
	Owner uuid.UUID
}

// Public is the audience of a stream any visitor may hold open.
func Public() Audience { return Audience{} }

// Private is the audience of a stream opened with the session of one account.
func Private(owner uuid.UUID) Audience { return Audience{Owner: owner} }

// hears reports whether this audience is offered a signal.
func (a Audience) hears(signal Signal) bool {
	if a.Owner == uuid.Nil {
		return signal.Public()
	}
	return !signal.Public() && signal.Recipient == a.Owner
}

// String names the audience in a log line without naming the account it belongs to: which stream
// ended is what an operator needs, and whose it was is already known to that account.
func (a Audience) String() string {
	if a.Owner == uuid.Nil {
		return "public"
	}
	return "private"
}

// Subscription is one stream's place in the fan-out: the signals its audience may hear, in the order
// they were published, up to the size of its queue.
type Subscription struct {
	audience Audience
	queue    chan Signal

	mu     sync.Mutex
	closed bool
	reason error
}

func newSubscription(audience Audience) *Subscription {
	return &Subscription{audience: audience, queue: make(chan Signal, queueSize)}
}

// Signals is the queue this subscription receives. It is closed when the subscription ends, so a
// reader learns of the end from the channel itself.
func (s *Subscription) Signals() <-chan Signal { return s.queue }

// Reason reports why the subscription ended, or nil while it is open and when it was closed
// deliberately.
func (s *Subscription) Reason() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reason
}

// offer hands one signal to this subscription, or closes it when its queue is full. Offering never
// blocks: one reader that cannot keep up must not hold up the delivery to the others.
func (s *Subscription) offer(signal Signal) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	select {
	case s.queue <- signal:
	default:
		s.close(ErrFellBehind)
	}
}

// Close ends the subscription. Ending it twice is not an error: the reader and the hub may both do it.
func (s *Subscription) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.close(nil)
}

func (s *Subscription) close(reason error) {
	if s.closed {
		return
	}
	s.closed = true
	if reason != nil {
		s.reason = reason
	}
	close(s.queue)
}

// Hub fans out the signals published on the channel to the streams currently subscribed. It keeps no
// history: a stream that was not subscribed when a signal was published never receives it, which is
// what makes the REST snapshot, and not the stream, the thing a client recovers from.
type Hub struct {
	pool *pgxpool.Pool

	mu          sync.Mutex
	subscribers map[*Subscription]struct{}
}

// NewHub builds the fan-out over the database the worker publishes on.
func NewHub(pool *pgxpool.Pool) *Hub {
	return &Hub{pool: pool, subscribers: map[*Subscription]struct{}{}}
}

// Subscribe registers one stream and returns its place in the fan-out. The caller ends it when the
// connection ends, whether by Close or by reading its closed queue.
func (h *Hub) Subscribe(audience Audience) *Subscription {
	subscription := newSubscription(audience)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subscribers[subscription] = struct{}{}
	return subscription
}

// forget removes a subscription that has ended, so a stream that is over stops being offered signals.
func (h *Hub) forget(subscription *Subscription) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subscribers, subscription)
}

// Subscribers reports how many streams are connected. It is what a process states about itself when
// asked whether the API is serving anybody.
func (h *Hub) Subscribers() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subscribers)
}

// publish offers one signal to every subscription whose audience may hear it.
func (h *Hub) publish(signal Signal) {
	h.mu.Lock()
	current := make([]*Subscription, 0, len(h.subscribers))
	for subscription := range h.subscribers {
		current = append(current, subscription)
	}
	h.mu.Unlock()

	for _, subscription := range current {
		if subscription.audience.hears(signal) {
			subscription.offer(signal)
		}
	}
}

// ServerTime reads the authoritative instant a handshake announces. It is the database clock rather
// than this process's, because the browser compares what it is told against the times a snapshot
// states.
func (h *Hub) ServerTime(ctx context.Context) (time.Time, error) {
	return database.Moment(ctx, database.QuerierFrom(ctx, h.pool))
}
