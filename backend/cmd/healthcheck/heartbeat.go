package main

import (
	"fmt"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/heartbeat"
)

// heartbeatProbe reports whether the process that leaves the mark is still running. What it checks
// is the mark and nothing else, so the worker's health does not depend on the API answering.
func heartbeatProbe() error {
	if err := heartbeat.Fresh(heartbeat.Path, time.Now()); err != nil {
		return fmt.Errorf("the worker is not reporting itself: %w", err)
	}
	return nil
}
