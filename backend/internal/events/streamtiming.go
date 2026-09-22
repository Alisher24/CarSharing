package events

import "time"

const (
	writeTimeout         = 5 * time.Second
	sessionCheckInterval = 5 * time.Second
	keepaliveInterval    = 15 * time.Second
)

type StreamTiming struct {
	WriteTimeout         time.Duration
	SessionCheckInterval time.Duration
	KeepaliveInterval    time.Duration
}

func DefaultStreamTiming() StreamTiming {
	return StreamTiming{
		WriteTimeout:         writeTimeout,
		SessionCheckInterval: sessionCheckInterval,
		KeepaliveInterval:    keepaliveInterval,
	}
}

// Validate rejects absent or non-positive stream intervals.
func (timing StreamTiming) Validate() error {
	if timing.WriteTimeout <= 0 || timing.SessionCheckInterval <= 0 || timing.KeepaliveInterval <= 0 {
		return ErrIncompleteStream
	}
	return nil
}
