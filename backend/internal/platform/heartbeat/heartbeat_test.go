package heartbeat_test

import (
	"context"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Alisher24/CarSharing/backend/internal/platform/heartbeat"
)

/** A path no other test writes, standing in for the /tmp of a container. */
func markPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "worker-heartbeat")
}

func TestBeatKeepsTheMarkFreshWhileItsProcessRuns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		path := markPath(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go heartbeat.Beat(ctx, path)

		// The first mark is written before the first interval, so a process that has just started is
		// already distinguishable from one that never ran.
		synctest.Wait()
		if err := heartbeat.Fresh(path, time.Now()); err != nil {
			t.Fatalf("a process that has just started is not fresh: %v", err)
		}

		time.Sleep(heartbeat.StaleAfter + heartbeat.Interval)
		synctest.Wait()
		if err := heartbeat.Fresh(path, time.Now()); err != nil {
			t.Fatalf("a running process went stale: %v", err)
		}

		cancel()
		time.Sleep(heartbeat.StaleAfter + heartbeat.Interval)
		synctest.Wait()
		if err := heartbeat.Fresh(path, time.Now()); err == nil {
			t.Fatal("a process that stopped marking stayed fresh")
		}
	})
}

func TestAMarkNobodyWroteIsNotFresh(t *testing.T) {
	if err := heartbeat.Fresh(markPath(t), time.Now()); err == nil {
		t.Fatal("a mark nobody wrote is fresh")
	}
}

func TestAMarkOlderThanTheStaleBoundIsNotFresh(t *testing.T) {
	path := markPath(t)
	if err := heartbeat.Write(path, time.Now().Add(-heartbeat.StaleAfter-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := heartbeat.Fresh(path, time.Now()); err == nil {
		t.Fatal("a mark older than the stale bound is fresh")
	}
}

func TestAMarkWithinTheStaleBoundIsFresh(t *testing.T) {
	path := markPath(t)
	if err := heartbeat.Write(path, time.Now().Add(-heartbeat.StaleAfter+time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := heartbeat.Fresh(path, time.Now()); err != nil {
		t.Fatalf("a mark within the stale bound is not fresh: %v", err)
	}
}

func TestTheStaleBoundIsSeveralIntervals(t *testing.T) {
	if heartbeat.StaleAfter <= heartbeat.Interval {
		t.Fatalf("a mark goes stale after %s, which is one interval or less", heartbeat.StaleAfter)
	}
}
