package main

import (
	"fmt"
	"os"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/heartbeat"
)

// heartbeatProbe reports whether the process that leaves the mark is still running. What it checks
// is the mark and nothing else, so the worker's health does not depend on the API answering.
func heartbeatProbe() int {
	if err := heartbeat.Fresh(heartbeat.Path, time.Now()); err != nil {
		fmt.Fprintf(os.Stderr, "the worker is not reporting itself: %v\n", err)
		return 1
	}
	return 0
}
