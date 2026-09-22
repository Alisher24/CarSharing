// Package heartbeat lets a process that serves nothing prove it is still running: it writes a mark
// to a file on a fixed interval, and a probe in another process reports it unhealthy once the mark
// goes stale. A process that is hung holds its container running, and `docker compose ps` shows a
// running container, so nothing else tells a stuck worker from a working one.
package heartbeat

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

const (
	// Path is where a container's mark is written. It is under /tmp, which the service mounts as a
	// tmpfs of its own: the mark outlives no restart and belongs to no volume.
	Path = "/tmp/worker-heartbeat"

	// Interval is how often the mark is written.
	Interval = 10 * time.Second

	// StaleAfter is how old a mark may be before its process counts as no longer making progress. It
	// is several intervals, so one late write on a loaded machine is not a failure.
	StaleAfter = 3 * Interval

	// markMode is the permission of the mark file, which the probe of the same user reads.
	markMode = 0o600
)

// Beat writes the mark every Interval until ctx is done. The first mark is written before the first
// interval, so a process that has not been up for a whole interval is not indistinguishable from one
// that never ran.
func Beat(ctx context.Context, path string) {
	mark(path)
	ticker := time.NewTicker(Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mark(path)
		}
	}
}

// Write records one mark, stating the moment it was written. A caller states its own moment so that
// a stale mark is something a test can produce rather than wait minutes for.
func Write(path string, moment time.Time) error {
	return os.WriteFile(path, []byte(moment.UTC().Format(time.RFC3339Nano)), markMode)
}

// Fresh reports whether the mark states a moment within StaleAfter of now. A missing, unreadable or
// older mark is an error saying what the probe observed, which is what the health check reports.
func Fresh(path string, now time.Time) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	written, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(string(content)))
	if err != nil {
		return fmt.Errorf("%s states no moment: %w", path, err)
	}
	if age := now.Sub(written); age > StaleAfter {
		return fmt.Errorf("the mark is %s old, later than %s", age.Round(time.Second), StaleAfter)
	}
	return nil
}

// mark reports a failure every interval rather than once: a process whose mark cannot be written is
// one the probe will report as unhealthy, and the reason belongs in the log beside that report.
func mark(path string) {
	if err := Write(path, time.Now()); err != nil {
		slog.Error("heartbeat could not be written", "path", path, "error", err.Error())
	}
}
