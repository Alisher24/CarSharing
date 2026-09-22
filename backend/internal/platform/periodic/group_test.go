package periodic_test

import (
	"context"
	"testing"
	"testing/synctest"

	"github.com/Alisher24/CarSharing/backend/internal/platform/periodic"
)

func TestRunAllWaitsForTheLastTaskToFinish(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		finished := make(chan struct{})
		release := make(chan struct{})
		returned := make(chan struct{})
		go func() {
			periodic.RunAll(ctx, func(ctx context.Context) {
				<-ctx.Done()
				<-release
				close(finished)
			})
			close(returned)
		}()
		cancel()
		synctest.Wait()
		select {
		case <-returned:
			t.Fatal("the process returned while its task still held dependencies")
		default:
		}
		close(release)
		<-returned
		select {
		case <-finished:
		default:
			t.Fatal("the process did not wait for its task")
		}
	})
}
