package events

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

// listenerRetryDelay separates two attempts to hold a listening connection, so a database that is
// restarting is not hammered while it comes back.
const listenerRetryDelay = time.Second

// Run listens on the signal channel until ctx is done, holding one connection for the whole
// subscription and taking a new one when that connection is lost. A lost listener costs promptness
// and not correctness: the signals published while it was down are gone, and every client repairs
// what it missed by reading the snapshot again.
func (h *Hub) Run(ctx context.Context) {
	for {
		err := h.listen(ctx)
		if ctx.Err() != nil {
			slog.Info("signal listener stopped")
			return
		}
		slog.Error("signal listener lost its connection", "error", err.Error())
		select {
		case <-ctx.Done():
			slog.Info("signal listener stopped")
			return
		case <-time.After(listenerRetryDelay):
		}
	}
}

// listen holds one listening connection and offers every payload it receives to the fan-out.
func (h *Hub) listen(ctx context.Context) error {
	connection, err := h.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer connection.Release()

	if _, err = connection.Exec(ctx, "LISTEN "+pgx.Identifier{Channel}.Sanitize()); err != nil {
		return err
	}
	for {
		notification, err := connection.Conn().WaitForNotification(ctx)
		if err != nil {
			return err
		}
		signal, err := decodeSignal([]byte(notification.Payload))
		if err != nil {
			// Every publisher of this channel is a process of this application, so a payload that
			// cannot be read is a defect there. Nothing can be delivered from it, and the listener
			// keeps its place rather than dropping the subscription over one bad message.
			slog.Error("published signal could not be read", "error", err.Error())
			continue
		}
		h.publish(signal)
	}
}
