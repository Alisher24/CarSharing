// Package lifecycle declares the bounds shared by processes as they start and stop.
package lifecycle

import "time"

// ShutdownTimeout is how long in-flight work is given to finish after a process receives a signal.
const ShutdownTimeout = 10 * time.Second
